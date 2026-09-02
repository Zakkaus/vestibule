package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func newLayeredSettings(t *testing.T) *Store {
	t.Helper()
	factory := factoryBaseline()
	fileChat := cloneGroupBaseline(factory)
	fileChat.ID = testGroupA
	fileChat.TimeoutSeconds = userFileValue(600)
	factoryChat := cloneGroupBaseline(factory)
	factoryChat.ID = testGroupB
	settings, err := NewStore("", SettingsBaseline{
		Groups:  []GroupBaseline{fileChat, factoryChat},
		Factory: factory,
	}, nil)
	requireNoError(t, err)
	return settings
}

func TestSettingsConfiguredTimeoutKeepsUserFileSource(t *testing.T) {
	group := requireSettingsView(t, newLayeredSettings(t), testGroupA)
	requireEqual(t, group.TimeoutSeconds(), Setting[int]{Value: 600, Source: SourceUserFile}, "file-managed timeout")
}

func TestSettingsFactoryTimeoutKeepsFactorySource(t *testing.T) {
	group := requireSettingsView(t, newLayeredSettings(t), testGroupB)
	requireEqual(t, group.TimeoutSeconds(), Setting[int]{Value: 240, Source: SourceFactory}, "factory timeout")
}

func TestSettingsFreshGroupHasNoOverrides(t *testing.T) {
	group := requireSettingsView(t, newLayeredSettings(t), testGroupB)
	requireDeepEqual(t, group.Overrides(), GroupOverrides{}, "empty chat override")
}

