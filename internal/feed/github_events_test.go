package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
)

type githubEventBot struct {
	fakeFeedBot
	texts []string
	errs  []error
}

func (bot *githubEventBot) SendMessage(_ context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	bot.sends++
	bot.texts = append(bot.texts, params.Text)
	if len(bot.errs) >= bot.sends && bot.errs[bot.sends-1] != nil {
		return nil, bot.errs[bot.sends-1]
	}
	return &telego.Message{MessageID: bot.sends}, nil
}

func githubTestItem(number int, pull bool) lookup.GitHubItem {
	kind := "issues"
	if pull {
		kind = "pull"
	}
	return lookup.GitHubItem{Number: number, Title: fmt.Sprintf("item-%d", number), Author: "author", URL: fmt.Sprintf("https://github.com/o/r/%s/%d", kind, number), IsPull: pull, State: "open"}
}

func githubEventFeed(issues, pulls bool, repos ...string) *settings.FeedConfig {
	feed := &settings.FeedConfig{ChatID: -1009000002411, Lang: "en"}
	for _, name := range repos {
		feed.GitHubRepos = append(feed.GitHubRepos, settings.GitHubRepo{Repo: name, Issues: boolPointerFeed(issues), Pulls: boolPointerFeed(pulls)})
	}
	return feed
}

func boolPointerFeed(value bool) *bool { return &value }

func emptyCommitFetch(context.Context, string, string) ([]lookup.Commit, error) { return nil, nil }

func TestGitHubEventSwitchesAvoidRESTWhenDisabled(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(false, false, "o/r")
	calls := 0
	pollGitHubWithFetchers(context.Background(), &githubEventBot{}, feed, &feedState{}, emptyCommitFetch,
		func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
			calls++
			return nil, false, nil
		})
	if calls != 0 {
		t.Fatalf("GitHub REST calls = %d, want 0 while both switches are disabled", calls)
	}
	feed = githubEventFeed(true, true, "o/a", "o/b")
	pollGitHubWithFetchers(context.Background(), &githubEventBot{}, feed, &feedState{}, emptyCommitFetch,
		func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
			calls++
			return nil, false, nil
		})
	if calls != 2 {
		t.Fatalf("GitHub REST calls = %d, want one for each enabled repository", calls)
	}
}

func TestGitHubEventBaselinesAreIndependent(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, false, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: "commit"}}}}
	page := []lookup.GitHubItem{githubTestItem(4, true), githubTestItem(3, false)}
	fetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return page, false, nil }
	pollGitHubWithFetchers(context.Background(), &githubEventBot{}, feed, state, emptyCommitFetch, fetch)
	first := state.GitHub.Repos["o/r@"]
	if !first.IssuesInitialized || first.LastIssue != 3 || first.PullsInitialized {
		t.Fatalf("first baseline = %+v", first)
	}
	feed.GitHubRepos[0].Pulls = boolPointerFeed(true)
	page = []lookup.GitHubItem{githubTestItem(6, true), githubTestItem(5, false), githubTestItem(4, true), githubTestItem(3, false)}
	bot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
	second := state.GitHub.Repos["o/r@"]
	if bot.sends != 1 || second.LastIssue != 5 || !second.PullsInitialized || second.LastPull != 6 {
		t.Fatalf("second baseline = %+v, sends=%d; want one issue and pull baseline", second, bot.sends)
	}
}

func TestGitHubEventBaselineDoesNotBackfill(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, false, "o/r")
	state := &feedState{}
	page := []lookup.GitHubItem{githubTestItem(2, false), githubTestItem(1, false)}
	fetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return page, false, nil }
	bot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
	page = append([]lookup.GitHubItem{githubTestItem(3, false)}, page...)
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
	if bot.sends != 1 || state.GitHub.Repos["o/r@"].LastIssue != 3 {
		t.Fatalf("baseline/backfill sends=%d state=%+v", bot.sends, state.GitHub)
	}
}

func TestGitHubEventsDeliverNumbersAscending(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: "commit", LastIssue: 1, IssuesInitialized: true, LastPull: 2, PullsInitialized: true}}}}
	page := []lookup.GitHubItem{githubTestItem(7, true), githubTestItem(6, false), githubTestItem(5, true), githubTestItem(4, false)}
	bot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return page, false, nil })
	want := []string{"#4", "#6", "#5", "#7"}
	if len(bot.texts) != len(want) {
		t.Fatalf("messages = %v", bot.texts)
	}
	for index := range want {
		if !strings.Contains(bot.texts[index], want[index]) {
			t.Fatalf("message order = %v, want %v", bot.texts, want)
		}
	}
}

