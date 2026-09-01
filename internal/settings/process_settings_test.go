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
