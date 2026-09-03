package settings

import (
	"os"
	"testing"
)

func baselineEqualGroupOverrides(baseline GroupBaseline) GroupOverrides {
	return GroupOverrides{
		DeliveryMode:            ptr(baseline.DeliveryMode.Value),
		VerifyMode:              ptr(baseline.VerifyMode.Value),
		NameSpoiler:             ptr(baseline.NameSpoiler.Value),
		BanSeconds:              ptr(baseline.BanSeconds.Value),
		MuteSeconds:             ptr(baseline.MuteSeconds.Value),
		VerifyInvited:           ptr(baseline.VerifyInvited.Value),
		WarnLimit:               ptr(baseline.WarnLimit.Value),
		LookupTTLSeconds:        ptr(baseline.LookupTTLSeconds.Value),
		TimeoutSeconds:          ptr(baseline.TimeoutSeconds.Value),
		VerifyMaxFails:          ptr(baseline.VerifyMaxFails.Value),
		VerifyRetrySeconds:      ptr(baseline.VerifyRetrySeconds.Value),
		AntispamEnabled:         ptr(baseline.AntispamEnabled.Value),
		ChannelWhitelist:        ptr(cloneInt64s(baseline.ChannelWhitelist.Value)),
		TrustedMemberGroupIDs:   ptr(cloneInt64s(baseline.TrustedMemberGroupIDs.Value)),
		KnownChatIDs:            ptr(cloneInt64s(baseline.KnownChatIDs.Value)),
		RequiredChannelID:       ptr(baseline.RequiredChannelID.Value),
		ChannelDisplay:          ptr(baseline.ChannelDisplay.Value),
		ChannelInviteURL:        ptr(baseline.ChannelInviteURL.Value),
		Questions:               ptr(cloneQuestions(baseline.Questions.Value)),
		FallbackQuestions:       ptr(cloneShortQuestions(baseline.FallbackQuestions.Value)),
		FallbackBuiltin:         ptr(baseline.FallbackBuiltin.Value),
		Lang:                    ptr(baseline.Lang.Value),
		RichMessages:            ptr(baseline.RichMessages.Value),
		PrivateQueryPerMin:      ptr(baseline.PrivateQueryPerMin.Value),
		AdminLogChatID:          ptr(baseline.AdminLogChatID.Value),
		RequiredChannelFailOpen: ptr(baseline.RequiredChannelFailOpen.Value),
	}
}

func TestSettingsBaselineEqualValuesDoNotPinChatOverrides(t *testing.T) {
	settings, path, initial := newSparseSettings(t)
	next := baselineEqualGroupOverrides(testSettingsBaseline().Groups[0])

	result, err := settings.Update(initial.ID(), initial.Revision(), next)
	if err != nil {
		t.Fatalf("baseline-equal save must not pin a chat override: %v", err)
	}
	requireEqual(t, result.Revision, initial.Revision(), "baseline-equal save must not advance the chat revision")
	requireDeepEqual(t, requireSettingsView(t, settings, initial.ID()).Overrides(), GroupOverrides{},
		"baseline-equal save must not pin a chat override")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("baseline-equal save created a pinned settings record: %v", err)
	}
}
