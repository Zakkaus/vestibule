package feed

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestGitHubStateNullAndPruning(t *testing.T) {
	var st feedState
	if err := json.Unmarshal([]byte(`{"github":{"next_repo":-1,"repos":{"o/r@":null,"o/keep@":{"last_id":"cursor"}}}}`), &st); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.GitHub.Repos["o/r@"]; ok {
		t.Fatal("null repository entry was retained")
	}
	if st.GitHub.NextRepo != -1 {
		t.Fatalf("raw next_repo changed before normalization: %d", st.GitHub.NextRepo)
	}
	raw, err := json.Marshal(feedState{GitHub: &githubState{Repos: map[string]githubRepoState{}}})
	if err != nil || strings.Contains(string(raw), `"github"`) {
		t.Fatalf("empty GitHub table serialized: %s (err=%v)", raw, err)
	}
	feed := &settings.FeedConfig{ChatID: -1009000002401, GitHubRepos: []settings.GitHubRepo{{Repo: "o/keep"}}}
	pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, &st, func(context.Context, string, string) ([]lookup.Commit, error) {
		return []lookup.Commit{}, nil
	})
	if st.GitHub.NextRepo != 0 || len(st.GitHub.Repos) != 1 {
		t.Fatalf("normalized state = %+v", st.GitHub)
	}
	feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/new"}, {Repo: "o/keep"}}
	st.GitHub.NextRepo = 99
	pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, &st, func(context.Context, string, string) ([]lookup.Commit, error) {
		return []lookup.Commit{}, nil
	})
	if st.GitHub.NextRepo != 1 || st.GitHub.Repos["o/keep@"].LastID != "cursor" {
		t.Fatalf("reordered state = %+v", st.GitHub)
	}
	st = feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/keep@": {}, "o/removed@": {LastID: "gone"},
	}}}
	pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, &st, func(context.Context, string, string) ([]lookup.Commit, error) {
		return nil, nil
	})
	if _, ok := st.GitHub.Repos["o/removed@"]; ok {
		t.Fatal("removed repository survived pruning")
	}

	feed.GitHubRepos = nil
	pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, &st, func(context.Context, string, string) ([]lookup.Commit, error) {
		t.Fatal("fetch called for empty repository list")
		return nil, nil
	})
	if st.GitHub != nil {
		t.Fatal("empty repository list retained GitHub state")
	}
}

func TestGitHubStateRestartWriteFailureAndRollbackShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.json")
	original := feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
		"o/r@": {LastID: "cursor"},
	}}}
	saveFeedState(path, original)
	reloaded := loadFeedState(path)
	if reloaded.GitHub == nil || reloaded.GitHub.Repos["o/r@"].LastID != "cursor" {
		t.Fatalf("reloaded state = %+v", reloaded.GitHub)
	}
	invalid := filepath.Join(t.TempDir(), "missing", "feed.json")
	saveFeedState(invalid, original)
	if _, err := os.Stat(invalid); !os.IsNotExist(err) {
		t.Fatalf("write failure unexpectedly created state: %v", err)
	}
	legacy := filepath.Join(t.TempDir(), "legacy.json")
	if err := os.WriteFile(legacy, []byte(`{"last_bug_id":4,"last_news_url":"u"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyState := loadFeedState(legacy)
	saveFeedState(legacy, legacyState)
	raw, err := os.ReadFile(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"github"`) {
		t.Fatalf("legacy state acquired GitHub field after rewrite: %s", raw)
	}
}

func TestRenderGitHubCommitEscapesEachFieldOnce(t *testing.T) {
	commit := lookup.Commit{
		URL:      `https://git.example/a&b/r/commit/abc?x="q"`,
		ShortSHA: "abcdefg",
		Title:    "Document &lt;tag&gt; literally",
		Author:   "Author & <name>",
	}
	text := renderGitHubCommit(commit, "owner/repo", "feature/&lt;topic&gt;", feedLanguage("en"))
	for _, want := range []string{`a&amp;b`, `&#34;q&#34;`, `Document &amp;lt;tag&amp;gt; literally`, `Author &amp; &lt;name&gt;`, `feature/&amp;lt;topic&amp;gt;`, "abcdefg", "owner/repo"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered commit missing %q: %s", want, text)
		}
	}
	withoutAuthor := commit
	withoutAuthor.Author = "  \t"
	if got := renderGitHubCommit(withoutAuthor, "owner/repo", "", feedLanguage("en")); strings.Contains(got, "Author") || strings.Contains(got, "Branch") {
		t.Errorf("optional fields rendered unexpectedly: %s", got)
	}
}

func TestGitHubNullStatesRetainLegacyJSON(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"github":null}`, `{"github":{}}`,
		`{"github":{"repos":null}}`, `{"github":{"repos":{"o/r@":null}}}`,
	} {
		var state feedState
		if err := json.Unmarshal([]byte(raw), &state); err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != `{"last_bug_id":0,"last_news_url":""}` {
			t.Errorf("%s rewrites as %s, want unchanged legacy shape", raw, got)
		}
	}
}

func TestGitHubReadditionDependsOnPersistedPruning(t *testing.T) {
	disableGitHubTestPacing(t)
	for _, successfulSave := range []bool{true, false} {
		path := filepath.Join(t.TempDir(), "feed.json")
		original := feedState{GitHub: &githubState{Repos: map[string]githubRepoState{
			"o/r@":    {LastID: githubTestCommit(0).ID},
			"o/keep@": {LastID: githubTestCommit(0).ID},
		}}}
		saveFeedState(path, original)
		current := loadFeedState(path)
		feed := githubTestFeed()
		feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/keep"}}
		empty := func(context.Context, string, string) ([]lookup.Commit, error) { return nil, nil }
		pollGitHubWithFetcher(context.Background(), &fakeFeedBot{}, feed, &current, empty)
		target := path
		if !successfulSave {
			// An invalid state directory forces a real write error even when tests run as root.
			target = filepath.Join(path, "feed.json")
		}
		saveFeedState(target, current)
		restarted := loadFeedState(path)
		feed.GitHubRepos = []settings.GitHubRepo{{Repo: "o/r"}}
		fetch := func(_ context.Context, repo, _ string) ([]lookup.Commit, error) {
			return githubTestRepoPage(repo, 1), nil
		}
		bot := &fakeFeedBot{}
		pollGitHubWithFetcher(context.Background(), bot, feed, &restarted, fetch)
		want := 0
		if !successfulSave {
			want = 1
		}
		if bot.sends != want {
			t.Errorf("successful prune save=%v: re-add sent %d, want %d", successfulSave, bot.sends, want)
		}
	}
}
