package lookup

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
)

// withDebianReleaseRoles installs live-looking Debian release roles and marks them fresh, so the
// handler under test uses them instead of reaching for distro-info-data.
func withDebianReleaseRoles(t *testing.T, roles map[string]string) {
	t.Helper()
	relInfo.mu.Lock()
	oldDebian, oldSeries := relInfo.debian, relInfo.debianSer
	oldFetched, oldRefreshing := relInfo.fetched, relInfo.refreshing
	relInfo.debian = roles
	relInfo.debianSer = map[string]bool{}
	relInfo.fetched = time.Now()
	relInfo.refreshing = false
	relInfo.mu.Unlock()
	t.Cleanup(func() {
		relInfo.mu.Lock()
		relInfo.debian, relInfo.debianSer = oldDebian, oldSeries
		relInfo.fetched, relInfo.refreshing = oldFetched, oldRefreshing
		relInfo.mu.Unlock()
	})
}

// pkgsAnswer runs /pkgs for one query against a fixed set of upstream answers and returns the
// text the group would see.
func pkgsAnswer(t *testing.T, query string, upstream func(*http.Request) (int, string)) string {
	t.Helper()
	resetLookupPackageCaches(t)
	withLookupHTTP(t, upstream)
	caller := &lookupTelegramCaller{}
	bot := newLookupTestBot(t, caller)
	service := New(nil, telegram.NewConnector(bot), &settings.Config{}, "")
	runLookupHandler(t, bot, service.OnPkgs, lookupMessage("/pkgs "+query, 77, 77, telego.ChatTypePrivate))
	if got := len(caller.methodCalls("sendMessage")); got != 1 {
		t.Fatalf("sendMessage calls = %d, want one answer", got)
	}
	return sentLookupMessage(t, caller, 0).Text
}

// The Gentoo row may carry the authoritative amd64/~amd64 keyword versions only when the search
// hit is the project that was asked about. Repology answers a fuzzy query with a project name of
// its own, and the first Gentoo search hit for that name is frequently a different package; used
// anyway, /pkgs states a Gentoo version for a package the reader never named.
func TestTheGentooKeywordLineIsUsedOnlyForTheProjectThatWasAsked(t *testing.T) {
	upstream := func(request *http.Request) (int, string) {
		switch {
		case strings.Contains(request.URL.Path, "/project/"):
			return http.StatusOK, `[{"repo":"gentoo","version":"0.9"}]`
		case strings.HasSuffix(request.URL.Path, "/packages/search"):
			return http.StatusOK, `<a href="/packages/app-emulation/libvirt">libvirt</a>`
		case strings.HasSuffix(request.URL.Path, ".json"):
			return http.StatusOK, `{"versions":[{"version":"11.5.0","keywords":["amd64"]}]}`
		}
		return http.StatusNotFound, ""
	}

	// "vmi" is a fuzzy hit: the Gentoo search answers with app-emulation/libvirt, whose name is
	// not the project. Its keyword versions must not become the answer.
	answer := pkgsAnswer(t, "vmi", upstream)
	if !strings.Contains(answer, "0.9") {
		t.Errorf("/pkgs vmi = %q, want the Gentoo version Repology reports for the project", answer)
	}
	if strings.Contains(answer, "11.5.0") || strings.Contains(answer, "Gentoo amd64") {
		t.Errorf("/pkgs vmi = %q: it states the keyword versions of app-emulation/libvirt, a package the reader never asked about", answer)
	}

	// Positive control: when the search hit is the project, the same path does use the
	// authoritative keyword versions.
	answer = pkgsAnswer(t, "libvirt", upstream)
	if !strings.Contains(answer, "11.5.0") || !strings.Contains(answer, "Gentoo amd64") {
		t.Errorf("/pkgs libvirt = %q, want the authoritative amd64 keyword version", answer)
	}
}

// Debian numbers its testing series above stable, so the highest-numbered Debian row is not the
// version anybody can install. Counting testing as stable makes /pkgs hand a member a version
// that apt will not find on their machine.
func TestDebianTestingIsNotOfferedAsTheDebianVersion(t *testing.T) {
	upstream := func(request *http.Request) (int, string) {
		switch {
		case strings.Contains(request.URL.Path, "/project/"):
			return http.StatusOK, `[{"repo":"debian_13","version":"8.4"},{"repo":"debian_14","version":"9.0"}]`
		case strings.HasSuffix(request.URL.Path, "/packages/search"):
			return http.StatusOK, "<html></html>"
		}
		return http.StatusNotFound, ""
	}

	withDebianReleaseRoles(t, map[string]string{"13": "stable", "14": "testing"})
	answer := pkgsAnswer(t, "nano", upstream)
	if !strings.Contains(answer, "8.4") || !strings.Contains(answer, "13") {
		t.Errorf("/pkgs nano = %q, want the version in Debian 13, the released stable series", answer)
	}
	if strings.Contains(answer, "9.0") {
		t.Errorf("/pkgs nano = %q: it offers the version from Debian testing, which is not installable from stable", answer)
	}

	// Positive control: the exclusion comes from the live release roles, not from a blanket rule
	// about higher-numbered series. With no series marked testing, the higher one is shown.
	withDebianReleaseRoles(t, map[string]string{"13": "stable"})
	answer = pkgsAnswer(t, "nano", upstream)
	if !strings.Contains(answer, "9.0") {
		t.Errorf("/pkgs nano with no testing series = %q, want the newest series shown", answer)
	}
}
