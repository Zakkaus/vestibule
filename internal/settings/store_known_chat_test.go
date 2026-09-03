package settings

import "testing"

func TestStoreKnownChatsIncludeEveryEffectiveReference(t *testing.T) {
	baseline := testSettingsBaseline()
	baseline.Groups[0].RequiredChannelID = BaselineValue[int64]{Value: -1009000000201, Source: SourceUserFile}
	baseline.Groups[0].AdminLogChatID = BaselineValue[int64]{Value: -1009000000202, Source: SourceUserFile}
	baseline.Groups[0].KnownChatIDs = BaselineValue[[]int64]{Value: []int64{-1009000000203}, Source: SourceUserFile}
	baseline.Groups[0].TrustedMemberGroupIDs = BaselineValue[[]int64]{Value: []int64{-1009000000204}, Source: SourceUserFile}
	settings, err := NewStore("", baseline, nil)
	requireNoError(t, err)

	tests := []struct {
		name string
		id   int64
		want bool
		harm string
	}{
		{"effective group", testGroupA, true, "the bot would auto-leave an effective group"},
		{"required channel", -1009000000201, true, "the bot would auto-leave its required channel"},
		{"admin log chat", -1009000000202, true, "the bot would auto-leave its audit trail"},
		{"known chat", -1009000000203, true, "the bot would auto-leave an allowlisted support chat"},
		{"trusted member group", -1009000000204, true, "the bot would auto-leave a verification bypass source"},
		{"zero", 0, false, "chat zero must never become authorised"},
		{"unrelated chat", -1009000000205, false, "an unrelated chat must remain unauthorised"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := settings.IsKnownChat(test.id); got != test.want {
				t.Fatalf("%s: IsKnownChat(%d) = %v, want %v", test.harm, test.id, got, test.want)
			}
		})
	}
}
