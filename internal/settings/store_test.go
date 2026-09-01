package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	testGroupA int64 = -1009000000001
	testGroupB int64 = -1009000000002
)

func testSettingsBaseline() SettingsBaseline {
	group := GroupBaseline{
		Enabled:                 BaselineValue[bool]{Value: true, Source: SourceFactory},
		DeliveryMode:            BaselineValue[string]{Value: DeliveryBoth, Source: SourceFactory},
		VerifyMode:              BaselineValue[string]{Value: ModeKernel, Source: SourceFactory},
		NameSpoiler:             BaselineValue[bool]{Value: true, Source: SourceFactory},
		BanSeconds:              BaselineValue[int]{Value: 0, Source: SourceFactory},
		LookupTTLSeconds:        BaselineValue[int]{Value: 180, Source: SourceFactory},
		LookupAutoDeleteEnabled: BaselineValue[bool]{Value: true, Source: SourceFactory},
		TimeoutSeconds:          BaselineValue[int]{Value: 240, Source: SourceUserFile},
		VerifyMaxFails:          BaselineValue[int]{Value: 3, Source: SourceFactory},
		VerifyRetrySeconds:      BaselineValue[int]{Value: 180, Source: SourceFactory},
		MuteSeconds:             BaselineValue[int]{Value: 3600, Source: SourceFactory},
		VerifyInvited:           BaselineValue[bool]{Value: true, Source: SourceFactory},
		WarnLimit:               BaselineValue[int]{Value: 3, Source: SourceFactory},
		AntispamEnabled:         BaselineValue[bool]{Value: false, Source: SourceUserFile},
		ChannelWhitelist:        BaselineValue[[]int64]{Value: []int64{-1007000000001}, Source: SourceUserFile},
		TrustedMemberGroupIDs:   BaselineValue[[]int64]{Value: []int64{-1006000000001}, Source: SourceUserFile},
		KnownChatIDs:            BaselineValue[[]int64]{Value: []int64{-1005000000001}, Source: SourceUserFile},
		RequiredChannelID:       BaselineValue[int64]{Value: 0, Source: SourceFactory},
		ChannelDisplay:          BaselineValue[string]{Value: "", Source: SourceFactory},
		ChannelInviteURL:        BaselineValue[string]{Value: "https://t.me/+configured", Source: SourceUserFile},
		Questions: BaselineValue[[]Question]{Value: []Question{{
			Q: "Select Portage", Options: []string{"Portage", "apt"}, Answer: 0,
		}}, Source: SourceUserFile},
		FallbackQuestions:       BaselineValue[[]ShortQuestion]{Value: []ShortQuestion{}, Source: SourceFactory},
		FallbackBuiltin:         BaselineValue[bool]{Value: true, Source: SourceFactory},
		Lang:                    BaselineValue[string]{Value: "zh", Source: SourceFactory},
		RichMessages:            BaselineValue[bool]{Value: false, Source: SourceFactory},
		PrivateQueryPerMin:      BaselineValue[int]{Value: 3, Source: SourceFactory},
		AdminLogChatID:          BaselineValue[int64]{Value: 0, Source: SourceFactory},
		RequiredChannelFailOpen: BaselineValue[bool]{Value: true, Source: SourceFactory},
	}
	groupA, groupB := group, group
	groupA.ID = testGroupA
	groupB.ID = testGroupB
	groupB.VerifyMode = BaselineValue[string]{Value: ModeQuiz, Source: SourceUserFile}
	return SettingsBaseline{
		Groups:  []GroupBaseline{groupA, groupB},
		Factory: group,
	}
}

