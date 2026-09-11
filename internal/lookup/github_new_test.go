package lookup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestNewConfiguresGitHubRequests(t *testing.T) {
	oldUserAgent, oldOverlays := userAgent, overlays
	oldNewsURL, oldNewsBase := newsURL, newsBase
	oldGitHubToken, oldGitHubAtomBase, oldGitHubAPIBase := githubToken, githubAtomBase, githubAPIBase
	t.Cleanup(func() {
		userAgent, overlays = oldUserAgent, oldOverlays
		newsURL, newsBase = oldNewsURL, oldNewsBase
		githubToken, githubAtomBase, githubAPIBase = oldGitHubToken, oldGitHubAtomBase, oldGitHubAPIBase
	})

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		serveConfiguredGitHubRequest(t, w, r)
	}))
	defer server.Close()
	New(nil, nil, &settings.Config{
		UserAgent:      "new-test-agent",
		Overlays:       []settings.OverlayCfg{{Name: "test", Repo: "owner/repo", Branch: "feature"}},
		NewsURL:        server.URL + "/news",
		GitHubAtomBase: server.URL + "/atom///",
		GitHubAPIBase:  server.URL + "/api///",
	}, "must-not-be-sent")
	if hits != 0 {
		t.Fatal("lookup construction unexpectedly made a network request")
	}
	commits, err := RecentCommits(context.Background(), "owner/repo", "feature/keep#x")
	if err != nil || len(commits) != 0 || hits != 1 {
		t.Fatalf("configured request: commits=%v error=%v hits=%d", commits, err, hits)
	}
	items, full, err := RecentGitHubItems(context.Background(), "owner/repo")
	if err != nil || len(items) != 0 || full || hits != 2 {
		t.Fatalf("configured REST request: items=%v full=%v error=%v hits=%d", items, full, err, hits)
	}
}

// serveConfiguredGitHubRequest answers the Atom and REST paths the configured lookup
// must reach, checking the headers each carries.
func serveConfiguredGitHubRequest(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.EscapedPath() {
	case "/atom/owner/repo/commits/feature/keep%23x.atom":
		if r.Header.Get("User-Agent") != "new-test-agent" || r.Header.Get("Authorization") != "" {
			t.Errorf("GitHub Atom request headers = %v", r.Header)
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"/>`))
	case "/api/repos/owner/repo/issues":
		if r.Header.Get("User-Agent") != "new-test-agent" ||
			r.Header.Get("Accept") != "application/vnd.github+json" ||
			r.Header.Get("Authorization") != "Bearer must-not-be-sent" {
			t.Errorf("GitHub REST request headers = %v", r.Header)
		}
		_, _ = w.Write([]byte(`[]`))
	default:
		t.Errorf("unexpected GitHub request path %q", r.URL.EscapedPath())
	}
}
