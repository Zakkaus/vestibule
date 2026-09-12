package settings

import "testing"

func TestProcessSettingsViewDetachesCollections(t *testing.T) {
	config := &Config{Overlays: []OverlayCfg{{Name: "gentoo", Repo: "gentoo/overlay"}}}

	overlays := config.ProcessSettings().Overlays()
	overlays.Value[0].Repo = "other/overlay"

	if config.Overlays[0].Repo != "gentoo/overlay" {
		t.Fatalf("process view mutated config overlays: %+v", config.Overlays)
	}
}

func TestProcessSettingsDistinguishesEmptyValuesFromNull(t *testing.T) {
	empty, err := LoadConfig(writeConfig(t, map[string]any{
		"news_url":       "",
		"overlays":       []any{},
		"stats_timezone": "",
	}))
	requireNoError(t, err)
	requireProcessSettingsSources(t, empty.ProcessSettings(), SourceUserFile, "explicit empty values")

	null, err := LoadConfig(writeConfig(t, map[string]any{
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
