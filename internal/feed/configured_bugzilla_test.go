package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfiguredBugzillaBaseDrivesFetchAndRenderedLinks(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bugs":[{"id":42,"summary":"configured source","status":"CONFIRMED"}]}`))
	}))
	defer server.Close()

	bugs, ok := fetchRecentBugs(context.Background(), server.URL, 0)
	if !ok || len(bugs) != 1 || requestedPath != "/rest/bug" {
		t.Fatalf("configured Bugzilla fetch = bugs %#v, ok %v, path %q", bugs, ok, requestedPath)
	}
	rendered := formatBug(server.URL, recentBug{
		ID:         42,
		Summary:    "configured source",
		Status:     "CONFIRMED",
		AssignedTo: bugUser{Name: "owner", RealName: "Owner"},
	}, feedLanguage("en"))
	for _, want := range []string{server.URL + "/42", server.URL + "/buglist.cgi?"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered Bugzilla item missing configured base %q: %s", want, rendered)
		}
	}
	if got := confirmNotice(server.URL, bugs[0], feedLanguage("en")); !strings.Contains(got, server.URL+"/42") {
		t.Errorf("confirmation notice missing configured base: %s", got)
	}
}
