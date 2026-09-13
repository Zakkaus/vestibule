package feed

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func postTrackedGitHubPull(t *testing.T, bot *githubEventBot, pulls ...lookup.GitHubItem) (*feedState, *settings.FeedConfig) {
	t.Helper()
	feed := githubEventFeed(false, true, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/r@": {LastID: "commit", LastPull: 1, PullsInitialized: true},
	}}}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return pulls, false, nil
	})
	if bot.sends != len(pulls) {
		t.Fatalf("posted messages = %d, want %d", bot.sends, len(pulls))
	}
	return state, feed
}

func pollGitHubPullState(bot *githubEventBot, feed *settings.FeedConfig, state *feedState, pulls ...lookup.GitHubItem) {
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return pulls, false, nil
	})
}

func TestGitHubPullMergeEditsThePostedMessage(t *testing.T) {
	disableGitHubTestPacing(t)
	bot := &githubEventBot{}
	pull := githubTestItem(2, true)
	state, feed := postTrackedGitHubPull(t, bot, pull)
	merged := "2026-09-14T01:19:16Z"
	pull.State, pull.MergedAt = "closed", &merged
	pollGitHubPullState(bot, feed, state, pull)
	if len(bot.editTexts) != 1 || !strings.Contains(bot.editTexts[0], "Merged") {
		t.Fatalf("merge edits = %q, want one message containing Merged", bot.editTexts)
	}
}

func TestGitHubBornMergedItemRendersItsState(t *testing.T) {
	merged := "2026-09-14T01:19:16Z"
	item := githubTestItem(2, true)
	item.State, item.MergedAt = "closed", &merged
	text := renderGitHubItem(item, "o/r", feedLanguage("en"))
	if strings.Contains(text, "opened") || !strings.Contains(text, "🟣") || !strings.Contains(text, "Merged") {
		t.Fatalf("born-merged pull text = %q", text)
	}
}

func TestGitHubReopenedItemEditsOnceAndStoresOpen(t *testing.T) {
	disableGitHubTestPacing(t)
	bot := &githubEventBot{}
	pull := githubTestItem(2, true)
	state, feed := postTrackedGitHubPull(t, bot, pull)
	pull.State = "closed"
	pollGitHubPullState(bot, feed, state, pull)
	pull.State = "open"
	pollGitHubPullState(bot, feed, state, pull)
	pollGitHubPullState(bot, feed, state, pull)
	if len(bot.editTexts) != 2 || !strings.Contains(bot.editTexts[1], "Reopened") {
		t.Fatalf("reopen edits = %q, want one closure and one reopen edit", bot.editTexts)
	}
	if tracked := state.GitHub.Repos["o/r@"].Tracked[pull.Number]; tracked.State != githubItemOpen {
		t.Fatalf("reopened state = %q, want %q", tracked.State, githubItemOpen)
	}
}

func TestGitHubEditFailuresFollowTelegramClassification(t *testing.T) {
	t.Run("not modified advances the stored state", func(t *testing.T) {
		disableGitHubTestPacing(t)
		bot := &githubEventBot{editErrors: []error{errors.New("message is not modified")}}
		pull := githubTestItem(2, true)
		state, feed := postTrackedGitHubPull(t, bot, pull)
		pull.State = "closed"
		pollGitHubPullState(bot, feed, state, pull)
		pollGitHubPullState(bot, feed, state, pull)
		if bot.edits != 1 {
			t.Fatalf("not-modified edits = %d, want 1", bot.edits)
		}
	})
	t.Run("uneditable message stops tracking", func(t *testing.T) {
		disableGitHubTestPacing(t)
		bot := &githubEventBot{editErrors: []error{errors.New("message to edit not found")}}
		pull := githubTestItem(2, true)
		state, feed := postTrackedGitHubPull(t, bot, pull)
		pull.State = "closed"
		pollGitHubPullState(bot, feed, state, pull)
		pollGitHubPullState(bot, feed, state, pull)
		if bot.edits != 1 {
			t.Fatalf("uneditable edits = %d, want 1", bot.edits)
		}
	})
	t.Run("countable bad request reaches the failure limit", func(t *testing.T) {
		disableGitHubTestPacing(t)
		bot := &githubEventBot{editErrors: make([]error, maxEditFails)}
		for i := range bot.editErrors {
			bot.editErrors[i] = errors.New("Bad Request: can't parse entities")
		}
		pull := githubTestItem(2, true)
		state, feed := postTrackedGitHubPull(t, bot, pull)
		pull.State = "closed"
		for range maxEditFails + 1 {
			pollGitHubPullState(bot, feed, state, pull)
		}
		if bot.edits != maxEditFails {
			t.Fatalf("countable edits = %d, want %d", bot.edits, maxEditFails)
		}
	})
	t.Run("rate limit ends this edit cycle", func(t *testing.T) {
		disableGitHubTestPacing(t)
		bot := &githubEventBot{editErrors: []error{errors.New("Too Many Requests: retry after 5")}}
		first, second := githubTestItem(2, true), githubTestItem(3, true)
		state, feed := postTrackedGitHubPull(t, bot, first, second)
		first.State, second.State = "closed", "closed"
		pollGitHubPullState(bot, feed, state, first, second)
		if bot.edits != 1 {
			t.Fatalf("rate-limited edits = %d, want 1", bot.edits)
		}
	})
}

