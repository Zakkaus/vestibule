package settings

import (
	"path/filepath"
	"testing"
)

func newLookupCapabilitySettings(t *testing.T) (*Store, string, SettingsBaseline) {
	t.Helper()
	configPath := writeConfig(t, map[string]any{
		"groups": []map[string]any{
			{
				"id":                     testGroupA,
				"gentoo_lookups_enabled": true,
				"linux_lookups_enabled":  false,
			},
			{"id": testGroupB},
		},
	})
	config, err := LoadConfig(configPath)
	requireNoError(t, err)
	baseline, err := LoadBaseline(configPath, config)
	requireNoError(t, err)
	statePath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(statePath, baseline, nil)
	requireNoError(t, err)
	return store, statePath, baseline
}

func TestLookupCapabilitiesUseFactoryAndUserFileProvenance(t *testing.T) {
	store, _, _ := newLookupCapabilitySettings(t)
	factoryGroup := requireSettingsView(t, store, testGroupB)
	requireLookupCapability(t, "GentooLookupsEnabled", factoryGroup.GentooLookupsEnabled(), false, SourceFactory)
	requireLookupCapability(t, "LinuxLookupsEnabled", factoryGroup.LinuxLookupsEnabled(), false, SourceFactory)

	userFileGroup := requireSettingsView(t, store, testGroupA)
	requireLookupCapability(t, "GentooLookupsEnabled", userFileGroup.GentooLookupsEnabled(), true, SourceUserFile)
	requireLookupCapability(t, "LinuxLookupsEnabled", userFileGroup.LinuxLookupsEnabled(), false, SourceUserFile)
}

func TestLookupCapabilityChatOverrideRestoresUserFileBaseline(t *testing.T) {
	store, _, _ := newLookupCapabilitySettings(t)
	group := requireSettingsView(t, store, testGroupA)
	overrides := group.Overrides()
	overrides.GentooLookupsEnabled = ptr(false)
	_, err := store.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)

	group = requireSettingsView(t, store, testGroupA)
	requireLookupCapability(t, "GentooLookupsEnabled", group.GentooLookupsEnabled(), false, SourceChatOverride)

	overrides.GentooLookupsEnabled = nil
	_, err = store.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	group = requireSettingsView(t, store, testGroupA)
	requireLookupCapability(t, "GentooLookupsEnabled", group.GentooLookupsEnabled(), true, SourceUserFile)
}

func TestLookupCapabilitiesRoundTripThroughSettingsJSON(t *testing.T) {
	store, statePath, baseline := newLookupCapabilitySettings(t)
	group := requireSettingsView(t, store, testGroupA)
	overrides := group.Overrides()
	overrides.GentooLookupsEnabled = ptr(false)
	overrides.LinuxLookupsEnabled = ptr(true)
	result, err := store.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	if !result.Durable {
		t.Fatalf("lookup capability commit was not durable")
	}

	reloaded, err := NewStore(statePath, baseline, nil)
	requireNoError(t, err)
	group = requireSettingsView(t, reloaded, testGroupA)
	requireLookupCapability(t, "GentooLookupsEnabled", group.GentooLookupsEnabled(), false, SourceChatOverride)
	requireLookupCapability(t, "LinuxLookupsEnabled", group.LinuxLookupsEnabled(), true, SourceChatOverride)
}

func requireLookupCapability(
	t *testing.T,
	name string,
	got Setting[bool],
	wantValue bool,
	wantSource Source,
) {
	t.Helper()
	if got.Value != wantValue || got.Source != wantSource {
		t.Fatalf("%s = value:%v source:%v, want value:%v source:%v",
			name, got.Value, got.Source, wantValue, wantSource)
	}
}
