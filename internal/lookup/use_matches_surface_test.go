package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

// Suggestions built in Go use the same canonical command as routing and the catalogue.
func TestUseMultipleMatchesSuggestsCanonicalCommand(t *testing.T) {
	atoms := []string{"www-client/firefox", "www-client/firefox-bin"}
	for _, l := range i18n.Languages() {
		got := tgfmt.RenderUseMultipleMatches(l, append([]string(nil), atoms...), lookup.TestPkgLookupAvailability{})
		want := "/guse "
		for _, atom := range atoms {
			if !strings.Contains(got, want+atom) {
				t.Errorf("%v: reply does not suggest %q for %s: %q", l, want, atom, got)
			}
		}
	}
}

func TestUseMultipleMatchesSortsAtoms(t *testing.T) {
	got := tgfmt.RenderUseMultipleMatches(i18n.LangEN, []string{"b/z", "a/a"}, lookup.TestPkgLookupAvailability{})
	if strings.Index(got, "a/a") > strings.Index(got, "b/z") {
		t.Errorf("atoms are not sorted: %q", got)
	}
}
