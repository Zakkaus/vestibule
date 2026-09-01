package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSettingsMigratesVersionOneLookupDisableWithoutSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := settingsFile{
		Version: 1,
		Groups: map[int64]groupRecord{
			testGroupA: {Revision: 7, GroupOverrides: GroupOverrides{LookupTTLSeconds: ptr(0)}},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	group, _ := settings.Settings(testGroupA)
	if group.Revision() != 7 {
		t.Fatalf("migrated revision = %d, want 7", group.Revision())
	}
	if got := group.LookupTTLSeconds(); got.Value != 180 || got.Source != SourceFactory {
		t.Fatalf("migrated remembered TTL = %+v, want default 180", got)
	}
	if got := group.LookupAutoDeleteEnabled(); got.Value || got.Source != SourceChatOverride {
		t.Fatalf("migrated enabled state = %+v, want runtime false", got)
	}
}

func TestSettingsMigratesVersionTwoDMFirstOverrides(t *testing.T) {
	const (
		explicitGroup int64 = -1009000000003
		defaultGroup  int64 = -1009000000004
	)
	path := filepath.Join(t.TempDir(), "settings.json")
	source := map[string]any{
		"version": 2,
		"groups": map[string]any{
			"-1009000000001": map[string]any{"revision": 7, "dm_first": true},
			"-1009000000002": map[string]any{"revision": 9, "dm_first": false},
			"-1009000000003": map[string]any{"revision": 11, "dm_first": true, "delivery_mode": DeliveryBoth},
			"-1009000000004": map[string]any{"revision": 13},
		},
	}
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	baseline := testSettingsBaseline()
	for _, groupID := range []int64{explicitGroup, defaultGroup} {
		group := baseline.Factory
		group.ID = groupID
		baseline.Groups = append(baseline.Groups, group)
	}
	settings, err := NewStore(path, baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	for groupID, want := range map[int64]Setting[string]{
		testGroupA:    {Value: DeliveryDM, Source: SourceChatOverride},
		testGroupB:    {Value: DeliveryGroup, Source: SourceChatOverride},
		explicitGroup: {Value: DeliveryBoth, Source: SourceChatOverride},
		defaultGroup:  {Value: DeliveryBoth, Source: SourceFactory},
	} {
		group, _ := settings.Settings(groupID)
		if got := group.DeliveryMode(); got != want {
			t.Errorf("group %d migrated delivery mode = %+v, want %+v", groupID, got, want)
		}
	}
	var migrated map[string]any
	decodeFile(t, path, &migrated)
	if got := int(migrated["version"].(float64)); got != SettingsSchemaVersion {
		t.Fatalf("migrated schema version = %d, want %d", got, SettingsSchemaVersion)
	}
	groups := migrated["groups"].(map[string]any)
	for groupID, want := range map[string]string{
		"-1009000000001": DeliveryDM,
		"-1009000000002": DeliveryGroup,
		"-1009000000003": DeliveryBoth,
	} {
		record := groups[groupID].(map[string]any)
		if got := record["delivery_mode"]; got != want {
			t.Errorf("group %s delivery_mode = %v, want %q", groupID, got, want)
		}
		if _, exists := record["dm_first"]; exists {
			t.Errorf("group %s retained dm_first after migration: %#v", groupID, record)
		}
	}
	if record := groups["-1009000000004"].(map[string]any); record["delivery_mode"] != nil {
		t.Errorf("group with no delivery keys gained an override: %#v", record)
	}
}

func newLegacyGoldenSettings(t *testing.T) (*Store, string) {
	t.Helper()
	fixture, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "settings.json"))
	requireNoError(t, err)
	path := filepath.Join(t.TempDir(), "settings.json")
	requireNoError(t, os.WriteFile(path, fixture, 0o600))
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	return settings, path
}

func TestSettingsLegacyGoldenMigrationPreservesEffectiveValues(t *testing.T) {
	settings, _ := newLegacyGoldenSettings(t)
	for _, groupID := range []int64{testGroupA, testGroupB} {
		group := requireSettingsView(t, settings, groupID)
		requireEqual(t, group.Revision(), uint64(1), "migrated group revision")
		requireEqual(t, group.Enabled().Value, false, "migrated group enabled")
		requireEqual(t, group.NameSpoiler().Value, false, "migrated group name spoiler")
		requireEqual(t, group.VerifyMode().Value, ModeMixed, "migrated group verify mode")
		requireEqual(t, group.Enabled().Source, SourceChatOverride, "migrated group enabled source")
		requireEqual(t, group.NameSpoiler().Source, SourceChatOverride, "migrated group name spoiler source")
		requireEqual(t, group.VerifyMode().Source, SourceChatOverride, "migrated group verify mode source")
	}
}

func TestSettingsLegacyGoldenMigrationWritesSparseRecords(t *testing.T) {
	_, path := newLegacyGoldenSettings(t)
	var migrated settingsFile
	decodeFile(t, path, &migrated)
	requireEqual(t, migrated.Version, SettingsSchemaVersion, "migrated settings version")
	requireEqual(t, len(migrated.Groups), 2, "migrated settings group count")
	for _, groupID := range []int64{testGroupA, testGroupB} {
		record := migrated.Groups[groupID]
		requireEqual(t, record.Revision, uint64(1), "migrated record revision")
		requirePointerValue(t, record.Enabled, false, "migrated record enabled")
		requirePointerValue(t, record.NameSpoiler, false, "migrated record name spoiler")
		requirePointerValue(t, record.VerifyMode, ModeMixed, "migrated record verify mode")
	}
}

func TestSettingsVersionZeroMigrationBacksUpOriginal(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Persistence().Writable {
		t.Fatalf("settings became unwritable after migration: %v", settings.Persistence().LastError)
	}
	backup, err := os.ReadFile(path + ".v0.bak")
	if err != nil {
		t.Fatalf("read version-zero backup: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("version-zero backup changed the original bytes:\nwant %q\n got %q", original, backup)
	}
	var migrated settingsFile
	decodeFile(t, path, &migrated)
	if migrated.Version != SettingsSchemaVersion {
		t.Fatalf("migrated schema version = %d, want %d", migrated.Version, SettingsSchemaVersion)
	}
}

func TestSettingsVersionZeroBackupFailureDoesNotBlockMigration(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".v0.bak", 0o700); err != nil {
		t.Fatal(err)
	}

	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status := settings.Persistence(); !status.Writable || status.LastError != nil {
		t.Fatalf("backup failure blocked migration: %+v", status)
	}
	var migrated settingsFile
	decodeFile(t, path, &migrated)
	if migrated.Version != SettingsSchemaVersion {
		t.Fatalf("migrated schema version = %d, want %d", migrated.Version, SettingsSchemaVersion)
	}
}

func TestSettingsMalformedLegacyAntispamDisablesWritesWithoutChangingFile(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "antispam.json")
	before := []byte(`{"enabled":`)
	if err := os.WriteFile(legacyPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(dir, "settings.json")
	settings, err := NewStore(settingsPath, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status := settings.Persistence(); status.Writable || status.LastError == nil {
		t.Fatalf("malformed legacy persistence status = %+v", status)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("malformed legacy antispam changed\nwant %s\n got %s", before, after)
	}
	if _, err := os.Stat(legacyPath + ".corrupt"); !os.IsNotExist(err) {
		t.Fatalf("malformed legacy antispam was moved: %v", err)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings file written after failed migration: %v", err)
	}
}