func TestGitHubStateChangesDoNotRedeliverCreationEvents(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: "commit", LastIssue: 10, IssuesInitialized: true, LastPull: 11, PullsInitialized: true}}}}
	issue, pull := githubTestItem(10, false), githubTestItem(11, true)
	issue.State, pull.State = "closed", "closed"
	merged := "2026-09-12T03:04:05Z"
	pull.MergedAt = &merged
	bot := &githubEventBot{}
	calls := 0
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		calls++
		return []lookup.GitHubItem{pull, issue}, false, nil
	})
	if calls != 1 || bot.sends != 0 {
		t.Fatalf("state-only changes: REST calls=%d sends=%d", calls, bot.sends)
	}
}

func TestGitHubIssueTruncationAdvancesCursorOnce(t *testing.T) {
	testGitHubAbsentCategoryTruncation(t, false)
}

func TestGitHubPullTruncationAdvancesCursorOnce(t *testing.T) {
	testGitHubAbsentCategoryTruncation(t, true)
}

func testGitHubAbsentCategoryTruncation(t *testing.T, pullCursor bool) {
	t.Helper()
	for _, test := range []struct {
		name       string
		pagePull   bool
		count      int
		newest     int
		wantCursor int
		wantWarn   int
	}{
		{name: "category present", pagePull: pullCursor, count: 30, newest: 60, wantCursor: 60, wantWarn: 1},
		{name: "category absent", pagePull: !pullCursor, count: 30, newest: 60, wantCursor: 60, wantWarn: 1},
		{name: "nonfull absent", pagePull: !pullCursor, count: 29, newest: 60, wantCursor: 1},
		{name: "overlapping absent", pagePull: !pullCursor, count: 30, newest: 30, wantCursor: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			disableGitHubTestPacing(t)
			feed := githubEventFeed(!pullCursor, pullCursor, "o/r")
			repoState := githubRepoState{LastID: "commit", LastIssue: 1, IssuesInitialized: true, LastPull: 1, PullsInitialized: true}
			state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": repoState}}}
			page := make([]lookup.GitHubItem, test.count)
			for index := range page {
				page[index] = githubTestItem(test.newest-index, test.pagePull)
			}
			var logs bytes.Buffer
			oldWriter := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(oldWriter) })
			fetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return page, test.count == 30, nil }
			bot := &githubEventBot{}
			pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
			pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, fetch)
			got := state.GitHub.Repos["o/r@"]
			cursor := got.LastIssue
			if pullCursor {
				cursor = got.LastPull
			}
			if cursor != test.wantCursor || bot.sends != 0 || strings.Count(logs.String(), "WARNING") != test.wantWarn {
				t.Fatalf("truncation cursor=%d sends=%d warnings=%d state=%+v", cursor, bot.sends, strings.Count(logs.String(), "WARNING"), got)
			}
		})
	}
}

func TestGitHubIssuesShareCommitBudget(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, false, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: githubTestCommit(0).ID, LastIssue: 0, IssuesInitialized: true}}}}
	commits := githubTestRepoPage("o/r", 8)
	items := []lookup.GitHubItem{githubTestItem(5, false), githubTestItem(4, false), githubTestItem(3, false), githubTestItem(2, false), githubTestItem(1, false)}
	bot := &githubEventBot{}
	commitFetch := func(context.Context, string, string) ([]lookup.Commit, error) { return commits, nil }
	itemFetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return items, false, nil }
	pollGitHubWithFetchers(context.Background(), bot, feed, state, commitFetch, itemFetch)
	if bot.sends != 10 || state.GitHub.Repos["o/r@"].LastIssue != 2 {
		t.Fatalf("first shared-budget round sends=%d state=%+v", bot.sends, state.GitHub)
	}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, commitFetch, itemFetch)
	if bot.sends != 13 || state.GitHub.Repos["o/r@"].LastIssue != 5 {
		t.Fatalf("second shared-budget round sends=%d state=%+v", bot.sends, state.GitHub)
	}
}

func TestGitHubEventBaselinesIgnoreExhaustedBudget(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: githubTestCommit(0).ID}}}}
	bot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), bot, feed, state,
		func(context.Context, string, string) ([]lookup.Commit, error) {
			return githubTestRepoPage("o/r", 10), nil
		},
		func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
			return []lookup.GitHubItem{githubTestItem(2, true), githubTestItem(1, false)}, false, nil
		})
	got := state.GitHub.Repos["o/r@"]
	if bot.sends != 10 || !got.IssuesInitialized || got.LastIssue != 1 || !got.PullsInitialized || got.LastPull != 2 {
		t.Fatalf("budget-exhausted baseline sends=%d state=%+v", bot.sends, got)
	}
}

