package settings

import "testing"

func TestSettingsChatOverridesWinForVerificationAndSupportSettings(t *testing.T) {
	settings, err := NewStore("", testSettingsBaseline(), nil)
	requireNoError(t, err)

	before := requireSettingsView(t, settings, testGroupA)
	trustedGroups := []int64{-1009000000101}
	next := before.Overrides()
	next.VerifyMaxFails = ptr(8)
	next.VerifyRetrySeconds = ptr(90)
	next.VerifyInvited = ptr(false)
	next.TrustedMemberGroupIDs = &trustedGroups
	next.AdminLogChatID = ptr(int64(-1009000000102))
	next.RequiredChannelFailOpen = ptr(false)
	_, err = settings.Update(before.ID(), before.Revision(), next)
	requireNoError(t, err)

	after := requireSettingsView(t, settings, testGroupA)
	requireEqual(t, after.VerifyMaxFails(), Setting[int]{Value: 8, Source: SourceChatOverride},
		"verify_max_fails override would leave the brute-force budget at its baseline")
	requireEqual(t, after.VerifyRetrySeconds(), Setting[int]{Value: 90, Source: SourceChatOverride},
		"verify_retry_seconds override would leave the retry delay at its baseline")
	requireEqual(t, after.VerifyInvited(), Setting[bool]{Value: false, Source: SourceChatOverride},
		"verify_invited override would keep invited members subject to the wrong policy")
	requireDeepEqual(t, after.TrustedMemberGroupIDs(), Setting[[]int64]{Value: trustedGroups, Source: SourceChatOverride},
		"trusted_member_group_ids override would keep the old verification bypass")
	requireEqual(t, after.AdminLogChatID(), Setting[int64]{Value: -1009000000102, Source: SourceChatOverride},
		"admin_log_chat_id override would send audit records to the abandoned chat")
	requireEqual(t, after.RequiredChannelFailOpen(), Setting[bool]{Value: false, Source: SourceChatOverride},
		"required_channel_fail_open override would admit members during a lookup outage")
}

func TestSettingsBuiltinFallbackHidesCustomQuestionsAndKeepsSource(t *testing.T) {
	staleQuestions := []ShortQuestion{{Q: "Package manager?", Answers: []string{"portage"}}}
	tests := []struct {
		name            string
		baselineBuiltin BaselineValue[bool]
		override        *bool
		wantSource      Source
	}{
		{
			name:            "baseline enables builtin fallback",
			baselineBuiltin: BaselineValue[bool]{Value: true, Source: SourceFactory},
			wantSource:      SourceFactory,
		},
		{
			name:            "chat override enables builtin fallback",
			baselineBuiltin: BaselineValue[bool]{Value: false, Source: SourceFactory},
			override:        ptr(true),
			wantSource:      SourceChatOverride,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := testSettingsBaseline().Groups[0]
			baseline.FallbackBuiltin = test.baselineBuiltin
			baseline.FallbackQuestions = BaselineValue[[]ShortQuestion]{
				Value:  cloneShortQuestions(staleQuestions),
				Source: SourceUserFile,
			}
			effective := buildEffectiveGroup(baseline, groupRecord{
				GroupOverrides: GroupOverrides{FallbackBuiltin: test.override},
			}, false)

			requireEqual(t, effective.fallbackBuiltin, Setting[bool]{Value: true, Source: test.wantSource},
				"fallback_builtin source")
			requireDeepEqual(t, effective.fallbackQuestions.Value, []ShortQuestion{},
				"builtin fallback would expose ignored custom questions to administrators")
			requireEqual(t, effective.fallbackQuestions.Source, test.wantSource,
				"builtin fallback question source")
		})
	}
}
