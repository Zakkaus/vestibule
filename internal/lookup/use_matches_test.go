package lookup

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

// Suggestions built in Go use the same canonical command as routing and the catalogue.
func TestUseMultipleMatchesSuggestsCanonicalCommand(t *testing.T) {
	atoms := []string{"www-client/firefox", "www-client/firefox-bin"}
	for _, l := range []i18n.Lang{i18n.LangEN, i18n.LangZH, i18n.LangZHHant} {
		got := renderUseMultipleMatches(l, append([]string(nil), atoms...), pkgLookupAvailability{})
		want := "/use "
		for _, atom := range atoms {
			if !strings.Contains(got, want+atom) {
				t.Errorf("%v: reply does not suggest %q for %s: %q", l, want, atom, got)
			}
		}
	}
}

func TestUseMultipleMatchesSortsAtoms(t *testing.T) {
	got := renderUseMultipleMatches(i18n.LangEN, []string{"b/z", "a/a"}, pkgLookupAvailability{})
	if strings.Index(got, "a/a") > strings.Index(got, "b/z") {
		t.Errorf("atoms are not sorted: %q", got)
	}
}

// The homepage link text used to be the English word, printed as-is to Chinese readers.
func TestHomepageLabelIsLocalized(t *testing.T) {
	want := map[i18n.Lang]string{i18n.LangEN: "homepage", i18n.LangZH: "主页", i18n.LangZHHant: "首頁"}
	for l, expected := range want {
		if got := i18n.Messages.LookupPackages.Use.Homepage.For(l); got != expected {
			t.Errorf("%v homepage label = %q, want %q", l, got, expected)
		}
	}
}