func TestGitHubEventBudgetRotatesAcrossRepositories(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/a", "o/b")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/a@": {LastID: githubTestCommit(0).ID, IssuesInitialized: true, PullsInitialized: true},
		"o/b@": {LastID: githubTestCommit(0).ID, IssuesInitialized: true, PullsInitialized: true},
	}}}
	var visited []string
	commitFetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
		return githubTestRepoPage(repo, 6), nil
	}
	itemFetch := func(_ context.Context, repo string) ([]lookup.GitHubItem, bool, error) {
		visited = append(visited, repo)
		return []lookup.GitHubItem{githubTestItem(8, true), githubTestItem(7, true), githubTestItem(6, false), githubTestItem(5, false), githubTestItem(4, false), githubTestItem(3, false), githubTestItem(2, false), githubTestItem(1, false)}, false, nil
	}
	pollGitHubWithFetchers(context.Background(), &githubEventBot{}, feed, state, commitFetch, itemFetch)
	if strings.Join(visited, ",") != "o/a" || state.GitHub.NextRepo != 1 || state.GitHub.Repos["o/a@"].LastIssue != 4 || state.GitHub.Repos["o/a@"].LastPull != 0 {
		t.Fatalf("first rotation visited=%v state=%+v", visited, state.GitHub)
	}
	visited = nil
	pollGitHubWithFetchers(context.Background(), &githubEventBot{}, feed, state, commitFetch, itemFetch)
	if strings.Join(visited, ",") != "o/b" || state.GitHub.NextRepo != 0 {
		t.Fatalf("second rotation visited=%v state=%+v", visited, state.GitHub)
	}
}

func TestGitHubEventCursorsResumeAfterRestart(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/r")
	original := feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: "commit", LastIssue: 4, IssuesInitialized: true, LastPull: 5, PullsInitialized: true}}}}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var restarted feedState
	if err := json.Unmarshal(raw, &restarted); err != nil {
		t.Fatal(err)
	}
	bot := &githubEventBot{}
	page := []lookup.GitHubItem{githubTestItem(7, true), githubTestItem(6, false), githubTestItem(5, true), githubTestItem(4, false)}
	pollGitHubWithFetchers(context.Background(), bot, feed, &restarted, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) { return page, false, nil })
	got := restarted.GitHub.Repos["o/r@"]
	if bot.sends != 2 || got.LastIssue != 6 || got.LastPull != 7 {
		t.Fatalf("restart sends=%d state=%+v", bot.sends, got)
	}
}

func TestGitHubRESTFailureDoesNotBlockCommits(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, true, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: githubTestCommit(0).ID, LastIssue: 9, IssuesInitialized: true, LastPull: 10, PullsInitialized: true}}}}
	bot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, func(context.Context, string, string) ([]lookup.Commit, error) {
		return githubTestRepoPage("o/r", 1), nil
	}, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return nil, false, errors.New("REST unavailable")
	})
	got := state.GitHub.Repos["o/r@"]
	if bot.sends != 1 || got.LastID != githubTestCommit(1).ID || got.LastIssue != 9 || got.LastPull != 10 {
		t.Fatalf("REST isolation sends=%d state=%+v", bot.sends, got)
	}
}

func TestGitHubEventBaselinePersistsWhenCommitFetchFails(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, false, "o/r")
	state := &feedState{}
	bot := &githubEventBot{}
	page := []lookup.GitHubItem{githubTestItem(1, false)}
	commitFetch := func(context.Context, string, string) ([]lookup.Commit, error) {
		return nil, errors.New("Atom unavailable")
	}
	itemFetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return page, false, nil
	}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, commitFetch, itemFetch)
	if state.GitHub == nil || !state.GitHub.Repos["o/r@"].IssuesInitialized || state.GitHub.Repos["o/r@"].LastIssue != 1 {
		t.Fatalf("event baseline after Atom failure = %+v", state.GitHub)
	}
	page = append([]lookup.GitHubItem{githubTestItem(2, false)}, page...)
	pollGitHubWithFetchers(context.Background(), bot, feed, state, commitFetch, itemFetch)
	if bot.sends != 1 || state.GitHub.Repos["o/r@"].LastIssue != 2 {
		t.Fatalf("event delivery after Atom failure: sends=%d state=%+v", bot.sends, state.GitHub)
	}
	raw := marshalFeedState(t, state)
	restarted := unmarshalFeedState(t, raw)
	recoveredBot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), recoveredBot, feed, restarted,
		func(context.Context, string, string) ([]lookup.Commit, error) {
			return githubTestRepoPage("o/r", 2), nil
		}, itemFetch)
	if recoveredBot.sends != 0 || restarted.GitHub.Repos["o/r@"].LastID != githubTestCommit(2).ID {
		t.Fatalf("Atom recovery sent %d historical commits; state=%+v", recoveredBot.sends, restarted.GitHub)
	}
	emptyRestarted := unmarshalFeedState(t, raw)
	emptyBot := &githubEventBot{}
	pollGitHubWithFetchers(context.Background(), emptyBot, feed, emptyRestarted, emptyCommitFetch, itemFetch)
	if emptyRestarted.GitHub.Repos["o/r@"].CommitBaselinePending {
		t.Fatalf("valid empty Atom page left commit baseline pending: state=%+v", emptyRestarted.GitHub)
	}
	pollGitHubWithFetchers(context.Background(), emptyBot, feed, emptyRestarted,
		func(context.Context, string, string) ([]lookup.Commit, error) {
			return githubTestRepoPage("o/r", 0), nil
		}, itemFetch)
	if emptyBot.sends != 1 || emptyRestarted.GitHub.Repos["o/r@"].LastID != githubTestCommit(0).ID {
		t.Fatalf("first commit after empty Atom baseline: sends=%d state=%+v", emptyBot.sends, emptyRestarted.GitHub)
	}
}

