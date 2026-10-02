package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestRenderNewsAvailability(t *testing.T) {
	item := lookup.NewsItem{Date: "2026-08-24", Title: "Kernel update", URL: "https://example.test/kernel"}
	tests := []struct {
		Name      string
		Arg       string
		Items     []lookup.NewsItem
		Available bool
		Want      []string
		NotWant   string
	}{
		{
			Name:      "authoritative empty result",
			Arg:       "missing",
			Available: true,
			Want:      []string{i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH)},
			NotWant:   i18n.Messages.LookupContent.News.Unavailable.For(i18n.LangZH),
		},
		{
			Name:    "index unavailable",
			Arg:     "missing",
			Want:    []string{i18n.Messages.LookupContent.News.Unavailable.For(i18n.LangZH)},
			NotWant: i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH),
		},
		{
			Name:      "available filtered miss",
			Arg:       "missing",
			Items:     []lookup.NewsItem{item},
			Available: true,
			Want:      []string{i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH)},
		},
		{
			Name:    "stale hit is incomplete",
			Arg:     "kernel",
			Items:   []lookup.NewsItem{item},
			Want:    []string{item.Title, i18n.Messages.LookupContent.News.Stale.For(i18n.LangZH)},
			NotWant: i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH),
		},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := tgfmt.RenderNews(i18n.LangZH, tt.Arg, tt.Items, tt.Available)
			for _, want := range tt.Want {
				if !strings.Contains(got, want) {
					t.Errorf("renderNews() = %q, want substring %q", got, want)
				}
			}
			if tt.NotWant != "" && strings.Contains(got, tt.NotWant) {
				t.Errorf("renderNews() = %q, unwanted substring %q", got, tt.NotWant)
			}
		})
	}
}

func TestNewsRenderingLimitsRepliesToEightItems(t *testing.T) {
	items := make([]lookup.NewsItem, 9)
	for i := range items {
		items[i] = lookup.NewsItem{Date: "2026-09-03", Title: "Kernel news", URL: "https://example.test/news"}
	}

	got := tgfmt.RenderNews(i18n.LangEN, "", items, true)
	if count := strings.Count(got, "\n • "); count != 8 {
		t.Errorf("/news rendered %d items; more than eight can make its Telegram reply too long", count)
	}
}
