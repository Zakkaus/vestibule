package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

const upgradeFixtureChatID int64 = -1009999900004

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
	requirePointerDeepEqual(t, record.ChannelWhitelist, []int64{-1009999900013}, "v3 channel whitelist")
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

func upgradeSettingsSource(t *testing.T, source any, chatIDs []int64) settingsFile {
	t.Helper()
	data, err := json.Marshal(source)
	requireNoError(t, err)
	path := filepath.Join(t.TempDir(), "settings.json")
	requireNoError(t, os.WriteFile(path, data, 0o600))
	upgraded, err := upgradeSettingsFile(path, chatIDs, false)
	requireNoError(t, err)
	return upgraded
}

func TestUpgradeKeepsEverySparseGroupOverride(t *testing.T) {
	const groupID int64 = -1009000002221
	want := groupRecord{
		Revision: 17,
		GroupOverrides: GroupOverrides{
			Enabled:                 ptr(false),
			DeliveryMode:            ptr(DeliveryDM),
			VerifyMode:              ptr(ModeMixed),
			NameSpoiler:             ptr(false),
			BanSeconds:              ptr(3600),
			LookupTTLSeconds:        ptr(123),
			LookupAutoDeleteEnabled: ptr(false),
			TimeoutSeconds:          ptr(300),
			VerifyMaxFails:          ptr(7),
			VerifyRetrySeconds:      ptr(60),
			MuteSeconds:             ptr(1800),
			VerifyInvited:           ptr(false),
			WarnLimit:               ptr(5),
			AntispamEnabled:         ptr(false),
			ChannelWhitelist:        ptr([]int64{-1009000002222}),
			TrustedMemberGroupIDs:   ptr([]int64{-1009000002223}),
			KnownChatIDs:            ptr([]int64{-1009000002224}),
			RequiredChannelID:       ptr(int64(-1009000002225)),
			ChannelDisplay:          ptr("@required"),
			ChannelInviteURL:        ptr("https://t.me/+required"),
			Questions:               ptr([]Question{{Q: "Question", Options: []string{"yes", "no"}, Answer: 0}}),
			FallbackQuestions:       ptr([]ShortQuestion{{Q: "Question", Answers: []string{"yes"}}}),
			FallbackBuiltin:         ptr(false),
			Lang:                    ptr("en"),
			RichMessages:            ptr(true),
			PrivateQueryPerMin:      ptr(9),
			AdminLogChatID:          ptr(int64(-1009000002226)),
			RequiredChannelFailOpen: ptr(false),
		},
	}
	upgraded := upgradeSettingsSource(t, settingsFile{
		Version: SettingsSchemaVersion - 1,
		Groups:  map[int64]groupRecord{groupID: want},
	}, nil)
	got, ok := upgraded.Groups[groupID]
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("group after upgrade = %#v, want %#v; a group's saved moderation, delivery, and access controls would be reset",
			got, want)
	}
}

func requireUpgradedGroupRecord(t *testing.T, got, want groupRecord, harm string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group after upgrade = %#v, want %#v; %s", got, want, harm)
	}
}

func TestUpgradeScopesLegacyTopLevelValuesToConfiguredUnsetGroups(t *testing.T) {
	const (
		configuredGroup    int64 = -1009000002231
		newConfiguredGroup int64 = -1009000002232
		unconfiguredGroup  int64 = -1009000002233
	)
	upgraded := upgradeSettingsSource(t, map[string]any{
		"enabled":      false,
		"name_spoiler": false,
		"verify_mode":  ModeQuiz,
		"groups": map[string]any{
			"-1009000002231": map[string]any{
				"revision":     11,
				"enabled":      true,
				"name_spoiler": true,
				"verify_mode":  ModeKernel,
			},
			"-1009000002233": map[string]any{"revision": 12},
		},
	}, []int64{configuredGroup, newConfiguredGroup})

	requireUpgradedGroupRecord(t, upgraded.Groups[configuredGroup], groupRecord{
		Revision: 11,
		GroupOverrides: GroupOverrides{
			Enabled:     ptr(true),
			NameSpoiler: ptr(true),
			VerifyMode:  ptr(ModeKernel),
		},
	}, "legacy top-level values overwrote that group's explicit decisions")
	requireUpgradedGroupRecord(t, upgraded.Groups[newConfiguredGroup], groupRecord{
		Revision: 1,
		GroupOverrides: GroupOverrides{
			Enabled:     ptr(false),
			NameSpoiler: ptr(false),
			VerifyMode:  ptr(ModeQuiz),
		},
	}, "the configured group lost its legacy settings")
	requireUpgradedGroupRecord(t, upgraded.Groups[unconfiguredGroup], groupRecord{Revision: 12},
		"legacy settings leaked into a group the operator did not configure")
}

func TestUpgradeLeavesInvalidLegacyVerifyModeOutOfOverrides(t *testing.T) {
	const groupID int64 = -1009000002241
	valid := upgradeSettingsSource(t, map[string]any{"verify_mode": ModeQuiz}, []int64{groupID})
	requirePointerValue(t, valid.Groups[groupID].VerifyMode, ModeQuiz, "valid legacy verify mode")

	invalid := upgradeSettingsSource(t, map[string]any{"verify_mode": "not-a-mode"}, []int64{groupID})
	if invalid.Groups[groupID].VerifyMode != nil {
		t.Fatalf("invalid legacy verify mode = %q; malformed state became a current group override",
			*invalid.Groups[groupID].VerifyMode)
	}
}
