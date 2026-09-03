package lookup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestVerLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool // want verLess(a,b)
	}{
		{"1.0-r2", "1.0-r10", true},   // r2 < r10  (regression guard: was wrongly false)
		{"1.0-r10", "1.0-r2", false},  // r10 is NOT older than r2
		{"1.0_p2", "1.0_p10", true},   // patch level, double digit
		{"1.0_rc9", "1.0_rc11", true}, // release candidate, double digit
		{"1.2", "1.10", true},         // plain numeric dotted parts
		{"1.10", "1.2", false},
		{"1.0", "1.0-r1", true}, // a revision is newer than the bare version
		{"2.0", "2.0", false},   // equal is not "less"
		{"9.1.1652", "9.2.0670", true},
		{"1.0.0", "1.0.0.0", true}, // more tokens (all-equal prefix) is newer
		// Gentoo suffix ordering: _alpha < _beta < _pre < _rc < (release) < _p, and -rN newer.
		{"1.0_rc1", "1.0", true},      // a release candidate is OLDER than the release
		{"1.0", "1.0_rc1", false},     // ...and the release is newer
		{"1.0_alpha1", "1.0", true},   // alpha is older
		{"1.0_beta", "1.0_rc1", true}, // beta < rc
		{"1.0_p1", "1.0", false},      // a patch level is NEWER than the release
		{"1.0", "1.0_p1", true},
		{"1.0_rc1", "1.0_rc2", true}, // rc1 < rc2
		{"1.0_p1", "1.0_rc9", false}, // a patch release is newer than any release candidate
		{"1.0_rc9", "1.0_p1", true},
		{"1.0_p1", "1.0_pre99", false}, // a patch release is newer than any pre-release
		{"1.0_pre99", "1.0_p1", true},
	}
	for _, c := range cases {
		if got := verLess(c.a, c.b); got != c.want {
			t.Errorf("verLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}

	ordered := []string{
		"1.0_alpha9",
		"1.0_beta9",
		"1.0_pre9",
		"1.0_rc9",
		"1.0",
		"1.0_p1",
		"1.0_p1-r1",
	}
	for older := range ordered {
		for newer := older + 1; newer < len(ordered); newer++ {
			if !verLess(ordered[older], ordered[newer]) {
				t.Errorf("documented Gentoo order: verLess(%q, %q) = false, want true", ordered[older], ordered[newer])
			}
			if verLess(ordered[newer], ordered[older]) {
				t.Errorf("documented Gentoo order: verLess(%q, %q) = true, want false", ordered[newer], ordered[older])
			}
		}
	}
}

func TestCommandArg(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"/pkg vim", "vim"},
		{"/pkg\nvim", "vim"},
		{"/pkg\tvim", "vim"},
		{"/pkg", ""},
		{"/pkg  a  b", "a b"},
		{"  /pkg  vim  ", "vim"},
	} {
		if got := commandArg(c.in); got != c.want {
			t.Errorf("commandArg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCmpNum(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2", "10", -1},
		{"10", "2", 1},
		{"007", "7", 0},                  // leading zeros ignored
		{"00", "0", 0},                   // all zeros
		{"99999999999999999999", "2", 1}, // no overflow: 20-digit number > 2
	}
	for _, c := range cases {
		if got := cmpNum(c.a, c.b); got != c.want {
			t.Errorf("cmpNum(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSearchMainTreeAvailability(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    []byte
		err     error
		wantOK  bool
		wantLen int
	}{
		{name: "answered empty", body: []byte("<html></html>"), wantOK: true},
		{name: "answered with match", body: []byte(`<a href="/packages/app-editors/vim">vim</a>`), wantOK: true, wantLen: 1},
		{name: "network failure", err: errors.New("connection reset")},
		{name: "server failure", err: &httpStatusError{url: "u", code: 503}},
		{name: "outbound busy", err: &httpBusyError{url: "u", wait: time.Millisecond}},
		{name: "body too large", err: &httpBodyTooLargeError{url: "u", limit: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := searchMainTreeWith(
				context.Background(),
				"vim",
				func(context.Context, string) (string, string, bool) {
					t.Fatal("bare search must not call the exact-version lookup")
					return "", "", false
				},
				func(context.Context, string, int64) ([]byte, error) { return tc.body, tc.err },
			)
			if ok != tc.wantOK || len(got) != tc.wantLen {
				t.Errorf("searchMainTreeWith() = (%v, %v), want len=%d ok=%v", got, ok, tc.wantLen, tc.wantOK)
			}
		})
	}
}

func TestSearchMainTreeExactAvailability(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stable, latest string
		available      bool
		wantOK         bool
		wantLen        int
	}{
		{name: "found", latest: "9.1", available: true, wantOK: true, wantLen: 1},
		{name: "answered missing", available: true, wantOK: true},
		{name: "lookup failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := searchMainTreeWith(
				context.Background(),
				"app-editors/vim",
				func(context.Context, string) (string, string, bool) {
					return tc.stable, tc.latest, tc.available
				},
				func(context.Context, string, int64) ([]byte, error) {
					t.Fatal("exact search must not fetch the HTML search page")
					return nil, nil
				},
			)
			if ok != tc.wantOK || len(got) != tc.wantLen {
				t.Errorf("searchMainTreeWith() = (%v, %v), want len=%d ok=%v", got, ok, tc.wantLen, tc.wantOK)
			}
		})
	}
}

