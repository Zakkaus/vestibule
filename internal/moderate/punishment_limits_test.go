package moderate

import (
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
)

func newPunishmentTestService(t *testing.T, telegram *fakeModBot) *Service {
	t.Helper()
	telegram.memberByID = map[int64]telego.ChatMember{
		guardCallerID: &telego.ChatMemberAdministrator{},
		guardTargetID: &telego.ChatMemberMember{},
	}
	return newTestService(t, guardTestConfig(settings.GroupConfig{ID: guardedGroupID}), telegram, "")
}

func runModerationText(t *testing.T, service *Service, telegram *fakeModBot, action guardedAction) {
	t.Helper()
	runGuardedAction(t, service, telegram, action, guardedGroupID)
}

// A Telegram mute with no expiry is lifted only by a human. /mute perm, /mute 0 and /mute
// rubbish all read as "no expiry" once the duration is committed unchecked, so the member is
// silenced indefinitely by a command the administrator thought was a typo.
func TestMuteRefusesADurationThatWouldSilenceTheMemberIndefinitely(t *testing.T) {
	for _, arg := range []string{"perm", "0", "rubbish", "-5"} {
		t.Run(arg, func(t *testing.T) {
			telegram := newFakeMod()
			service := newPunishmentTestService(t, telegram)
			runModerationText(t, service, telegram, guardedAction{
				name: "mute", text: "/mute " + arg, run: (*Service).OnMute,
			})

			if telegram.mutes != 0 {
				t.Errorf("mute calls = %d for %d seconds, want none: /mute %s must not restrict a "+
					"member with no expiry", telegram.mutes, telegram.lastMuteSeconds, arg)
			}
			assertModerationNotifications(t, telegram, fakeModNotification{
				chatID: guardedGroupID,
				text: i18n.Messages.Moderate.Mute.Usage.Render(i18n.LangEN,
					tgfmt.ModerationBanDurationStatus(i18n.LangEN, 3600)),
			})
			assertModerationCommandCleanup(t, telegram)
		})
	}
}

// The positive control for the refusal above: a duration /mute can read is applied.
func TestMuteAppliesADurationItCouldRead(t *testing.T) {
	telegram := newFakeMod()
	service := newPunishmentTestService(t, telegram)
	runModerationText(t, service, telegram, guardedAction{
		name: "mute", text: "/mute 30m", run: (*Service).OnMute,
	})

	if telegram.mutes != 1 || telegram.lastMuteSeconds != 1800 {
		t.Fatalf("mute calls = %d for %d seconds, want one 1800-second mute",
			telegram.mutes, telegram.lastMuteSeconds)
	}
}

func assertBanTimeRefused(t *testing.T, service *Service, telegram *fakeModBot, arg string) {
	t.Helper()
	if got := service.banDuration(guardedGroupID); got != 3600 {
		t.Errorf("ban duration = %d after /bantime %s, want 3600 left alone: a duration the bot "+
			"could not read became the group's ban policy", got, arg)
	}
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: guardedGroupID,
		text:   i18n.Messages.Moderate.BanTime.Usage.For(i18n.LangEN),
	})
}

// A /bantime argument the bot cannot read must not be committed. Committed, it writes zero
// seconds, which is Telegram's permanent ban: every later /ban and /sb in the group becomes
// permanent, and the administrator is told the value they typed was accepted.
func TestBanTimeRefusesAnArgumentItCouldNotRead(t *testing.T) {
	for _, arg := range []string{"abc", "5x", "1.5h", "-5"} {
		t.Run(arg, func(t *testing.T) {
			telegram := newFakeMod()
			service := newPunishmentTestService(t, telegram)
			runModerationText(t, service, telegram, guardedAction{
				name: "bantime", text: "/bantime " + arg, run: (*Service).OnBanTime,
			})
			assertBanTimeRefused(t, service, telegram, arg)
		})
	}
}

// A duration larger than Telegram can express is the same hazard by a different route: the
// product of the value and its unit is clamped away to zero, so /bantime 99999999999 reads as
// "permanent" rather than as a refusal, and nobody is told.
func TestBanTimeRefusesADurationTooLargeToExpress(t *testing.T) {
	for _, arg := range []string{"99999999999", "2147483649", "9999999999d"} {
		t.Run(arg, func(t *testing.T) {
			if seconds, ok := parseBanDuration(arg); ok {
				t.Errorf("parseBanDuration(%q) = (%d,true), want a refusal: an out-of-range "+
					"duration must not be read as a value", arg, seconds)
			}
			telegram := newFakeMod()
			service := newPunishmentTestService(t, telegram)
			runModerationText(t, service, telegram, guardedAction{
				name: "bantime", text: "/bantime " + arg, run: (*Service).OnBanTime,
			})
			assertBanTimeRefused(t, service, telegram, arg)
		})
	}
}

// The positive control for the two refusals above: an in-range duration is committed and
// reported, so a refusal is about the argument rather than about /bantime being inert.
func TestBanTimeCommitsADurationItCouldRead(t *testing.T) {
	telegram := newFakeMod()
	service := newPunishmentTestService(t, telegram)
	runModerationText(t, service, telegram, guardedAction{
		name: "bantime", text: "/bantime 7d", run: (*Service).OnBanTime,
	})

	if got := service.banDuration(guardedGroupID); got != 604800 {
		t.Fatalf("ban duration = %d, want 604800 after /bantime 7d", got)
	}
	l := i18n.LangEN
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: guardedGroupID,
		text: i18n.Messages.Moderate.BanTime.Set.Render(l,
			tgfmt.ModerationBanDurationStatus(l, 604800),
			i18n.Messages.Moderate.BanTime.TemporaryDescription.For(l)),
	})
}

// An unmute Telegram refused leaves the member silenced. Reporting it as done sends the
// administrator away believing the member can speak, and nobody is left watching for it.
func TestUnmuteFailureTellsTheAdministratorTheMemberIsStillSilenced(t *testing.T) {
	telegram := newFakeMod()
	telegram.unmuteErr = errors.New("telegram rejected the unmute")
	service := newPunishmentTestService(t, telegram)
	runModerationText(t, service, telegram, guardedAction{
		name: "unmute", text: "/unmute", run: (*Service).OnUnmute,
	})

	if telegram.unmutes != 1 {
		t.Fatalf("unmute calls = %d, want one rejected attempt", telegram.unmutes)
	}
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: guardedGroupID,
		text:   i18n.Messages.Moderate.Mute.UnmuteFailed.For(i18n.LangEN),
	})
}

// The positive control for the report above: a successful unmute reports the lift instead.
func TestUnmuteSuccessReportsTheLift(t *testing.T) {
	telegram := newFakeMod()
	service := newPunishmentTestService(t, telegram)
	message := moderationCommand(guardedGroupID, "/unmute")
	runModerationText(t, service, telegram, guardedAction{
		name: "unmute", text: "/unmute", run: (*Service).OnUnmute,
	})

	l := i18n.LangEN
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: guardedGroupID,
		text: i18n.Messages.Moderate.Mute.Unmuted.Render(l,
			tgfmt.DisplayName(message.ReplyToMessage.From), guardTargetID,
			tgfmt.DisplayName(message.From)),
	})
}
