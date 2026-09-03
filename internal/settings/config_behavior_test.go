package settings

import "testing"

func TestConfigAdminLogAndFeedChatsStayOnAutoLeaveAllowlist(t *testing.T) {
	config := &Config{
		AdminLogChatID: -1009000000301,
		Feeds: []FeedConfig{
			{ChatID: -1009000000302},
			{ChatID: -1009000000303},
		},
	}
	for _, chat := range []struct {
		id   int64
		harm string
	}{
		{-1009000000301, "the bot would auto-leave the moderation audit chat"},
		{-1009000000302, "the bot would auto-leave the first feed destination"},
		{-1009000000303, "the bot would auto-leave a later feed destination"},
	} {
		if !config.IsKnownChat(chat.id) {
			t.Fatalf("%s: IsKnownChat(%d) = false", chat.harm, chat.id)
		}
	}
	if config.IsKnownChat(-1009000000304) {
		t.Fatal("an unrelated chat was added to the auto-leave allowlist")
	}
}

func TestConfigAbsentVerifyInvitedStillRequiresVerification(t *testing.T) {
	disabled, enabled := false, true
	tests := []struct {
		name  string
		value *bool
		want  bool
		harm  string
	}{
		{"absent", nil, true, "absent verify_invited would let invited accounts bypass verification"},
		{"explicitly disabled", &disabled, false, "explicit verify_invited=false was ignored"},
		{"explicitly enabled", &enabled, true, "explicit verify_invited=true was ignored"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{VerifyInvited: test.value}
			if got := config.VerifyInvitedMembers(); got != test.want {
				t.Fatalf("%s: VerifyInvitedMembers() = %v, want %v", test.harm, got, test.want)
			}
		})
	}
}
