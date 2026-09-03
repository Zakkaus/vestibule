package lookup

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestSearchTransientNotDefinitive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := searchTitles(ctx, wikiSources[0], "anything", 4); ok {
		t.Error("searchTitles must return ok=false on a fetch failure (not a false 'no entries')")
	}
	if _, ok := searchArchcn(ctx, "anything", 5); ok {
		t.Error("searchArchcn must return ok=false on a fetch failure (not a false 'no results')")
	}
}

func TestPickWikiTitlesDedup(t *testing.T) {
	g := wikiSource{classify: classifyGentoo}
	// Case-insensitive topics prefer zh-cn and drop unsupported translations.
	got := g.pickWikiTitles(i18n.LangZH, []string{
		"NVIDIA/nvidia-drivers",
		"NVidia/nvidia-drivers/zh-cn",
		"NVIDIA/nvidia-drivers/fr",
	}, 4)
	if want := []string{"NVidia/nvidia-drivers/zh-cn"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gentoo dedup = %v, want %v", got, want)
	}

	a := wikiSource{classify: classifyArch}
	// The localized Arch title must replace its English base topic.
	if got := a.pickWikiTitles(i18n.LangZH, []string{"NVIDIA", "Nvidia (简体中文)"}, 4); !reflect.DeepEqual(got, []string{"Nvidia (简体中文)"}) {
		t.Errorf("arch dedup = %v, want [Nvidia (简体中文)]", got)
	}

	if got := a.pickWikiTitles(i18n.LangZH, []string{"A", "B", "C", "D", "E"}, 3); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Errorf("cap = %v, want [A B C]", got)
	}
}

func TestPickWikiTitlesPrioritizesRequesterLanguageAndFiltersForeignPages(t *testing.T) {
	a := wikiSource{classify: classifyArch}
	got := a.pickWikiTitles(i18n.LangZH, []string{
		"English first",
		"Russian only (Русский)",
		"Fallback topic (Русский)",
		"日本語ページ",
		"Japanese only (日本語)",
		"Chinese first (简体中文)",
		"Fallback topic",
		"Chinese second (简体中文)",
		"English second",
		"Polish only (Polski)",
	}, 4)
	want := []string{
		"Chinese first (简体中文)",
		"Chinese second (简体中文)",
		"English first",
		"Fallback topic",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wiki language selection = %v, want %v; foreign pages must not displace Chinese or English results", got, want)
	}

	got = a.pickWikiTitles(i18n.LangZH, []string{"Fallback topic (Русский)", "Fallback topic"}, 1)
	if want := []string{"Fallback topic"}; !reflect.DeepEqual(got, want) {
		t.Errorf("English wiki page was not preferred over an unsupported translation: got %v", got)
	}
}

func TestWikiResultNotice(t *testing.T) {
	l := i18n.LangZH
	noMatches := i18n.Messages.LookupContent.Wiki.NoMatches.For(l)
	sourceJoin := i18n.Messages.LookupContent.Wiki.SourceJoin.For(l)
	sourcesUnavailable := func(names ...string) string {
		return i18n.Messages.LookupContent.Wiki.SourcesUnavailable.Render(l, strings.Join(names, sourceJoin))
	}
	gentooWiki := wikiSources[0].name + " Wiki"
	archWiki := wikiSources[1].name + " Wiki"
	tests := []struct {
		name  string
		found bool
		srcOK []bool
		want  string
	}{
		{
			name:  "complete miss",
			srcOK: []bool{true, true},
			want:  noMatches,
		},
		{
			name:  "Gentoo unavailable",
			srcOK: []bool{false, true},
			want:  sourcesUnavailable(gentooWiki),
		},
		{
			name:  "Arch unavailable with a hit",
			found: true,
			srcOK: []bool{true, false},
			want:  sourcesUnavailable(archWiki),
		},
		{
			name:  "all unavailable",
			srcOK: []bool{false, false},
			want:  sourcesUnavailable(gentooWiki, archWiki),
		},
		{
			name:  "complete hit",
			found: true,
			srcOK: []bool{true, true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wikiResultNotice(l, tt.found, tt.srcOK)
			if got != tt.want {
				t.Errorf("wikiResultNotice() = %q, want %q", got, tt.want)
			}
			if got == noMatches && (!tt.srcOK[0] || !tt.srcOK[1]) {
				t.Errorf("unavailable source produced a definitive miss: %q", got)
			}
		})
	}
}