func TestGitHubTrackedItemsSurviveStateRoundTrip(t *testing.T) {
	const source = `{"github":{"repos":{"o/r@":{"last_id":"cursor","tracked":{"7":{"msg_id":42,"state":"merged","edit_fails":3}}}}}}`
	var state feedState
	if err := json.Unmarshal([]byte(source), &state); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	github, _ := document["github"].(map[string]any)
	repos, _ := github["repos"].(map[string]any)
	repo, _ := repos["o/r@"].(map[string]any)
	tracked, _ := repo["tracked"].(map[string]any)
	record, _ := tracked["7"].(map[string]any)
	if record["msg_id"] != float64(42) || record["state"] != "merged" || record["edit_fails"] != float64(3) {
		t.Fatalf("tracked GitHub record after round trip = %#v", record)
	}
}

func TestGitHubStateWithoutTrackedLoads(t *testing.T) {
	var state feedState
	if err := json.Unmarshal([]byte(`{"github":{"repos":{"o/r@":{"last_id":"cursor"}}}}`), &state); err != nil {
		t.Fatal(err)
	}
	if state.GitHub == nil || state.GitHub.Repos["o/r@"].LastID != "cursor" {
		t.Fatalf("legacy GitHub state = %#v", state.GitHub)
	}
}

func TestGitHubEditBudgetIsSharedWithBugzilla(t *testing.T) {
	disableGitHubTestPacing(t)
	now := time.Now()
	feed := githubEventFeed(false, true, "o/r")
	off := false
	feed.Bugs, feed.News = &off, &off
	state := &feedState{
		Tracked: map[string]*trackedBug{},
		GitHub: &githubState{Repos: map[string]githubRepoState{
			"o/r@": {LastID: "commit", Tracked: map[int]trackedGitHubItem{99: {MsgID: 99, State: githubItemOpen}}},
		}},
	}
	bugs := make([]recentBug, maxEditsPerCycle)
	for i := range bugs {
		id := i + 1
		state.Tracked[strconv.Itoa(id)] = &trackedBug{MsgID: id, State: "CONFIRMED|"}
		bugs[i] = recentBug{ID: id, Status: "IN_PROGRESS"}
	}
	item := githubTestItem(99, true)
	item.State = githubItemClosed
	sources := feedSources{
		recent:        func(context.Context, int) ([]recentBug, bool) { return nil, true },
		tracked:       func(context.Context, []int) ([]recentBug, bool) { return bugs, true },
		githubCommits: emptyCommitFetch,
		githubItems: func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
			return []lookup.GitHubItem{item}, false, nil
		},
	}
	bot := &githubEventBot{}
	states := map[int64]*feedState{feed.ChatID: state}
	next := map[int64]time.Time{feed.ChatID: now}
	pollAllWithSources(context.Background(), bot, []*settings.FeedConfig{feed}, states, "", now, next, sources)
	if bot.edits != maxEditsPerCycle || len(bot.editTexts) == 0 || !strings.Contains(bot.editTexts[0], "Closed") {
		t.Fatalf("first cycle edits = %d, texts = %q; want the GitHub edit plus %d Bugzilla edits", bot.edits, bot.editTexts, maxEditsPerCycle-1)
	}
	pollAllWithSources(context.Background(), bot, []*settings.FeedConfig{feed}, states, "", now.Add(time.Hour), next, sources)
	if bot.edits != maxEditsPerCycle+1 {
		t.Fatalf("two-cycle edits = %d, want %d shared-budget edits", bot.edits, maxEditsPerCycle+1)
	}
}

func TestGitHubTrackingEvictsTerminalsFirst(t *testing.T) {
	state := githubRepoState{Tracked: make(map[int]trackedGitHubItem, maxGitHubTracked)}
	for number := 1; number <= maxGitHubTracked; number++ {
		state.Tracked[number] = trackedGitHubItem{MsgID: number, State: githubItemOpen}
	}
	state.Tracked[1] = trackedGitHubItem{MsgID: 1, State: githubItemMerged}
	item := githubTestItem(maxGitHubTracked+1, true)
	state.trackGitHubItem(item, maxGitHubTracked+1)
	_, retained := state.Tracked[1]
	if len(state.Tracked) != maxGitHubTracked || retained || state.Tracked[item.Number].State != githubItemOpen {
		t.Fatalf("tracked GitHub items = %#v", state.Tracked)
	}
}
