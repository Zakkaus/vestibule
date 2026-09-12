package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type processSettingsSources struct {
	newsURL       bool
	overlays      bool
	statsTimezone bool
}

func processSettingsSourcesFromConfig(data []byte) (processSettingsSources, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return processSettingsSources{}, err
	}
	if _, legacy := fields["disabled_modules"]; legacy {
		return processSettingsSources{}, fmt.Errorf("disabled_modules is no longer supported; use modules to select enabled optional modules")
	}
	return processSettingsSources{
		newsURL:       processSettingPresent(fields, "news_url"),
		overlays:      processSettingPresent(fields, "overlays"),
		statsTimezone: processSettingPresent(fields, "stats_timezone"),
	}, nil
}

func processSettingPresent(fields map[string]json.RawMessage, name string) bool {
	value, ok := fields[name]
	return ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

// ProcessView is a read-only view of effective process-level settings and their sources.
type ProcessView struct {
	config *Config
}

// ProcessSettings returns a read-only view of the process-level settings.
func (c *Config) ProcessSettings() ProcessView {
	return ProcessView{config: c}
}

func (v ProcessView) NewsURL() Setting[string] {
	return processSetting(v.config.NewsURL, v.config.processSettingsSources.newsURL)
}

func (v ProcessView) Overlays() Setting[[]OverlayCfg] {
	return processSetting(cloneOverlayConfigs(v.config.Overlays), v.config.processSettingsSources.overlays)
}

func (v ProcessView) StatsTimezone() Setting[string] {
	return processSetting(v.config.StatsTimezone, v.config.processSettingsSources.statsTimezone)
}

func processSetting[T any](value T, managedByFile bool) Setting[T] {
	source := SourceFactory
	if managedByFile {
		source = SourceUserFile
	}
	return Setting[T]{Value: value, Source: source}
}

func cloneOverlayConfigs(values []OverlayCfg) []OverlayCfg {
	if values == nil {
		return []OverlayCfg{}
	}
	return append([]OverlayCfg(nil), values...)
}
