package moderate

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
)

func TestPurgeNoticeReportsRevokedMessageHistory(t *testing.T) {
	const (
		groupID    int64 = -1009000000502
		banSeconds int   = 7200
	)
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{
		7: &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator},
		8: &telego.ChatMemberMember{},
	}
	service := newTestService(t, &settings.Config{
		GroupIDs:         []int64{groupID},
		Groups:           []settings.GroupConfig{{ID: groupID}},
		BanSeconds:       banSeconds,
		NotifyTTLSeconds: -1,
		Lang:             "en",
	}, telegram, "")

	runFakeHandler(t, newAPITestBot(t, telegram), service.OnPurge, telego.Update{
		Message: moderationCommand(groupID, "/sb"),
	})

	l := service.groupLanguage(groupID)
	action := i18n.Messages.Moderate.Ban.Action.Render(
		l,
		i18n.Messages.Moderate.Ban.PurgeVerb.For(l),
		tgfmt.ModerationBanDurationStatus(l, banSeconds),
	)
	want := i18n.Messages.Moderate.Ban.Applied.Render(
		l,
		action,
		"Member",
		8,
		"Admin",
	)
	if got := telegram.lastSendText; got != want {
		t.Fatalf("purge notice = %q, want %q; administrators would not learn that the message history was purged", got, want)
	}
}
