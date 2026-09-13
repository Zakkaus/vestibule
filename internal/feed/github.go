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
	"github.com/Zakkaus/vestibule/internal/telegram/queue"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	maxCommitsPerCycle = 10
	maxGitHubTracked   = 100

	githubItemOpen     = "open"
	githubItemReopened = "reopened"
	githubItemMerged   = "merged"
	githubItemClosed   = "closed"
)

type trackedGitHubItem struct {
	MsgID     int    `json:"msg_id"`
	State     string `json:"state"`
	EditFails int    `json:"edit_fails,omitempty"`
}

type githubRepoState struct {
	LastID                string                    `json:"last_id"`
	CommitBaselinePending bool                      `json:"commit_baseline_pending,omitempty"`
	LastIssue             int                       `json:"last_issue,omitempty"`
	IssuesInitialized     bool                      `json:"issues_initialized,omitempty"`
	LastPull              int                       `json:"last_pull,omitempty"`
	PullsInitialized      bool                      `json:"pulls_initialized,omitempty"`
	Tracked               map[int]trackedGitHubItem `json:"tracked,omitempty"`
}

type githubState struct {
	Repos         map[string]githubRepoState `json:"repos,omitempty"`
	NextRepo      int                        `json:"next_repo"`
	RemovedCycles map[string]int             `json:"removed_cycles,omitempty"`
}

func (s feedState) MarshalJSON() ([]byte, error) {
	type stateJSON feedState
	if s.GitHub != nil && len(s.GitHub.Repos) == 0 {
		s.GitHub = nil
	}
	return json.Marshal(stateJSON(s))
}

