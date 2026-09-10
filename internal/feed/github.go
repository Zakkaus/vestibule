package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
)

const maxCommitsPerCycle = 10

type githubRepoState struct {
	LastID string `json:"last_id"`
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
	pollGitHubWithFetcher(ctx, bot, f, st, lookup.RecentCommits)
}

func pollGitHubWithFetcher(ctx context.Context, bot feedBot, f *settings.FeedConfig, st *feedState, fetch func(context.Context, string, string) ([]lookup.Commit, error)) {
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
		repo := f.GitHubRepos[index]
		key := githubRepoKey(repo.Repo, repo.Branch)
		repoState, initialized := gs.Repos[key]
		commits, err := fetch(ctx, repo.Repo, repo.Branch)
		result := githubRepoTransient
		if err != nil {
			log.Printf("feed: WARNING GitHub %s@%s: %v", repo.Repo, repo.Branch, err)
		} else {
			result = deliverGitHubRepo(ctx, bot, f, repo, &repoState, initialized, feedLanguage(f.Lang), &budget, commits)
		}
		if initialized || result == githubRepoComplete {
			gs.Repos[key] = repoState
		}
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