// A repository state written before the event cursors existed still delivers commits.
func TestGitHubLegacyStateStillDeliversCommits(t *testing.T) {
	disableGitHubTestPacing(t)
	legacyFeed := githubEventFeed(false, false, "o/r")
	legacyState := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {}}}}
	legacyBot := &githubEventBot{}
	itemFetch := func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return nil, false, nil
	}
	pollGitHubWithFetchers(context.Background(), legacyBot, legacyFeed, legacyState,
		func(context.Context, string, string) ([]lookup.Commit, error) {
			return githubTestRepoPage("o/r", 2), nil
		}, itemFetch)
	if legacyBot.sends != 3 || legacyState.GitHub.Repos["o/r@"].LastID != githubTestCommit(2).ID {
		t.Fatalf("legacy empty Atom state: sends=%d state=%+v", legacyBot.sends, legacyState.GitHub)
	}
}

func marshalFeedState(t *testing.T, state *feedState) []byte {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func unmarshalFeedState(t *testing.T, raw []byte) *feedState {
	t.Helper()
	var state feedState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return &state
}

func TestGitHubTransientIssueSendPreservesCursor(t *testing.T) {
	testGitHubTransientEventCursor(t, false)
}

func TestGitHubTransientPullSendPreservesCursor(t *testing.T) {
	testGitHubTransientEventCursor(t, true)
}

func TestGitHubPermanentEventRejectionAdvancesCursor(t *testing.T) {
	disableGitHubTestPacing(t)
	feed := githubEventFeed(true, false, "o/r")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": {LastID: "commit", LastIssue: 1, IssuesInitialized: true}}}}
	bot := &githubEventBot{errs: []error{&telegoapi.Error{ErrorCode: 400, Description: "message is too long"}}}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return []lookup.GitHubItem{githubTestItem(2, false)}, false, nil
	})
	if bot.sends != 1 || state.GitHub.Repos["o/r@"].LastIssue != 2 {
		t.Fatalf("permanent rejection attempts=%d state=%+v", bot.sends, state.GitHub)
	}
}

func testGitHubTransientEventCursor(t *testing.T, pull bool) {
	t.Helper()
	disableGitHubTestPacing(t)
	feed := githubEventFeed(!pull, pull, "o/r")
	repoState := githubRepoState{LastID: "commit", LastIssue: 1, IssuesInitialized: true, LastPull: 1, PullsInitialized: true}
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{"o/r@": repoState}}}
	bot := &githubEventBot{errs: []error{errors.New("temporary transport failure")}}
	pollGitHubWithFetchers(context.Background(), bot, feed, state, emptyCommitFetch, func(context.Context, string) ([]lookup.GitHubItem, bool, error) {
		return []lookup.GitHubItem{githubTestItem(2, pull)}, false, nil
	})
	got := state.GitHub.Repos["o/r@"]
	cursor := got.LastIssue
	if pull {
		cursor = got.LastPull
	}
	if bot.sends != 1 || cursor != 1 {
		t.Fatalf("transient event attempts=%d cursor=%d, want 1 and 1", bot.sends, cursor)
	}
}

func TestRenderGitHubOpenedEventsEscapesFields(t *testing.T) {
	item := lookup.GitHubItem{Number: 7, URL: `https://github.com/o/r/issues/7?x="q"&a=b`, Title: `Title & <tag>`, Author: `A & <B>`}
	text := renderGitHubItem(item, `o/&lt;r&gt;`, feedLanguage("en"))
	for _, want := range []string{`#7`, `x=&#34;q&#34;&amp;a=b`, `Title &amp; &lt;tag&gt;`, `A &amp; &lt;B&gt;`, `o/&amp;lt;r&amp;gt;`} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered issue missing %q: %s", want, text)
		}
	}
	item.IsPull = true
	if pull := renderGitHubItem(item, "o/r", feedLanguage("en")); !strings.Contains(pull, "Pull request") {
		t.Fatalf("rendered pull request = %q", pull)
	}
}