func TestPkgCacheRefreshAvailability(t *testing.T) {
	sources := []overlay{{name: "answered"}, {name: "failed"}}
	pc := &pkgCache{pkgs: map[string]map[string]string{}, available: map[string]bool{}}
	status := pc.refreshWith(context.Background(), sources, func(_ context.Context, source overlay) (map[string]string, error) {
		if source.name == "failed" {
			return nil, errors.New("upstream unavailable")
		}
		return map[string]string{"app-editors/vim": "9.1"}, nil
	})
	if !status["answered"] || status["failed"] {
		t.Errorf("refresh status = %v, want answered=true failed=false", status)
	}
	if got := pc.pkgs["answered"]["app-editors/vim"]; got != "9.1" {
		t.Errorf("successful overlay result = %q, want 9.1", got)
	}
}

func TestRenderPkgAvailability(t *testing.T) {
	renderers := []struct {
		name string
		fn   func(i18n.Lang, []string, pkgLookupAvailability) string
	}{
		{
			name: "plain",
			fn: func(l i18n.Lang, main []string, availability pkgLookupAvailability) string {
				return renderPkg(l, "vim", main, map[string][2]string{}, nil, availability)
			},
		},
		{
			name: "rich",
			fn: func(l i18n.Lang, main []string, availability pkgLookupAvailability) string {
				return renderPkgRich(l, "vim", main, map[string][2]string{}, nil, availability)
			},
		},
	}
	cases := []struct {
		name         string
		main         []string
		availability pkgLookupAvailability
		want         func(i18n.Lang, pkgLookupAvailability) string
		notWant      func(i18n.Lang, pkgLookupAvailability) string
	}{
		{
			name:         "complete miss",
			availability: pkgLookupAvailability{official: true, overlays: map[string]bool{"guru": true}},
			want: func(l i18n.Lang, _ pkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.NotFound.For(l)
			},
			notWant: func(l i18n.Lang, availability pkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.Unavailable.Render(l, availability.unavailableSources(l))
			},
		},
		{
			name:         "lookup unavailable",
			availability: pkgLookupAvailability{overlays: map[string]bool{"guru": true}},
			want: func(l i18n.Lang, availability pkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.Unavailable.Render(l, availability.unavailableSources(l))
			},
			notWant: func(l i18n.Lang, _ pkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.NotFound.For(l)
			},
		},
		{
			name:         "partial hit",
			main:         []string{"app-editors/vim"},
			availability: pkgLookupAvailability{official: true, overlays: map[string]bool{"guru": false}},
			want: func(l i18n.Lang, availability pkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Source.PartialResults.Render(l, availability.unavailableSources(l))
			},
		},
	}
	for _, l := range i18n.Languages() {
		for _, renderer := range renderers {
			for _, tc := range cases {
				t.Run(l.String()+"/"+renderer.name+"/"+tc.name, func(t *testing.T) {
					got := renderer.fn(l, tc.main, tc.availability)
					want := tc.want(l, tc.availability)
					if !strings.Contains(got, want) {
						t.Errorf("rendered result %q does not contain %q", got, want)
					}
					if tc.notWant != nil {
						notWant := tc.notWant(l, tc.availability)
						if strings.Contains(got, notWant) {
							t.Errorf("rendered result %q unexpectedly contains %q", got, notWant)
						}
					}
				})
			}
		}
	}
}

