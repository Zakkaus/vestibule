package lookup

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const githubItemsFixture = `[
  {"number":42,"html_url":"https://github.com/owner/repo/issues/42","title":"Issue title","user":{"login":"alice"},"created_at":"2026-09-12T01:02:03Z","state":"open"},
  {"number":43,"html_url":"https://github.com/owner/repo/pull/43","title":"Pull\u0001 title","user":{"login":"bob"},"created_at":"2026-09-12T02:03:04Z","state":"closed","pull_request":{"merged_at":"2026-09-12T03:04:05Z"}}
]`

const githubNullItemsFixture = `null`

func TestRecentGitHubItemsUsesConfiguredURLAndHeaders(t *testing.T) {
	oldBase, oldAgent, oldToken, oldClient := githubAPIBase, userAgent, githubToken, httpClient
	t.Cleanup(func() { githubAPIBase, userAgent, githubToken, httpClient = oldBase, oldAgent, oldToken, oldClient })
	userAgent, githubToken = "github-items-test", "token-value"
	httpClient = &http.Client{Transport: githubItemsRoundTripper(func(r *http.Request) *http.Response {
		if got := r.URL.RequestURI(); got != "/root/repos/owner/repo/issues?state=all&sort=created&direction=desc&per_page=30" {
			t.Errorf("request URI = %q", got)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("Authorization") != "Bearer token-value" || r.Header.Get("User-Agent") != userAgent {
			t.Errorf("request headers = %v", r.Header)
		}
		w := httptest.NewRecorder()
		_, _ = w.Write([]byte(githubItemsFixture))
		return w.Result()
	})}
	configureGitHub(&settings.Config{GitHubAPIBase: "https://api.example.invalid/root///"})
	items, full, err := RecentGitHubItems(context.Background(), "owner/repo")
	if err != nil || full || len(items) != 2 {
		t.Fatalf("RecentGitHubItems() = %d items, full %v, error %v", len(items), full, err)
	}
}

func TestParseGitHubItemsFixture(t *testing.T) {
	items, full, err := parseGitHubItems([]byte(githubItemsFixture), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if full || len(items) != 2 || items[0].Number != 42 || items[0].IsPull || items[0].Author != "alice" || items[1].Number != 43 || !items[1].IsPull || items[1].Title != "Pull title" || items[1].MergedAt == nil {
		t.Fatalf("parsed items = %+v, full=%v", items, full)
	}
	longTitle := strings.Repeat("界", 201)
	truncated := strings.Replace(githubItemsFixture, "Issue title", longTitle, 1)
	items, _, err = parseGitHubItems([]byte(truncated), "owner/repo")
	if err != nil || len([]rune(items[0].Title)) != 200 {
		t.Fatalf("truncated title length = %d, error %v", len([]rune(items[0].Title)), err)
	}
}

func TestParseGitHubItemsRejectsNullRoot(t *testing.T) {
	if _, _, err := parseGitHubItems([]byte(githubNullItemsFixture), "owner/repo"); err == nil {
		t.Fatal("GitHub REST null root was accepted")
	}
}

func TestParseGitHubItemsAcceptsPullWithoutMergedAt(t *testing.T) {
	body := strings.Replace(githubItemsFixture, `"pull_request":{"merged_at":"2026-09-12T03:04:05Z"}`, `"pull_request":{}`, 1)
	items, _, err := parseGitHubItems([]byte(body), "owner/repo")
	if err != nil || len(items) != 2 || !items[1].IsPull || items[1].MergedAt != nil {
		t.Fatalf("pull without merged_at: items=%+v error=%v", items, err)
	}
}

func TestParseGitHubItemsAcceptsCaseInsensitiveRepo(t *testing.T) {
	items, _, err := parseGitHubItems([]byte(githubItemsFixture), "OWNER/REPO")
	if err != nil || len(items) != 2 {
		t.Fatalf("mixed-case repository: items=%+v error=%v", items, err)
	}
}

func TestParseGitHubItemsRejectsInvalidEntries(t *testing.T) {
	tests := []struct{ name, old, replacement string }{
		{name: "missing number", old: `"number":42`, replacement: `"other":42`},
		{name: "zero number", old: `"number":42`, replacement: `"number":0`},
		{name: "wrong URL", old: `https://github.com/owner/repo/issues/42`, replacement: `https://evil.invalid/owner/repo/issues/42`},
		{name: "empty title", old: `Issue title`, replacement: ``},
		{name: "empty login", old: `"login":"alice"`, replacement: `"login":""`},
		{name: "bad created_at", old: `2026-09-12T01:02:03Z`, replacement: `yesterday`},
		{name: "bad state", old: `"state":"open"`, replacement: `"state":"merged"`},
		{name: "bad merged_at", old: `2026-09-12T03:04:05Z`, replacement: `soon`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := strings.Replace(githubItemsFixture, test.old, test.replacement, 1)
			if _, _, err := parseGitHubItems([]byte(body), "owner/repo"); err == nil {
				t.Fatal("invalid GitHub REST page was accepted")
			}
		})
	}
}

func TestParseGitHubItemsReportsFullPage(t *testing.T) {
	entry := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(githubItemsFixture), "["), "]"))
	entry = strings.Split(entry, "},\n  {")[0] + "}"
	entries := make([]string, 30)
	for i := range entries {
		entries[i] = strings.Replace(entry, `"number":42`, `"number":`+strings.Repeat("1", i%3+1), 1)
	}
	_, full, err := parseGitHubItems([]byte("["+strings.Join(entries, ",")+"]"), "owner/repo")
	if err != nil || !full {
		t.Fatalf("full page = %v, error %v; want true", full, err)
	}
}

func TestRecentGitHubItemsRejectsOversizedResponse(t *testing.T) {
	oldBase, oldClient := githubAPIBase, httpClient
	t.Cleanup(func() { githubAPIBase, httpClient = oldBase, oldClient })
	httpClient = &http.Client{Transport: githubItemsRoundTripper(func(*http.Request) *http.Response {
		w := httptest.NewRecorder()
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxGitHubRESTBytes+1))
		return w.Result()
	})}
	githubAPIBase = "https://api.example.invalid"
	_, _, err := RecentGitHubItems(context.Background(), "owner/repo")
	if err == nil {
		t.Fatal("oversized GitHub REST response was accepted")
	}
}

type githubItemsRoundTripper func(*http.Request) *http.Response

func (roundTrip githubItemsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request), nil
}
