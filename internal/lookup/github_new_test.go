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
	oldGitHubToken, oldGitHubBase := githubToken, githubAtomBase
	t.Cleanup(func() {
		userAgent, overlays = oldUserAgent, oldOverlays
		newsURL, newsBase = oldNewsURL, oldNewsBase
		githubToken, githubAtomBase = oldGitHubToken, oldGitHubBase
	})

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.EscapedPath() != "/atom/owner/repo/commits/feature/keep%23x.atom" {
			t.Errorf("unexpected GitHub request path %q", r.URL.EscapedPath())
		}
		if r.Header.Get("User-Agent") != "new-test-agent" || r.Header.Get("Authorization") != "" {
			t.Errorf("GitHub request headers = %v", r.Header)
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"/>`))
	}))
	defer server.Close()
	New(nil, nil, &settings.Config{
		UserAgent:      "new-test-agent",
		Overlays:       []settings.OverlayCfg{{Name: "test", Repo: "owner/repo", Branch: "feature"}},
		NewsURL:        server.URL + "/news",
		GitHubAtomBase: server.URL + "/atom///",
	}, "must-not-be-sent")
	if hits != 0 {
		t.Fatal("lookup construction unexpectedly made a network request")
	}
	commits, err := RecentCommits(context.Background(), "owner/repo", "feature/keep#x")
	if err != nil || len(commits) != 0 || hits != 1 {
		t.Fatalf("configured request: commits=%v error=%v hits=%d", commits, err, hits)
	}
}