func (s *githubState) UnmarshalJSON(data []byte) error {
	var raw struct {
		Repos         map[string]json.RawMessage `json:"repos"`
		NextRepo      int                        `json:"next_repo"`
		RemovedCycles map[string]int             `json:"removed_cycles"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.NextRepo = raw.NextRepo
	s.RemovedCycles = raw.RemovedCycles
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
	if st.RemovedCycles == nil {
		st.RemovedCycles = map[string]int{}
	}
	keys := configuredGitHubRepos(f)
	for key := range st.RemovedCycles {
		if keys[key] {
			delete(st.RemovedCycles, key)
		}
	}
	for key := range st.Repos {
		if keys[key] {
			continue
		}
		if st.RemovedCycles[key] > 0 {
			delete(st.Repos, key)
			delete(st.RemovedCycles, key)
			continue
		}
		st.RemovedCycles[key] = 1
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

func githubItemState(item lookup.GitHubItem) string {
	if item.IsPull && item.MergedAt != nil {
		return githubItemMerged
	}
	if item.State == githubItemClosed {
		return githubItemClosed
	}
	return githubItemOpen
}

func (state *githubRepoState) trackGitHubItem(item lookup.GitHubItem, messageID int) {
	if messageID == 0 {
		return
	}
	if state.Tracked == nil {
		state.Tracked = make(map[int]trackedGitHubItem)
	}
	if _, exists := state.Tracked[item.Number]; !exists {
		for len(state.Tracked) >= maxGitHubTracked {
			state.evictTrackedGitHubItem()
		}
	}
	state.Tracked[item.Number] = trackedGitHubItem{MsgID: messageID, State: githubItemState(item)}
}

func (state *githubRepoState) evictTrackedGitHubItem() {
	candidate, terminal := 0, 0
	for number, tracked := range state.Tracked {
		if candidate == 0 || number < candidate {
			candidate = number
		}
		if tracked.State != githubItemOpen && (terminal == 0 || number < terminal) {
			terminal = number
		}
	}
	if terminal != 0 {
		candidate = terminal
	}
	delete(state.Tracked, candidate)
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
	return renderGitHubItemState(item, repo, l, githubItemState(item))
}

func renderGitHubItemState(item lookup.GitHubItem, repo string, l i18n.Lang, state string) string {
	catalog := i18n.Messages.Feed.GitHub
	marker, status := "🟢", catalog.State.Open
	switch state {
	case githubItemReopened:
		status = catalog.State.Reopened
	case githubItemMerged:
		marker, status = "🟣", catalog.State.Merged
	case githubItemClosed:
		marker, status = "🔴", catalog.State.Closed
	}
	kind := catalog.Kind.Issue
	if item.IsPull {
		kind = catalog.Kind.Pull
	}
	return catalog.Item.Render(l,
		marker,
		html.EscapeString(item.URL),
		html.EscapeString(repo),
		item.Number,
		kind.For(l),
		status.For(l),
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

func pollGitHubWithFetcher(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, fetch func(context.Context, string, string) ([]lookup.Commit, error)) {
	edits := 0
	pollGitHubWithEditBudget(ctx, bot, f, st, &edits, fetch, lookup.RecentGitHubItems)
}

type githubItemsFetcher func(context.Context, string) ([]lookup.GitHubItem, bool, error)

func pollGitHubWithFetchers(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, commitFetch func(context.Context, string, string) ([]lookup.Commit, error), itemFetch githubItemsFetcher) {
	edits := 0
	pollGitHubWithEditBudget(ctx, bot, f, st, &edits, commitFetch, itemFetch)
}

func pollGitHubWithEditBudget(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, edits *int, commitFetch func(context.Context, string, string) ([]lookup.Commit, error), itemFetch githubItemsFetcher) githubRepoResult {
	if len(f.GitHubRepos) == 0 {
		if st.GitHub == nil {
			return githubRepoComplete
		}
		normalizeGitHubState(st.GitHub, f)
		if len(st.GitHub.Repos) == 0 {
			st.GitHub = nil
		}
		return githubRepoComplete
	}
	gs := st.GitHub
	if gs == nil {
		gs = &githubState{Repos: map[string]githubRepoState{}}
	}
	normalizeGitHubState(gs, f)
	budget, result := 0, githubRepoComplete
	start := gs.NextRepo
	for visited := 0; visited < len(f.GitHubRepos) && budget < maxCommitsPerCycle; visited++ {
		if ctx.Err() != nil {
			result = githubRepoCanceled
			break
		}
		index := (start + visited) % len(f.GitHubRepos)
		result = pollGitHubRepo(ctx, bot, f, gs, f.GitHubRepos[index], &budget, edits, commitFetch, itemFetch)
		if result == githubRepoRateLimited || result == githubRepoCanceled {
			gs.NextRepo = index
			break
		}
		gs.NextRepo = (index + 1) % len(f.GitHubRepos)
	}
	if len(gs.Repos) == 0 {
		st.GitHub = nil
	} else {
		st.GitHub = gs
	}
	return result
}

// pollGitHubRepo fetches one repository's commits and, when enabled, its issues and
// pull requests, refreshes tracked messages, delivers what the budget allows, and stores state.
func pollGitHubRepo(ctx context.Context, bot feedBot, f *settings.FeedConfig, gs *githubState, repo settings.GitHubRepo, budget, edits *int, commitFetch func(context.Context, string, string) ([]lookup.Commit, error), itemFetch githubItemsFetcher) githubRepoResult {
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
		result = refreshTrackedGitHubItems(ctx, bot, f, repo.Repo, &repoState, feedLanguage(f.Lang), items, edits)
		if result == githubRepoComplete {
			result = deliverGitHubEvents(ctx, bot, f, repo, &repoState, feedLanguage(f.Lang), budget, items, full)
		}
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

func refreshTrackedGitHubItems(ctx context.Context, bot feedBot, f *settings.FeedConfig, repo string, state *githubRepoState, l i18n.Lang, items []lookup.GitHubItem, edits *int) githubRepoResult {
	for _, item := range items {
		tracked, ok := state.Tracked[item.Number]
		if !ok || tracked.State == githubItemState(item) {
			continue
		}
		if *edits >= maxEditsPerCycle {
			return githubRepoComplete
		}
		current := githubItemState(item)
		display := current
		if current == githubItemOpen && tracked.State != githubItemOpen {
			display = githubItemReopened
		}
		edit := tgfmt.HTMLMessage(f.ChatID, renderGitHubItemState(item, repo, l, display))
		opCtx, cancel := context.WithTimeout(ctx, feedTelegramTimeout)
		_, err := bot.EditMessageText(opCtx, &telego.EditMessageTextParams{
			ChatID:             tu.ID(f.ChatID),
			MessageID:          tracked.MsgID,
			Text:               edit.Text,
			ParseMode:          edit.ParseMode,
			LinkPreviewOptions: edit.LinkPreviewOptions,
		})
		cancel()
		(*edits)++
		switch {
		case err == nil || queue.IsNotModified(err):
			tracked.EditFails = 0
			tracked.State = current
			state.Tracked[item.Number] = tracked
		case queue.IsRateLimited(err):
			log.Printf("feed: edit tracked GitHub item %s #%d in %d rate-limited (%v) — pausing edits this cycle", repo, item.Number, f.ChatID, err)
			return githubRepoRateLimited
		case queue.PermanentEditError(err):
			log.Printf("feed: drop tracked GitHub item %s #%d in %d (uneditable): %v", repo, item.Number, f.ChatID, err)
			delete(state.Tracked, item.Number)
		case queue.CountablePermanentEditError(err):
			tracked.EditFails++
			log.Printf("feed: edit tracked GitHub item %s #%d in %d (deterministic 400 %d/%d): %v", repo, item.Number, f.ChatID, tracked.EditFails, maxEditFails, err)
			if tracked.EditFails >= maxEditFails {
				log.Printf("feed: drop tracked GitHub item %s #%d in %d after %d deterministic edit rejections", repo, item.Number, f.ChatID, maxEditFails)
				delete(state.Tracked, item.Number)
			} else {
				state.Tracked[item.Number] = tracked
			}
		default:
			tracked.EditFails = 0
			state.Tracked[item.Number] = tracked
			log.Printf("feed: edit tracked GitHub item %s #%d in %d (transient, tracking retained): %v", repo, item.Number, f.ChatID, err)
		}
		if !queue.Pace(ctx, feedSendPause) {
			return githubRepoCanceled
		}
	}
	return githubRepoComplete
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
		messageID, ok, rateLimited, permanent := postFeed(ctx, bot, f.ChatID, renderGitHubItem(item, repo.Repo, l), false, 0)
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
		if ok {
			state.trackGitHubItem(item, messageID)
		}
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
