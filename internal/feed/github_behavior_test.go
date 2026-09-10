package feed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego/telegoapi"
)

func githubTestCommit(n int) lookup.Commit {
	sha := fmt.Sprintf("%040x", n)
	return lookup.Commit{
		ID: "tag:github.com,2008:Grit::Commit/" + sha, SHA: sha, ShortSHA: sha[:7],
		Title: fmt.Sprintf("commit-%d", n), URL: "https://github.com/o/r/commit/" + sha,
	}
}

func githubTestFeed() *settings.FeedConfig {
	return &settings.FeedConfig{ChatID: -1009000002401, Lang: "en", GitHubRepos: []settings.GitHubRepo{{Repo: "o/r"}}}
}
func disableGitHubTestPacing(t *testing.T) {
	t.Helper()
	old := feedSendPause
	feedSendPause = 0
	t.Cleanup(func() { feedSendPause = old })
}

func TestGitHubStateTable(t *testing.T) {
	disableGitHubTestPacing(t)
	commits := []lookup.Commit{githubTestCommit(3), githubTestCommit(2), githubTestCommit(1)}
	key := githubRepoKey("o/r", "")
	cases := []struct {
		name, cursor string
		state        *githubState
		result       []lookup.Commit
		err          error
		wantCursor   string
		wantGitHub   bool
		wantSends    int
	}{
		{name: "missing key fetch failure", err: errors.New("timeout")},
		{name: "missing key empty page", result: []lookup.Commit{}, wantGitHub: true},
		{name: "missing key nonempty page", result: commits, wantGitHub: true, wantCursor: commits[0].ID},
		{name: "empty cursor fetch failure", state: &githubState{Repos: map[string]githubRepoState{key: {}}}, err: errors.New("timeout"), wantGitHub: true},
		{name: "empty cursor empty page", state: &githubState{Repos: map[string]githubRepoState{key: {}}}, result: []lookup.Commit{}, wantGitHub: true},
		{name: "empty cursor nonempty page", state: &githubState{Repos: map[string]githubRepoState{key: {}}}, result: commits, wantGitHub: true, wantCursor: commits[0].ID, wantSends: 3},
		{name: "nonempty cursor fetch failure", cursor: commits[1].ID, state: &githubState{Repos: map[string]githubRepoState{key: {LastID: commits[1].ID}}}, err: errors.New("timeout"), wantGitHub: true, wantCursor: commits[1].ID},
		{name: "nonempty cursor empty page", cursor: commits[1].ID, state: &githubState{Repos: map[string]githubRepoState{key: {LastID: commits[1].ID}}}, result: []lookup.Commit{}, wantGitHub: true, wantCursor: commits[1].ID},
		{name: "cursor at newest", cursor: commits[0].ID, state: &githubState{Repos: map[string]githubRepoState{key: {LastID: commits[0].ID}}}, result: commits, wantGitHub: true, wantCursor: commits[0].ID},
		{name: "cursor in page", cursor: commits[1].ID, state: &githubState{Repos: map[string]githubRepoState{key: {LastID: commits[1].ID}}}, result: commits, wantGitHub: true, wantCursor: commits[0].ID, wantSends: 1},
		{name: "cursor outside page", cursor: "missing", state: &githubState{Repos: map[string]githubRepoState{key: {LastID: "missing"}}}, result: commits, wantGitHub: true, wantCursor: commits[0].ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &feedState{GitHub: tc.state}
			bot := &fakeFeedBot{}
			fetch := func(context.Context, string, string) ([]lookup.Commit, error) { return tc.result, tc.err }
			pollGitHubWithFetcher(context.Background(), bot, githubTestFeed(), st, fetch)
			if (st.GitHub != nil) != tc.wantGitHub {
				t.Fatalf("github state present = %v, want %v (%+v)", st.GitHub != nil, tc.wantGitHub, st.GitHub)
			}
			if tc.wantGitHub && st.GitHub.Repos[key].LastID != tc.wantCursor {
				t.Errorf("cursor = %q, want %q", st.GitHub.Repos[key].LastID, tc.wantCursor)
			}
			if bot.sends != tc.wantSends {
				t.Errorf("sends = %d, want %d", bot.sends, tc.wantSends)
			}
		})
	}
}

