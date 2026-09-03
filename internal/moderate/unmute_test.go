package moderate

import (
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestUnmuteReportsRestrictionLiftOutcomeToAdministrator(t *testing.T) {
	const groupID = int64(-1009123456789)
	l := i18n.LangEN
	tests := []struct {
		name        string
		unmuteErr   error
		wantText    string
		outcome     string
		memberState string
	}{
		{
			name:        "Telegram rejects the restriction lift",
			unmuteErr:   errors.New("telegram rejected unmute"),
			wantText:    i18n.Messages.Moderate.Mute.UnmuteFailed.For(l),
			outcome:     "rejected",
			memberState: "the member remains muted",
		},
		{
			name:        "Telegram lifts the restriction",
			wantText:    i18n.Messages.Moderate.Mute.Unmuted.Render(l, "Member", 8, "Admin"),
			outcome:     "accepted",
			memberState: "the member was unmuted",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			telegram := newFakeMod()
			telegram.unmuteErr = test.unmuteErr
			telegram.memberByID = map[int64]telego.ChatMember{
				7: &telego.ChatMemberAdministrator{},
				8: &telego.ChatMemberMember{},
			}
			service := newTestService(t, &settings.Config{
				GroupIDs:         []int64{groupID},
				Groups:           []settings.GroupConfig{{ID: groupID}},
				Lang:             "en",
				NotifyTTLSeconds: -1,
			}, telegram, "")
			message := moderationCommand(groupID, "/unmute")
			runFakeHandler(t, newAPITestBot(t, telegram), service.OnUnmute, telego.Update{Message: message})

			if telegram.unmutes != 1 {
				t.Fatalf("/unmute calls = %d, want 1", telegram.unmutes)
			}
			assertModerationCommandCleanup(t, telegram)
			wantNotification := fakeModNotification{chatID: groupID, text: test.wantText}
			if len(telegram.notifications) != 1 {
				t.Fatalf("Telegram %s /unmute; %s, but the administrator received %d notifications, want %#v", test.outcome, test.memberState, len(telegram.notifications), wantNotification)
			}
			if got := telegram.notifications[0]; got != wantNotification {
				t.Fatalf("Telegram %s /unmute; %s, but the administrator notification = %#v, want %#v", test.outcome, test.memberState, got, wantNotification)
			}
		})
	}
}
