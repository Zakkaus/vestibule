package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

const upgradeFixtureChatID int64 = -1001234500001

func upgradeFixtureRecord(t *testing.T, fixture string) groupRecord {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", fixture))
	requireNoError(t, err)
	path := filepath.Join(t.TempDir(), "settings.yaml")
	requireNoError(t, os.WriteFile(path, source, 0o600))
	upgraded, err := upgradeSettingsFile(path, []int64{upgradeFixtureChatID}, true)
	requireNoError(t, err)
	record, ok := upgraded.Groups[upgradeFixtureChatID]
	if !ok {
		t.Fatalf("upgraded groups = %#v; configured chat missing", upgraded.Groups)
	}
	if record.MuteSeconds != nil || record.WarnLimit != nil {
		t.Fatalf("new fields must remain sparse and inherit factory defaults: %+v", record.GroupOverrides)
	}
	_, err = upgradeSettingsFile(path, []int64{upgradeFixtureChatID}, true)
	requireNoError(t, err)
	t.Logf("%s upgraded: revision=%d overrides=%+v", fixture, record.Revision, record.GroupOverrides)
	return record
}

func TestUpgradeSettingsVersionZero(t *testing.T) {
	record := upgradeFixtureRecord(t, "settings.json")
	requireEqual(t, record.Revision, uint64(1), "v0 revision")
	requirePointerValue(t, record.Enabled, false, "v0 enabled")
	requirePointerValue(t, record.NameSpoiler, false, "v0 name spoiler")
	requirePointerValue(t, record.VerifyMode, ModeMixed, "v0 verify mode")
}

func TestUpgradeSettingsVersionOne(t *testing.T) {
	record := upgradeFixtureRecord(t, "settings-v1.json")
	requireEqual(t, record.Revision, uint64(4), "v1 revision")
	requirePointerValue(t, record.Enabled, false, "v1 enabled")
	requirePointerValue(t, record.LookupTTLSeconds, 321, "v1 lookup TTL")
	requirePointerValue(t, record.LookupAutoDeleteEnabled, true, "v1 lookup auto-delete")
	requirePointerValue(t, record.VerifyMode, ModeQuiz, "v1 verify mode")
}

func TestUpgradeSettingsVersionTwo(t *testing.T) {
	record := upgradeFixtureRecord(t, "settings-v2.json")
	requireEqual(t, record.Revision, uint64(5), "v2 revision")
	requirePointerValue(t, record.Enabled, false, "v2 enabled")
	requirePointerValue(t, record.DeliveryMode, DeliveryDM, "v2 delivery mode")
	requirePointerValue(t, record.BanSeconds, 86400, "v2 ban seconds")
	requirePointerValue(t, record.Lang, "en", "v2 language")
}

func TestUpgradeSettingsVersionThree(t *testing.T) {
	record := upgradeFixtureRecord(t, "settings-v3.json")
	requireEqual(t, record.Revision, uint64(6), "v3 revision")
	requirePointerValue(t, record.DeliveryMode, DeliveryBoth, "v3 delivery mode")
	requirePointerValue(t, record.TimeoutSeconds, 420, "v3 timeout")
	requirePointerValue(t, record.AntispamEnabled, false, "v3 antispam")
	requirePointerDeepEqual(t, record.ChannelWhitelist, []int64{-1007000000001}, "v3 channel whitelist")
}

func TestEveryGroupSettingHasCopyRule(t *testing.T) {
	covered := make(map[string]bool, len(groupCopyRules))
	for _, rule := range groupCopyRules {
		if len(rule.path) != 1 {
			t.Fatalf("group copy rule must be relative: %v", rule.path)
		}
		covered[rule.path[0]] = true
	}
	if !covered["revision"] {
		t.Error("group revision has no copy rule")
	}
	typ := reflect.TypeOf(GroupOverrides{})
	var defaults struct {
		Factory map[string]any `yaml:"factory"`
	}
	if err := yaml.Unmarshal([]byte(defaultsYAML), &defaults); err != nil {
		t.Fatal(err)
	}
	for i := range typ.NumField() {
		name := typ.Field(i).Tag.Get("json")
		if comma := len(name); comma > 0 {
			for j, char := range name {
				if char == ',' {
					name = name[:j]
					break
				}
			}
		}
		if name == "" || name == "-" {
			continue
		}
		if !covered[name] {
			t.Errorf("GroupOverrides.%s (%s) has no configupgrade copy rule", typ.Field(i).Name, name)
		}
		if _, ok := defaults.Factory[name]; !ok {
			t.Errorf("GroupOverrides.%s (%s) has no factory value in defaults.yaml", typ.Field(i).Name, name)
		}
	}
}
