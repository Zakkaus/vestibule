package settings

import "testing"

func TestProcessSettingsViewDetachesCollections(t *testing.T) {
	bugs := true
	news := true
	silentBugs := true
	config := &Config{
		Feeds: []FeedConfig{{
			ChatID: -1009000000201, Bugs: &bugs, News: &news, SilentBugs: &silentBugs,
		}},
		Overlays: []OverlayCfg{{Name: "gentoo", Repo: "gentoo/overlay"}},
	}

	view := config.ProcessSettings()
	feeds := view.Feeds()
	overlays := view.Overlays()
	feeds.Value[0].ChatID = -1009000000202
	*feeds.Value[0].Bugs = false
	*feeds.Value[0].News = false
	*feeds.Value[0].SilentBugs = false
	overlays.Value[0].Repo = "other/overlay"

	if config.Feeds[0].ChatID != -1009000000201 || !*config.Feeds[0].Bugs || !*config.Feeds[0].News ||
		!*config.Feeds[0].SilentBugs || config.Overlays[0].Repo != "gentoo/overlay" {
		t.Fatalf("process view mutated config: feeds=%+v overlays=%+v", config.Feeds, config.Overlays)
	}
}

func TestProcessSettingsKeepsLegacyFeedFileManaged(t *testing.T) {
	const chatID int64 = -1009000002251
	config, err := LoadConfig(writeConfig(t, map[string]any{
		"feed": map[string]any{"chat_id": chatID, "lang": "en", "interval_seconds": 300},
	}))
	requireNoError(t, err)

	feeds := config.ProcessSettings().Feeds()
	if feeds.Source != SourceUserFile || len(feeds.Value) != 1 || feeds.Value[0].ChatID != chatID ||
		feeds.Value[0].Lang != "en" {
		t.Fatalf("legacy feed view = %+v; an operator's legacy feed would disappear or look writable in the console",
			feeds)
	}
}

func TestProcessSettingsDistinguishesEmptyValuesFromNull(t *testing.T) {
	empty, err := LoadConfig(writeConfig(t, map[string]any{
		"feeds":          []any{},
		"news_url":       "",
		"overlays":       []any{},
		"stats_timezone": "",
	}))
	requireNoError(t, err)
	requireProcessSettingsSources(t, empty.ProcessSettings(), SourceUserFile, "explicit empty values")

	null, err := LoadConfig(writeConfig(t, map[string]any{
		"feed":           nil,
		"feeds":          nil,
		"news_url":       nil,
		"overlays":       nil,
		"stats_timezone": nil,
	}))
	requireNoError(t, err)
	requireProcessSettingsSources(t, null.ProcessSettings(), SourceFactory, "null values")
}

func requireProcessSettingsSources(t *testing.T, view ProcessView, want Source, label string) {
	t.Helper()
	for name, got := range map[string]Source{
		"feeds":          view.Feeds().Source,
		"news_url":       view.NewsURL().Source,
		"overlays":       view.Overlays().Source,
		"stats_timezone": view.StatsTimezone().Source,
	} {
		if got != want {
			t.Fatalf("%s source for %s = %q, want %q; the console would report the wrong owner for that setting",
				label, name, got, want)
		}
	}
}
