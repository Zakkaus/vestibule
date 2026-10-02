package lookup

import (
	"context"
	"reflect"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestSearchTransientNotDefinitive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := SearchTitles(ctx, WikiSources[0], "anything", 4); ok {
		t.Error("searchTitles must return ok=false on a fetch failure (not a false 'no entries')")
	}
	if _, ok := SearchArchcn(ctx, "anything", 5); ok {
		t.Error("searchArchcn must return ok=false on a fetch failure (not a false 'no results')")
	}
}

func TestPickWikiTitlesDedup(t *testing.T) {
	g := WikiSource{classify: classifyGentoo}
	// Case-insensitive topics prefer zh-cn and drop unsupported translations.
	got := g.PickWikiTitles(i18n.LangZH, []string{
		"NVIDIA/nvidia-drivers",
		"NVidia/nvidia-drivers/zh-cn",
		"NVIDIA/nvidia-drivers/fr",
	}, 4)
	if want := []string{"NVidia/nvidia-drivers/zh-cn"}; !reflect.DeepEqual(got, want) {
		t.Errorf("gentoo dedup = %v, want %v", got, want)
	}

	a := WikiSource{classify: classifyArch}
	// The localized Arch title must replace its English base topic.
	if got := a.PickWikiTitles(i18n.LangZH, []string{"NVIDIA", "Nvidia (简体中文)"}, 4); !reflect.DeepEqual(got, []string{"Nvidia (简体中文)"}) {
		t.Errorf("arch dedup = %v, want [Nvidia (简体中文)]", got)
	}

	if got := a.PickWikiTitles(i18n.LangZH, []string{"A", "B", "C", "D", "E"}, 3); !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Errorf("cap = %v, want [A B C]", got)
	}
}

func TestPickWikiTitlesPrioritizesRequesterLanguageAndFiltersForeignPages(t *testing.T) {
	a := WikiSource{classify: classifyArch}
	got := a.PickWikiTitles(i18n.LangZH, []string{
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

	got = a.PickWikiTitles(i18n.LangZH, []string{"Fallback topic (Русский)", "Fallback topic"}, 1)
	if want := []string{"Fallback topic"}; !reflect.DeepEqual(got, want) {
		t.Errorf("English wiki page was not preferred over an unsupported translation: got %v", got)
	}
}