// Gentoo numbers letter-suffixed releases above the bare version they patch (openssl 1.1.1w is
// newer than 1.1.1). When two tokens share a prefix, the one with characters left over is the
// greater one; reverse that and /pkg reports the older release as the latest available version.
func TestALetterSuffixedReleaseIsNewerThanTheBareVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool // want verLess(a, b)
	}{
		{"1.1.1", "1.1.1w", true},   // dev-libs/openssl: the letter patch level is newer
		{"1.1.1w", "1.1.1", false},  // ...and never older
		{"1.1.1s", "1.1.1w", true},  // two letter patch levels still order between themselves
		{"6.0", "6.0a", true},       // app-arch/unzip-6.0 vs a lettered respin
		{"1.2", "1.2a", true},       // the shared-prefix rule, at its smallest
		{"1.2a", "1.2", false},      //
		{"1.2a", "1.2a", false},     // equal is not less
		{"1.0-r2", "1.0-r10", true}, // positive control: the numeric path is untouched
	} {
		if got := verLess(c.a, c.b); got != c.want {
			t.Errorf("verLess(%q, %q) = %v, want %v: the latest version shown for a letter-suffixed package would be the older release",
				c.a, c.b, got, c.want)
		}
	}
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1w", "1", 1},  // leftover characters make the token greater
		{"1", "1w", -1}, // and their absence makes it smaller
		{"1", "1", 0},   // positive control: nothing left over on either side
	} {
		if got := cmpToken(c.a, c.b); got != c.want {
			t.Errorf("cmpToken(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// An underscore component that is not one of Gentoo's five known suffixes still has to order.
// Repology carries date-stamped versions in that shape, and comparing them as equal lets /pkgs
// present a year-old version as the newest one the family ships.
func TestUnknownUnderscoreSuffixesOrderByTheirRawToken(t *testing.T) {
	for _, c := range []struct {
		older, newer string
	}{
		{"1.2_20240101", "1.2_20250101"}, // date-stamped snapshots, a year apart
		{"1.2_git20240101", "1.2_git20250101"},
		{"3.0_alpha", "3.0_alpha2"}, // positive control: a known suffix still orders
	} {
		if !verLess(c.older, c.newer) {
			t.Errorf("verLess(%q, %q) = false, want true: the older version would be offered as the family's newest",
				c.older, c.newer)
		}
		if verLess(c.newer, c.older) {
			t.Errorf("verLess(%q, %q) = true, want false: the newer version would be treated as the older one",
				c.newer, c.older)
		}
	}
}

// A -rN revision is split off before the rest is parsed. Without the split, "1.0_p1-r1" reads as
// an unknown suffix that sorts after every release, so it outranks the genuinely newer "1.0_p2"
// and /pkg names the older patch level as the latest version.
func TestARevisionIsSplitOffBeforeTheSuffixesAreCompared(t *testing.T) {
	for _, c := range []struct {
		older, newer string
	}{
		{"1.0_p1-r1", "1.0_p2"},    // a revision of p1 is still older than p2
		{"1.0_rc1-r1", "1.0_rc2"},  //
		{"1.0_p1-r1", "1.0_p1-r2"}, // positive control: revisions order among themselves
		{"1.0", "1.0-r1"},          // positive control: a revision is newer than the bare version
	} {
		if !verLess(c.older, c.newer) {
			t.Errorf("verLess(%q, %q) = false, want true: the older patch level would be reported as the newest version",
				c.older, c.newer)
		}
		if verLess(c.newer, c.older) {
			t.Errorf("verLess(%q, %q) = true, want false", c.newer, c.older)
		}
	}
}