func TestEmptyOverridesInheritFactoryDefault(t *testing.T) {
	settings, err := NewStore("", testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	group, _ := settings.Settings(testGroupA)
	next := group.Overrides()
	next.Enabled = ptr(false)
	if _, err = settings.Update(group.ID(), group.Revision(), next); err != nil {
		t.Fatal(err)
	}
	group, _ = settings.Settings(testGroupA)
	if _, err = settings.Update(group.ID(), group.Revision(), GroupOverrides{}); err != nil {
		t.Fatal(err)
	}
	group, _ = settings.Settings(testGroupA)
	if got := group.Enabled(); !got.Value || got.Source != SourceFactory {
		t.Fatalf("empty override resolved enabled to %+v, want factory true", got)
	}
}

func TestSettingsRejectsInvalidWholeRecord(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GroupOverrides)
	}{
		{name: "delivery mode", mutate: func(next *GroupOverrides) { next.DeliveryMode = ptr("sidecar") }},
		{name: "verification mode", mutate: func(next *GroupOverrides) { next.VerifyMode = ptr("sidecar") }},
		{name: "language", mutate: func(next *GroupOverrides) { next.Lang = ptr("fr") }},
		{name: "verification timeout", mutate: func(next *GroupOverrides) { next.TimeoutSeconds = ptr(29) }},
		{name: "ban boundary", mutate: func(next *GroupOverrides) { next.BanSeconds = ptr(10) }},
		{name: "mute boundary", mutate: func(next *GroupOverrides) { next.MuteSeconds = ptr(10) }},
		{name: "warning limit", mutate: func(next *GroupOverrides) { next.WarnLimit = ptr(0) }},
		{name: "private query rate", mutate: func(next *GroupOverrides) { next.PrivateQueryPerMin = ptr(0) }},
		{name: "question answer", mutate: func(next *GroupOverrides) {
			questions := []Question{{Q: "Package manager?", Options: []string{"Portage", "apt"}, Answer: 2}}
			next.Questions = &questions
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings, err := NewStore("", testSettingsBaseline(), nil)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := settings.Settings(testGroupA)
			next := before.Overrides()
			test.mutate(&next)
			if _, err = settings.Update(before.ID(), before.Revision(), next); err == nil {
				t.Fatal("invalid complete record was accepted")
			}
			after, _ := settings.Settings(testGroupA)
			if after.Revision() != before.Revision() ||
				after.DeliveryMode().Value != before.DeliveryMode().Value ||
				after.VerifyMode().Value != before.VerifyMode().Value {
				t.Fatalf("failed validation published revision=%d delivery=%q verify=%q",
					after.Revision(), after.DeliveryMode().Value, after.VerifyMode().Value)
			}
		})
	}
}

func TestGroupLanguageSettingSourceAndRevision(t *testing.T) {
	baseline := testSettingsBaseline()
	baseline.Groups[0].Lang = BaselineValue[string]{Value: "zh-Hant", Source: SourceUserFile}
	settings, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	if err != nil {
		t.Fatal(err)
	}

	group, _ := settings.Settings(testGroupA)
	if got := group.Lang(); got.Value != "zh-Hant" || got.Source != SourceUserFile {
		t.Fatalf("configured language = %+v", got)
	}
	override := group.Overrides()
	override.Lang = ptr("en")
	result, err := settings.Update(testGroupA, group.Revision(), override)
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 1 {
		t.Fatalf("language revision = %d, want 1", result.Revision)
	}
	group, _ = settings.Settings(testGroupA)
	if got := group.Lang(); got.Value != "en" || got.Source != SourceChatOverride {
		t.Fatalf("runtime language = %+v", got)
	}

	stale := group.Overrides()
	stale.Lang = ptr("zh")
	if _, err := settings.Update(testGroupA, 0, stale); !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("stale language commit error = %v, want conflict", err)
	}
	invalid := group.Overrides()
	invalid.Lang = ptr("fr")
	if _, err := settings.Update(testGroupA, group.Revision(), invalid); err == nil {
		t.Fatal("unsupported runtime language was accepted")
	}
	group, _ = settings.Settings(testGroupA)
	if group.Revision() != 1 || group.Lang().Value != "en" {
		t.Fatalf("failed language commit published %+v at revision %d", group.Lang(), group.Revision())
	}

	restore := group.Overrides()
	restore.Lang = nil
	if _, err := settings.Update(testGroupA, group.Revision(), restore); err != nil {
		t.Fatal(err)
	}
	group, _ = settings.Settings(testGroupA)
	if got := group.Lang(); got.Value != "zh-Hant" || got.Source != SourceUserFile {
		t.Fatalf("restored language = %+v", got)
	}
}

func decodeFile(t *testing.T, path string, dst any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, data)
	}
}

func ptr[T any](value T) *T { return &value }
