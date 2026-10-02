package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestWikiResultNotice(t *testing.T) {
	l := i18n.LangZH
	noMatches := i18n.Messages.LookupContent.Wiki.NoMatches.For(l)
	sourceJoin := i18n.Messages.LookupContent.Wiki.SourceJoin.For(l)
	sourcesUnavailable := func(names ...string) string {
		return i18n.Messages.LookupContent.Wiki.SourcesUnavailable.Render(l, strings.Join(names, sourceJoin))
	}
	gentooWiki := (*lookup.TestWikiSources)[0].Name + " Wiki"
	archWiki := (*lookup.TestWikiSources)[1].Name + " Wiki"
	tests := []struct {
		Name  string
		Found bool
		SrcOK []bool
		Want  string
	}{
		{
			Name:  "complete miss",
			SrcOK: []bool{true, true},
			Want:  noMatches,
		},
		{
			Name:  "Gentoo unavailable",
			SrcOK: []bool{false, true},
			Want:  sourcesUnavailable(gentooWiki),
		},
		{
			Name:  "Arch unavailable with a hit",
			Found: true,
			SrcOK: []bool{true, false},
			Want:  sourcesUnavailable(archWiki),
		},
		{
			Name:  "all unavailable",
			SrcOK: []bool{false, false},
			Want:  sourcesUnavailable(gentooWiki, archWiki),
		},
		{
			Name:  "complete hit",
			Found: true,
			SrcOK: []bool{true, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := tgfmt.WikiResultNotice(l, tt.Found, tt.SrcOK)
			if got != tt.Want {
				t.Errorf("wikiResultNotice() = %q, want %q", got, tt.Want)
			}
			if got == noMatches && (!tt.SrcOK[0] || !tt.SrcOK[1]) {
				t.Errorf("unavailable source produced a definitive miss: %q", got)
			}
		})
	}
}