func TestGitHubBudgetAndRotation(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}, {Repo: "o/c"}}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {LastID: "old-o/a"},
		githubRepoKey("o/b", ""): {LastID: "old-o/b"},
		githubRepoKey("o/c", ""): {LastID: "old-o/c"},
	}}}
	var visited []string
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		commits := make([]lookup.Commit, 12)
		for i := range commits {
			commits[i] = lookup.Commit{ID: "new-" + repo + "-" + string(rune('a'+i))}
		}
		commits[len(commits)-1].ID = "old-" + repo
		return commits, nil
	}
	bot := &fakeFeedBot{}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if bot.sends != 10 {
		t.Fatalf("sends = %d, want 10", bot.sends)
	}
	if len(visited) != 1 || visited[0] != "o/a" {
		t.Fatalf("visited %v, want one visit to first repo", visited)
	}
	if state.GitHub.NextRepo != 1 {
		t.Errorf("next repo = %d, want 1 after budget stop", state.GitHub.NextRepo)
	}
	for _, wantRepo := range []string{"o/b", "o/c"} {
		visited, bot.sends = nil, 0
		pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
		if strings.Join(visited, ",") != wantRepo || bot.sends != 10 {
			t.Fatalf("rotated round: visited=%v sends=%d, want %s and 10", visited, bot.sends, wantRepo)
		}
	}
	visited, bot.sends = nil, 0
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if strings.Join(visited, ",") != "o/a,o/b,o/c" || bot.sends != 3 {
		t.Fatalf("remaining commits: visited=%v sends=%d, want one per repo", visited, bot.sends)
	}

	state = &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {},
		githubRepoKey("o/b", ""): {},
		githubRepoKey("o/c", ""): {},
	}}}
	visited = nil
	pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, state,
		func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
			visited = append(visited, repo)
			return []lookup.Commit{}, nil
		})
	if strings.Join(visited, ",") != "o/a,o/b,o/c" || state.GitHub.NextRepo != 0 {
		t.Errorf("complete circle = %v next=%d, want all repos and next 0", visited, state.GitHub.NextRepo)
	}
}

func TestGitHubErrorsAndCancellation(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {LastID: "old-a"},
		githubRepoKey("o/b", ""): {LastID: "old-b"},
	}}}
	visited := []string{}
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		if repo == "o/a" {
			return nil, errors.New("temporary")
		}
		return []lookup.Commit{{ID: "new-b"}, {ID: "old-b"}}, nil
	}
	bot := &fakeFeedBot{}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if len(visited) != 2 || bot.sends != 1 || state.GitHub.Repos[githubRepoKey("o/b", "")].LastID != "new-b" {
		t.Fatalf("transient repo prevented later progress: visited=%v state=%+v", visited, state.GitHub)
	}

	ctx, cancel := context.WithCancel(context.Background())
	visited = nil
	fetch = func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		cancel()
		return []lookup.Commit{{ID: "new-a"}, {ID: "old-a"}}, nil
	}
	state = &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {LastID: "old-a"},
		githubRepoKey("o/b", ""): {LastID: "old-b"},
	}}}
	bot = &fakeFeedBot{}
	pollGitHubWithFetcher(ctx, bot, feed, state, fetch)
	if len(visited) != 1 || bot.sends != 0 || state.GitHub.Repos["o/a@"].LastID != "old-a" {
		t.Errorf("cancellation: visited=%v sends=%d state=%+v, want untouched prefix", visited, bot.sends, state.GitHub)
	}
}

func TestGitHubPermanentRejectionAndRateLimit(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {LastID: "old-o/a"},
		githubRepoKey("o/b", ""): {LastID: "old-o/b"},
	}}}
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		return []lookup.Commit{{ID: "new-" + repo}, {ID: "old-" + repo}}, nil
	}
	bot := &scriptedFeedBot{fakeFeedBot: &fakeFeedBot{}, sendErrs: []error{&telegoapi.Error{ErrorCode: 400, Description: "message is too long"}, nil}}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if state.GitHub.Repos[githubRepoKey("o/a", "")].LastID != "new-o/a" || bot.sends != 2 {
		t.Errorf("permanent rejection did not advance: state=%+v sends=%d", state.GitHub, bot.sends)
	}

	state = &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		githubRepoKey("o/a", ""): {LastID: "old-o/a"},
		githubRepoKey("o/b", ""): {LastID: "old-o/b"},
	}}}
	bot = &scriptedFeedBot{fakeFeedBot: &fakeFeedBot{}, sendErrs: []error{errors.New("Too Many Requests: retry after 5")}}
	visited := []string{}
	fetch = func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		return []lookup.Commit{{ID: "new-" + repo}, {ID: "old-" + repo}}, nil
	}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if len(visited) != 1 || bot.sends != 1 || state.GitHub.NextRepo != 0 || state.GitHub.Repos[githubRepoKey("o/a", "")].LastID != "old-o/a" {
		t.Errorf("rate limit did not stop destination: visited=%v next=%d state=%+v", visited, state.GitHub.NextRepo, state.GitHub)
	}
}

