package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const legacyAntispamRuntimeGroup int64 = -1009000000003

var legacyAntispamWhitelist = []int64{-1009999900015, -1009999900013, -1009999900014}

type legacyAntispamMigration struct {
	settings     *Store
	settingsPath string
	antispamPath string
	fixture      []byte
}

func newLegacyAntispamMigration(t *testing.T) legacyAntispamMigration {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "antispam-legacy-current.json"))
	requireNoError(t, err)
	dir := t.TempDir()
	antispamPath := filepath.Join(dir, "antispam.json")
	requireNoError(t, os.WriteFile(antispamPath, fixture, 0o600))
	current := settingsFile{
		Version:          SettingsSchemaVersion - 1,
		RegisteredGroups: []RegisteredGroup{{ID: legacyAntispamRuntimeGroup, RegisteredBy: 42}},
		Groups:           map[int64]groupRecord{},
	}
	data, err := json.Marshal(current)
	requireNoError(t, err)
	settingsPath := filepath.Join(dir, "settings.json")
	requireNoError(t, os.WriteFile(settingsPath, data, 0o600))
	settings, err := NewStore(settingsPath, testSettingsBaseline(), nil)
	requireNoError(t, err)
	return legacyAntispamMigration{
		settings:     settings,
		settingsPath: settingsPath,
		antispamPath: antispamPath,
		fixture:      fixture,
	}
}

func TestSettingsAntispamMigrationAppliesToEveryEffectiveGroup(t *testing.T) {
	migration := newLegacyAntispamMigration(t)
	for _, groupID := range []int64{testGroupA, testGroupB, legacyAntispamRuntimeGroup} {
		group := requireSettingsView(t, migration.settings, groupID)
		requireEqual(t, group.Revision(), uint64(1), "migrated group revision")
		requireEqual(t, group.AntispamEnabled(), Setting[bool]{Value: true, Source: SourceChatOverride}, "migrated antispam")
		whitelist := group.ChannelWhitelist()
		requireDeepEqual(t, whitelist.Value, legacyAntispamWhitelist, "migrated whitelist")
		requireEqual(t, whitelist.Source, SourceChatOverride, "migrated whitelist source")
	}
}

func TestSettingsAntispamMigrationWritesCurrentState(t *testing.T) {
	migration := newLegacyAntispamMigration(t)
	var migrated settingsFile
	decodeFile(t, migration.settingsPath, &migrated)
	requireEqual(t, migrated.Version, SettingsSchemaVersion, "migrated settings version")
	requireEqual(t, len(migrated.Groups), 3, "migrated settings group count")
}

func TestSettingsAntispamMigrationPreservesLegacyFile(t *testing.T) {
	migration := newLegacyAntispamMigration(t)
	afterMigration, err := os.ReadFile(migration.antispamPath)
	requireNoError(t, err)
	requireDeepEqual(t, afterMigration, migration.fixture, "legacy antispam after migration")
}

func TestSettingsAntispamMigrationDoesNotReapply(t *testing.T) {
	migration := newLegacyAntispamMigration(t)
	group := requireSettingsView(t, migration.settings, testGroupA)
	override := group.Overrides()
	override.AntispamEnabled = ptr(false)
	empty := []int64{}
	override.ChannelWhitelist = &empty
	_, err := migration.settings.Update(group.ID(), group.Revision(), override)
	requireNoError(t, err)
	reloaded, err := NewStore(migration.settingsPath, testSettingsBaseline(), nil)
	requireNoError(t, err)
	group = requireSettingsView(t, reloaded, testGroupA)
	requireEqual(t, group.AntispamEnabled().Value, false, "legacy antispam after reload")
	requireEqual(t, len(group.ChannelWhitelist().Value), 0, "legacy whitelist after reload")
	afterReload, err := os.ReadFile(migration.antispamPath)
	requireNoError(t, err)
	requireDeepEqual(t, afterReload, migration.fixture, "legacy antispam after reload")
}
