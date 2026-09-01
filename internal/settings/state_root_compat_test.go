package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	stateCompatGroupA int64 = -1001234500001
	stateCompatGroupB int64 = -1001234500002
)

func stateCompatConfig() *Config {
	return &Config{Groups: []GroupConfig{{ID: stateCompatGroupA}, {ID: stateCompatGroupB}},
		GroupIDs:       []int64{stateCompatGroupA, stateCompatGroupB},
		TimeoutSeconds: 240,
		VerifyMaxFails: 3}
}

func TestStateCompatAntispamMigration(t *testing.T) {
	fixture := stateCompatFixture(t, "antispam.json")
	wantWhitelist := []int64{
		-1007000000003,
		-1007000000001,
		-1007000000002,
	}
	tests := []struct {
		name string
		data []byte
	}{
		{name: "current", data: fixture},
		{name: "unknown top-level key", data: stateCompatWithUnknown(t, fixture)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			legacyPath := filepath.Join(dir, "antispam.json")
			if err := os.WriteFile(legacyPath, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			settings, err := NewStore(filepath.Join(dir, "settings.json"), settingsBaselineFromConfig(stateCompatConfig(), configPresence{}), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, groupID := range []int64{stateCompatGroupA, stateCompatGroupB} {
				group, ok := settings.Settings(groupID)
				if !ok {
					t.Fatalf("group %d missing after antispam migration", groupID)
				}
				if !group.AntispamEnabled().Value || !reflect.DeepEqual(group.ChannelWhitelist().Value, wantWhitelist) {
					t.Fatalf("group %d antispam = enabled:%v whitelist:%v", groupID, group.AntispamEnabled().Value, group.ChannelWhitelist().Value)
				}
			}
			if after := stateCompatRead(t, legacyPath); !bytes.Equal(after, tt.data) {
				t.Fatal("legacy antispam fixture changed during migration")
			}
		})
	}
}

func newStateCompatSettings(t *testing.T, data []byte) (*Store, string) {
	t.Helper()
	path := stateCompatTempFile(t, "settings.json", data)
	settings, err := NewStore(path, settingsBaselineFromConfig(stateCompatConfig(), configPresence{}), nil)
	requireNoError(t, err)
	return settings, path
}

func assertStateCompatLegacySettings(t *testing.T, settings *Store, path string) {
	t.Helper()
	for _, groupID := range []int64{stateCompatGroupA, stateCompatGroupB} {
		group := requireSettingsView(t, settings, groupID)
		requireEqual(t, group.Enabled().Value, false, "legacy group enabled")
		requireEqual(t, group.NameSpoiler().Value, false, "legacy group name spoiler")
		requireEqual(t, group.VerifyMode().Value, ModeMixed, "legacy group verify mode")
		requireEqual(t, group.DeliveryMode().Value, DeliveryBoth, "legacy group delivery mode")
	}
	var migrated map[string]any
	stateCompatDecode(t, stateCompatRead(t, path), &migrated)
	requireDeepEqual(t, migrated["version"], float64(SettingsSchemaVersion), "migrated settings version")
	groups, ok := migrated["groups"].(map[string]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("migrated settings groups = %#v", migrated["groups"])
	}
}

func TestStateCompatLegacySettings(t *testing.T) {
	settings, path := newStateCompatSettings(t, stateCompatFixture(t, "settings.json"))
	assertStateCompatLegacySettings(t, settings, path)
}

func TestStateCompatLegacySettingsWithUnknownKey(t *testing.T) {
	fixture := stateCompatFixture(t, "settings.json")
	settings, path := newStateCompatSettings(t, stateCompatWithUnknown(t, fixture))
	assertStateCompatLegacySettings(t, settings, path)
}

func TestStateCompatSchemaV2Settings(t *testing.T) {
	settings, path := newStateCompatSettings(t, stateCompatFixture(t, "settings-v2.json"))
	group := requireSettingsView(t, settings, stateCompatGroupA)
	requireEqual(t, group.Revision(), uint64(5), "schema-v2 revision")
	requireEqual(t, group.Enabled().Value, false, "schema-v2 enabled")
	requireEqual(t, group.DeliveryMode().Value, DeliveryDM, "schema-v2 delivery mode")
	requireEqual(t, group.BanSeconds().Value, 86400, "schema-v2 ban seconds")
	requireEqual(t, group.Lang().Value, "en", "schema-v2 language")
	var upgraded map[string]any
	stateCompatDecode(t, stateCompatRead(t, path), &upgraded)
	record := upgraded["groups"].(map[string]any)["-1001234500001"].(map[string]any)
	if _, exists := record["dm_first"]; exists {
		t.Fatalf("obsolete dm_first survived allowlisted upgrade: %#v", record)
	}
}

func stateCompatFixture(t *testing.T, name string) []byte {
	t.Helper()
	return stateCompatRead(t, filepath.Join("..", "..", "testdata", "state", name))
}

func stateCompatTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func stateCompatRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stateCompatDecode(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("decode state JSON: %v\n%s", err, data)
	}
}

func stateCompatWithUnknown(t *testing.T, data []byte) []byte {
	t.Helper()
	var root any
	stateCompatDecode(t, data, &root)
	future := map[string]any{"schema": float64(99), "value": "preserve known fields"}
	switch value := root.(type) {
	case map[string]any:
		value["future_compat_key"] = future
	case []any:
		if len(value) == 0 {
			t.Fatal("cannot add an unknown record key to an empty fixture")
		}
		record, ok := value[0].(map[string]any)
		if !ok {
			t.Fatalf("fixture first record is %T, want object", value[0])
		}
		record["future_compat_key"] = future
	default:
		t.Fatalf("fixture root is %T, want object or array", root)
	}
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