func TestGitHubTransientSendPreservesPrefix(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}}
	oldest := githubTestCommit(0).ID
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/a@": {LastID: oldest}, "o/b@": {LastID: oldest},
	}}}
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		if repo == "o/a" {
			return []lookup.Commit{githubTestCommit(3), githubTestCommit(2), githubTestCommit(1), githubTestCommit(0)}, nil
		}
		return []lookup.Commit{githubTestCommit(1), githubTestCommit(0)}, nil
	}
	bot := &scriptedFeedBot{fakeFeedBot: &fakeFeedBot{}, sendErrs: []error{nil, errors.New("temporary transport failure"), nil}}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if bot.sends != 3 {
		t.Fatalf("send attempts = %d, want successful prefix, failed item and later repo", bot.sends)
	}
	for _, key := range []string{"o/a@", "o/b@"} {
		if got := state.GitHub.Repos[key].LastID; got != githubTestCommit(1).ID {
			t.Errorf("%s cursor = %q, want only delivered prefix", key, got)
		}
	}
}

func TestGitHubPermanentRejectionsConsumeBudget(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/a@": {LastID: githubTestCommit(0).ID}, "o/b@": {LastID: githubTestCommit(0).ID},
	}}}
	var visited []string
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		var commits []lookup.Commit
		for n := 11; n >= 0; n-- {
			commits = append(commits, githubTestCommit(n))
		}
		return commits, nil
	}
	bot := &fakeFeedBot{sendErr: &telegoapi.Error{ErrorCode: 400, Description: "message is too long"}}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if bot.sends != 10 || strings.Join(visited, ",") != "o/a" {
		t.Fatalf("permanent rejection budget: sends=%d visited=%v, want 10 and o/a", bot.sends, visited)
	}
	if state.GitHub.Repos["o/a@"].LastID != githubTestCommit(10).ID || state.GitHub.NextRepo != 1 {
		t.Fatalf("permanent rejection cursor or rotation = %+v", state.GitHub)
	}
}

func githubTestRepoPage(repo string, newest int) []lookup.Commit {
	commits := make([]lookup.Commit, 0, newest+1)
	for n := newest; n >= 0; n-- {
		commit := githubTestCommit(n)
		commit.URL = "https://github.com/" + repo + "/commit/" + commit.SHA
		commits = append(commits, commit)
	}
	return commits
}

func TestGitHubBudgetIsSharedAcrossRepositories(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubTestFeed()
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/a"}, {Repo: "o/b"}, {Repo: "o/c"}}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/a@": {LastID: githubTestCommit(0).ID},
		"o/b@": {LastID: githubTestCommit(0).ID},
		"o/c@": {LastID: githubTestCommit(0).ID},
	}}}
	var visited []string
	fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		visited = append(visited, repo)
		return githubTestRepoPage(repo, 6), nil
	}
	bot := &fakeFeedBot{}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if bot.sends != 10 || strings.Join(visited, ",") != "o/a,o/b" {
		t.Fatalf("shared budget: sends=%d visited=%v, want 6+4 across two repositories", bot.sends, visited)
	}
	if state.GitHub.Repos["o/a@"].LastID != githubTestCommit(6).ID ||
		state.GitHub.Repos["o/b@"].LastID != githubTestCommit(4).ID ||
		state.GitHub.Repos["o/c@"].LastID != githubTestCommit(0).ID || state.GitHub.NextRepo != 2 {
		t.Fatalf("shared-budget cursors or rotation = %+v", state.GitHub)
	}
}