func TestSettingsGroupOverrideUsesChatSource(t *testing.T) {
	settings := newLayeredSettings(t)
	group := requireSettingsView(t, settings, testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	_, err := settings.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	requireEqual(t, group.Enabled(), Setting[bool]{Value: false, Source: SourceChatOverride}, "chat override source")
}

func TestSettingsGroupOverrideLeavesOtherChatUnchanged(t *testing.T) {
	settings := newLayeredSettings(t)
	group := requireSettingsView(t, settings, testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	_, err := settings.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	other := requireSettingsView(t, settings, testGroupB)
	requireEqual(t, other.Enabled(), Setting[bool]{Value: true, Source: SourceFactory}, "other chat enabled")
	requireEqual(t, other.Revision(), uint64(0), "other chat revision")
}

func TestSettingsEmptyOverrideRestoresFactoryEnabled(t *testing.T) {
	settings := newLayeredSettings(t)
	group := requireSettingsView(t, settings, testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	_, err := settings.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	_, err = settings.Update(group.ID(), group.Revision(), GroupOverrides{})
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	requireEqual(t, group.Enabled(), Setting[bool]{Value: true, Source: SourceFactory}, "empty override enabled")
}

func TestSettingsEmptyOverrideKeepsUserFileTimeout(t *testing.T) {
	settings := newLayeredSettings(t)
	group := requireSettingsView(t, settings, testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	_, err := settings.Update(group.ID(), group.Revision(), overrides)
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	_, err = settings.Update(group.ID(), group.Revision(), GroupOverrides{})
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	requireEqual(t, group.TimeoutSeconds(), Setting[int]{Value: 600, Source: SourceUserFile}, "empty override timeout")
}

func newSparseSettings(t *testing.T) (*Store, string, GroupView) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	return settings, path, requireSettingsView(t, settings, testGroupA)
}

func sparseOverrides() GroupOverrides {
	emptyIDs := []int64{}
	emptyQuestions := []Question{}
	fallback := []ShortQuestion{{Q: "Package manager?", Answers: []string{"portage", "emerge"}}}
	return GroupOverrides{
		Enabled:                 ptr(false),
		DeliveryMode:            ptr(DeliveryGroup),
		VerifyMode:              ptr(ModeMixed),
		NameSpoiler:             ptr(false),
		BanSeconds:              ptr(3600),
		LookupTTLSeconds:        ptr(300),
		LookupAutoDeleteEnabled: ptr(false),
		TimeoutSeconds:          ptr(600),
		VerifyMaxFails:          ptr(-1),
		VerifyRetrySeconds:      ptr(-1),
		AntispamEnabled:         ptr(true),
		ChannelWhitelist:        &emptyIDs,
		TrustedMemberGroupIDs:   ptr([]int64{-1009999900012}),
		KnownChatIDs:            &emptyIDs,
		RequiredChannelID:       ptr(int64(-1009999900008)),
		ChannelDisplay:          ptr("@required"),
		ChannelInviteURL:        ptr(""),
		Questions:               &emptyQuestions,
		FallbackQuestions:       &fallback,
		FallbackBuiltin:         ptr(false),
		Lang:                    ptr("en"),
		RichMessages:            ptr(true),
		PrivateQueryPerMin:      ptr(7),
	}
}

func reloadSparseOverrides(t *testing.T) (*Store, string, GroupView, []ShortQuestion) {
	t.Helper()
	settings, path, initial := newSparseSettings(t)
	result, err := settings.Update(testGroupA, initial.Revision(), sparseOverrides())
	requireNoError(t, err)
	requireEqual(t, result.Revision, uint64(1), "sparse override revision")
	requireEqual(t, result.Durable, true, "sparse override durability")
	reloaded, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	fallback := []ShortQuestion{{Q: "Package manager?", Answers: []string{"portage", "emerge"}}}
	return reloaded, path, requireSettingsView(t, reloaded, testGroupA), fallback
}

func TestSettingsSparseBaselineSources(t *testing.T) {
	_, _, initial := newSparseSettings(t)
	requireEqual(t, initial.Enabled(), Setting[bool]{Value: true, Source: SourceFactory}, "initial enabled")
	requireEqual(t, initial.DeliveryMode(), Setting[string]{Value: DeliveryBoth, Source: SourceFactory}, "initial delivery mode")
	requireEqual(t, initial.TimeoutSeconds(), Setting[int]{Value: 240, Source: SourceUserFile}, "initial timeout")
	requireEqual(t, initial.LookupAutoDeleteEnabled(), Setting[bool]{Value: true, Source: SourceFactory}, "initial lookup auto-delete")
	requireEqual(t, initial.AntispamEnabled(), Setting[bool]{Value: false, Source: SourceUserFile}, "initial antispam")
	requireEqual(t, initial.Lang(), Setting[string]{Value: "zh", Source: SourceFactory}, "initial language")
}

func TestSettingsBaselineEqualOverrideStaysSparse(t *testing.T) {
	settings, path, initial := newSparseSettings(t)
	overrides := initial.Overrides()
	overrides.Enabled = ptr(true)
	result, err := settings.Update(testGroupA, initial.Revision(), overrides)
	requireNoError(t, err)
	requireEqual(t, result.Revision, uint64(0), "baseline-equal revision")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("baseline-equal value created settings file: %v", err)
	}
}

func requireEmptyInt64Setting(t *testing.T, got Setting[[]int64], source Source, label string) {
	t.Helper()
	requireEqual(t, len(got.Value), 0, label+" length")
	requireEqual(t, got.Source, source, label+" source")
}

func requireEmptyQuestionSetting(t *testing.T, got Setting[[]Question], source Source, label string) {
	t.Helper()
	requireEqual(t, len(got.Value), 0, label+" length")
	requireEqual(t, got.Source, source, label+" source")
}

func TestSettingsSparseOverridesRoundTrip(t *testing.T) {
	_, _, group, fallback := reloadSparseOverrides(t)
	requireEqual(t, group.Revision(), uint64(1), "reloaded revision")
	requireEqual(t, group.Enabled(), Setting[bool]{Value: false, Source: SourceChatOverride}, "reloaded enabled")
	requireEqual(t, group.DeliveryMode(), Setting[string]{Value: DeliveryGroup, Source: SourceChatOverride}, "reloaded delivery mode")
	requireEmptyInt64Setting(t, group.ChannelWhitelist(), SourceChatOverride, "explicit empty channel whitelist")
	requireEqual(t, group.LookupTTLSeconds(), Setting[int]{Value: 300, Source: SourceChatOverride}, "remembered lookup TTL")
	requireEqual(t, group.LookupAutoDeleteEnabled(), Setting[bool]{Value: false, Source: SourceChatOverride}, "disabled lookup auto-delete")
	requireEqual(t, group.AntispamEnabled(), Setting[bool]{Value: true, Source: SourceChatOverride}, "enabled antispam")
	requireEmptyInt64Setting(t, group.KnownChatIDs(), SourceChatOverride, "explicit empty known chats")
	requireEmptyQuestionSetting(t, group.Questions(), SourceChatOverride, "explicit empty question bank")
	requireEqual(t, group.ChannelInviteURL(), Setting[string]{Value: "", Source: SourceChatOverride}, "explicit empty channel invite")
	fallbackSetting := group.FallbackQuestions()
	requireDeepEqual(t, fallbackSetting.Value, fallback, "fallback questions")
	requireEqual(t, fallbackSetting.Source, SourceChatOverride, "fallback question source")
	requireEqual(t, group.FallbackBuiltin().Value, false, "fallback builtin")
	requireEqual(t, group.Lang().Value, "en", "fallback language")
	requireEqual(t, group.PrivateQueryPerMin(), Setting[int]{Value: 7, Source: SourceChatOverride}, "chat query rate")
	requireEqual(t, group.RichMessages(), Setting[bool]{Value: true, Source: SourceChatOverride}, "chat rich messages")
}

func TestSettingsSparseRestoreDropsEnabledOverride(t *testing.T) {
	settings, path, group, _ := reloadSparseOverrides(t)
	restore := group.Overrides()
	restore.Enabled = nil
	result, err := settings.Update(testGroupA, group.Revision(), restore)
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	requireEqual(t, result.Revision, uint64(2), "restored revision")
	requireEqual(t, group.Enabled(), Setting[bool]{Value: true, Source: SourceFactory}, "restored enabled")

	var raw map[string]any
	decodeFile(t, path, &raw)
	groups := raw["groups"].(map[string]any)
	record := groups["-1009000000001"].(map[string]any)
	if _, exists := record["enabled"]; exists {
		t.Fatalf("restored field remains in sparse record: %#v", record)
	}
	requireEqual(t, len(record["channel_whitelist"].([]any)), 0, "encoded empty whitelist length")
}

func disableLookupAutoDelete(t *testing.T) (*Store, CommitResult) {
	t.Helper()
	settings, err := NewStore("", testSettingsBaseline(), nil)
	requireNoError(t, err)
	group := requireSettingsView(t, settings, testGroupA)
	enabled := group.Overrides()
	enabled.LookupTTLSeconds = ptr(300)
	enabled.LookupAutoDeleteEnabled = ptr(true)
	first, err := settings.Update(group.ID(), group.Revision(), enabled)
	requireNoError(t, err)
	requireEqual(t, first.Revision, uint64(1), "enable revision")

	group = requireSettingsView(t, settings, testGroupA)
	disabled := group.Overrides()
	disabled.LookupAutoDeleteEnabled = ptr(false)
	second, err := settings.Update(group.ID(), group.Revision(), disabled)
	requireNoError(t, err)
	return settings, second
}

func TestSettingsDisabledLookupAutoDeleteKeepsTTL(t *testing.T) {
	settings, _ := disableLookupAutoDelete(t)
	group := requireSettingsView(t, settings, testGroupA)
	requireEqual(t, group.LookupTTLSeconds(), Setting[int]{Value: 300, Source: SourceChatOverride}, "disabled remembered TTL")
}

func TestSettingsDisabledLookupAutoDeleteUsesOverrideSource(t *testing.T) {
	settings, _ := disableLookupAutoDelete(t)
	group := requireSettingsView(t, settings, testGroupA)
	requireEqual(t, group.LookupAutoDeleteEnabled(), Setting[bool]{Value: false, Source: SourceChatOverride}, "disabled lookup auto-delete")
}

func TestSettingsReenabledLookupAutoDeleteRestoresFactoryValue(t *testing.T) {
	settings, second := disableLookupAutoDelete(t)
	group := requireSettingsView(t, settings, testGroupA)
	reEnabled := group.Overrides()
	reEnabled.LookupAutoDeleteEnabled = ptr(true)
	third, err := settings.Update(group.ID(), group.Revision(), reEnabled)
	requireNoError(t, err)
	group = requireSettingsView(t, settings, testGroupA)
	requireEqual(t, second.Revision, uint64(2), "disable revision")
	requireEqual(t, third.Revision, uint64(3), "re-enable revision")
	requireEqual(t, group.LookupTTLSeconds(), Setting[int]{Value: 300, Source: SourceChatOverride}, "re-enabled TTL")
	requireEqual(t, group.LookupAutoDeleteEnabled(), Setting[bool]{Value: true, Source: SourceFactory}, "re-enabled lookup auto-delete")
}
