package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"sort"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
)

const maxCommitsPerCycle = 10

type githubRepoState struct {
	LastID                string `json:"last_id"`
	CommitBaselinePending bool   `json:"commit_baseline_pending,omitempty"`
	LastIssue             int    `json:"last_issue,omitempty"`
	IssuesInitialized     bool   `json:"issues_initialized,omitempty"`
	LastPull              int    `json:"last_pull,omitempty"`
	PullsInitialized      bool   `json:"pulls_initialized,omitempty"`
}

type githubState struct {
	Repos    map[string]githubRepoState `json:"repos,omitempty"`
	NextRepo int                        `json:"next_repo"`
}

func (s feedState) MarshalJSON() ([]byte, error) {
	type stateJSON feedState
	if s.GitHub != nil && len(s.GitHub.Repos) == 0 {
		s.GitHub = nil
	}
	return json.Marshal(stateJSON(s))
}

// UnmarshalJSON drops null repository entries instead of turning them into initialized cursors.
func (s *githubState) UnmarshalJSON(data []byte) error {
	var raw struct {
		Repos    map[string]json.RawMessage `json:"repos"`
		NextRepo int                        `json:"next_repo"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.NextRepo = raw.NextRepo
	s.Repos = make(map[string]githubRepoState, len(raw.Repos))
	for key, value := range raw.Repos {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var state githubRepoState
		if err := json.Unmarshal(value, &state); err != nil {
			return fmt.Errorf("github repo %q: %w", key, err)
		}
		s.Repos[key] = state
	}
	return nil
}

func githubRepoKey(repo, branch string) string { return repo + "@" + branch }

func configuredGitHubRepos(f *settings.FeedConfig) map[string]bool {
	keys := make(map[string]bool, len(f.GitHubRepos))
	for _, repo := range f.GitHubRepos {
		keys[githubRepoKey(repo.Repo, repo.Branch)] = true
	}
	return keys
}

func normalizeGitHubState(st *githubState, f *settings.FeedConfig) {
	if st.Repos == nil {
		st.Repos = map[string]githubRepoState{}
	}
	keys := configuredGitHubRepos(f)
	for key := range st.Repos {
		if !keys[key] {
			delete(st.Repos, key)
		}
	}
	if len(f.GitHubRepos) > 0 {
		st.NextRepo = normalizeGitHubIndex(st.NextRepo, len(f.GitHubRepos))
	}
}

func normalizeGitHubIndex(index, count int) int {
	if count == 0 {
		return 0
	}
	index %= count
	if index < 0 {
		index += count
	}
	return index
}

func renderGitHubCommit(c lookup.Commit, repo, branch string, l i18n.Lang) string {
	catalog := i18n.Messages.Feed.GitHub
	branchText := ""
	if branch != "" {
		branchText = catalog.Branch.Render(l, html.EscapeString(branch))
	}
	authorText := ""
	if strings.TrimSpace(c.Author) != "" {
		authorText = catalog.Author.Render(l, html.EscapeString(c.Author))
	}
	return catalog.Commit.Render(l,
		html.EscapeString(c.URL),
		html.EscapeString(repo),
		html.EscapeString(c.ShortSHA),
		html.EscapeString(c.Title),
		branchText,
		authorText,
	)
}

func renderGitHubItem(item lookup.GitHubItem, repo string, l i18n.Lang) string {
	template := i18n.Messages.Feed.GitHub.IssueOpened
	if item.IsPull {
		template = i18n.Messages.Feed.GitHub.PullOpened
	}
	return template.Render(l,
		html.EscapeString(item.URL),
		html.EscapeString(repo),
		item.Number,
		html.EscapeString(item.Title),
		html.EscapeString(item.Author),
	)
}

type githubRepoResult uint8

const (
	githubRepoComplete githubRepoResult = iota
	githubRepoTransient
	githubRepoRateLimited
	githubRepoCanceled
)

func githubPending(commits []lookup.Commit, cursor string) ([]lookup.Commit, bool) {
	if cursor == "" {
		return commits, true
	}
	for i, commit := range commits {
		if commit.ID == cursor {
			return commits[:i], true
		}
	}
	return nil, false
}
func deliverGitHubRepo(ctx context.Context, bot feedBot, f *settings.FeedConfig, repo settings.GitHubRepo, state *githubRepoState, initialized bool, l i18n.Lang, budget *int, commits []lookup.Commit) githubRepoResult {
	if len(commits) == 0 {
		log.Printf("feed: GitHub %s@%s returned a valid empty page", repo.Repo, repo.Branch)
		return githubRepoComplete
	}
	pending, found := githubPending(commits, state.LastID)
	if !found {
		log.Printf("feed: WARNING %d: GitHub cursor for %s@%s is not on the fetched page; re-baselining", f.ChatID, repo.Repo, repo.Branch)
		state.LastID = commits[0].ID
		return githubRepoComplete
	}
	if !initialized && state.LastID == "" {
		state.LastID = commits[0].ID
		log.Printf("feed: %d baselining GitHub cursor for %s@%s", f.ChatID, repo.Repo, repo.Branch)
		return githubRepoComplete
	}
	for i := len(pending) - 1; i >= 0 && *budget < maxCommitsPerCycle; i-- {
		if ctx.Err() != nil {
			return githubRepoCanceled
		}
		commit := pending[i]
		_, ok, rateLimited, permanent := postFeed(ctx, bot, f.ChatID, renderGitHubCommit(commit, repo.Repo, repo.Branch, l), false, 0)
		if rateLimited {
			return githubRepoRateLimited
		}
		if !ok {
			if permanent {
				log.Printf("feed: skip permanently rejected GitHub commit %s in %d", commit.ID, f.ChatID)
				state.LastID = commit.ID
				(*budget)++
				continue
			}
			return githubRepoTransient
		}
		state.LastID = commit.ID
		(*budget)++
	}
	return githubRepoComplete
}

func pollGitHub(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState) {
	pollGitHubWithFetchers(ctx, bot, f, st, lookup.RecentCommits, lookup.RecentGitHubItems)
}

func pollGitHubWithFetcher(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, fetch func(context.Context, string, string) ([]lookup.Commit, error)) {
	pollGitHubWithFetchers(ctx, bot, f, st, fetch, lookup.RecentGitHubItems)
}

type githubItemsFetcher func(context.Context, string) ([]lookup.GitHubItem, bool, error)

func pollGitHubWithFetchers(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, commitFetch func(context.Context, string, string) ([]lookup.Commit, error), itemFetch githubItemsFetcher) {
	if len(f.GitHubRepos) == 0 {
		st.GitHub = nil
		return
	}
	gs := st.GitHub
	if gs == nil {
		gs = &githubState{Repos: map[string]githubRepoState{}}
	}
	normalizeGitHubState(gs, f)
	budget := 0
	start := gs.NextRepo
	for visited := 0; visited < len(f.GitHubRepos) && budget < maxCommitsPerCycle; visited++ {
		if ctx.Err() != nil {
			break
		}
		index := (start + visited) % len(f.GitHubRepos)
		result := pollGitHubRepo(ctx, bot, f, gs, f.GitHubRepos[index], &budget, commitFetch, itemFetch)
		if result == githubRepoRateLimited {
			gs.NextRepo = index
			break
		}
		if result == githubRepoCanceled {
			break
		}
		gs.NextRepo = (index + 1) % len(f.GitHubRepos)
	}
	if len(gs.Repos) == 0 {
		st.GitHub = nil
		return
	}
	st.GitHub = gs
}

// pollGitHubRepo fetches one repository's commits and, when enabled, its issues and
// pull requests, delivers what the budget allows, and stores the repository state.
func pollGitHubRepo(ctx context.Context, bot feedBot, f *settings.FeedConfig, gs *githubState, repo settings.GitHubRepo, budget *int, commitFetch func(context.Context, string, string) ([]lookup.Commit, error), itemFetch githubItemsFetcher) githubRepoResult {
	key := githubRepoKey(repo.Repo, repo.Branch)
	repoState, initialized := gs.Repos[key]
	commits, commitErr := commitFetch(ctx, repo.Repo, repo.Branch)
	items, full, itemsOK := fetchGitHubItems(ctx, repo, itemFetch)
	result := githubRepoTransient
	if commitErr != nil {
		log.Printf("feed: WARNING GitHub %s@%s: %v", repo.Repo, repo.Branch, commitErr)
	} else {
		result = deliverGitHubRepo(ctx, bot, f, repo, &repoState, initialized && !repoState.CommitBaselinePending, feedLanguage(f.Lang), budget, commits)
		if repoState.CommitBaselinePending && result == githubRepoComplete {
			repoState.CommitBaselinePending = false
		}
	}
	saveState := initialized || result == githubRepoComplete
	if result != githubRepoRateLimited && result != githubRepoCanceled && itemsOK {
		result = deliverGitHubEvents(ctx, bot, f, repo, &repoState, feedLanguage(f.Lang), budget, items, full)
		if !initialized && commitErr != nil {
			repoState.CommitBaselinePending = true
		}
		saveState = true
	}
	if saveState {
		gs.Repos[key] = repoState
	}
	return result
}

// fetchGitHubItems reads the issues page only when a kind is enabled; a failed read is
// logged and reported as not usable so the cursors stay where they are.
func fetchGitHubItems(ctx context.Context, repo settings.GitHubRepo, itemFetch githubItemsFetcher) ([]lookup.GitHubItem, bool, bool) {
	if !repo.IssuesOn() && !repo.PullsOn() {
		return nil, false, false
	}
	items, full, err := itemFetch(ctx, repo.Repo)
	if err != nil {
		log.Printf("feed: WARNING GitHub REST %s: %v", repo.Repo, err)
		return nil, false, false
	}
	return items, full, true
}

func deliverGitHubEvents(ctx context.Context, bot feedBot, f *settings.FeedConfig, repo settings.GitHubRepo, state *githubRepoState, l i18n.Lang, budget *int, items []lookup.GitHubItem, full bool) githubRepoResult {
	pageLow, pageHigh := githubItemBounds(items)
	result := deliverGitHubCategory(ctx, bot, f, repo, state, l, budget, items, full, pageLow, pageHigh, false)
	if result == githubRepoRateLimited || result == githubRepoCanceled {
		return result
	}
	return deliverGitHubCategory(ctx, bot, f, repo, state, l, budget, items, full, pageLow, pageHigh, true)
}

// githubCategory is one kind of item (issue or pull request) with its own cursor and
// baseline flag on the repository state.
type githubCategory struct {
	kind        string
	cursor      *int
	initialized *bool
	items       []lookup.GitHubItem
}

func selectGitHubCategory(repo settings.GitHubRepo, state *githubRepoState, items []lookup.GitHubItem, pull bool) (githubCategory, bool) {
	category := githubCategory{kind: "issue", cursor: &state.LastIssue, initialized: &state.IssuesInitialized}
	enabled := repo.IssuesOn()
	if pull {
		category = githubCategory{kind: "pull request", cursor: &state.LastPull, initialized: &state.PullsInitialized}
		enabled = repo.PullsOn()
	}
	for _, item := range items {
		if item.IsPull == pull {
			category.items = append(category.items, item)
		}
	}
	return category, enabled
}

// settleGitHubCursor baselines a kind on first sight and advances past a truncated
// page; it reports whether delivery should proceed.
func settleGitHubCursor(f *settings.FeedConfig, repo settings.GitHubRepo, c githubCategory, full bool, pageLow, pageHigh int) bool {
	low, high := githubItemBounds(c.items)
	if !*c.initialized {
		*c.initialized = true
		if high > 0 {
			*c.cursor = high
		} else if full && pageLow > *c.cursor {
			warnGitHubTruncation(f, repo, c.kind)
			*c.cursor = pageHigh
		}
		log.Printf("feed: %d baselining GitHub %s cursor for %s@%s", f.ChatID, c.kind, repo.Repo, repo.Branch)
		return false
	}
	if full && ((low > 0 && low > *c.cursor) || (low == 0 && pageLow > *c.cursor)) {
		warnGitHubTruncation(f, repo, c.kind)
		if high > 0 {
			*c.cursor = high
		} else {
			*c.cursor = pageHigh
		}
		return false
	}
	return true
}

func deliverGitHubCategory(ctx context.Context, bot feedBot, f *settings.FeedConfig, repo settings.GitHubRepo, state *githubRepoState, l i18n.Lang, budget *int, items []lookup.GitHubItem, full bool, pageLow, pageHigh int, pull bool) githubRepoResult {
	category, enabled := selectGitHubCategory(repo, state, items, pull)
	if !enabled || !settleGitHubCursor(f, repo, category, full, pageLow, pageHigh) {
		return githubRepoComplete
	}
	sort.Slice(category.items, func(i, j int) bool { return category.items[i].Number < category.items[j].Number })
	for _, item := range category.items {
		if item.Number <= *category.cursor || *budget >= maxCommitsPerCycle {
			continue
		}
		if ctx.Err() != nil {
			return githubRepoCanceled
		}
		_, ok, rateLimited, permanent := postFeed(ctx, bot, f.ChatID, renderGitHubItem(item, repo.Repo, l), false, 0)
		if rateLimited {
			return githubRepoRateLimited
		}
		if !ok && !permanent {
			return githubRepoTransient
		}
		if permanent {
			log.Printf("feed: skip permanently rejected GitHub %s #%d in %d", category.kind, item.Number, f.ChatID)
		}
		*category.cursor = item.Number
		(*budget)++
	}
	return githubRepoComplete
}

func githubItemBounds(items []lookup.GitHubItem) (int, int) {
	if len(items) == 0 {
		return 0, 0
	}
	low, high := items[0].Number, items[0].Number
	for _, item := range items[1:] {
		low = min(low, item.Number)
		high = max(high, item.Number)
	}
	return low, high
}

func warnGitHubTruncation(f *settings.FeedConfig, repo settings.GitHubRepo, kind string) {
	log.Printf("feed: WARNING %d: GitHub %s cursor for %s@%s is behind the fetched page; re-baselining", f.ChatID, kind, repo.Repo, repo.Branch)
}
