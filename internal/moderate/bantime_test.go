package moderate

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
)

func TestParseBanDuration(t *testing.T) {
	valid := map[string]int{
		"0": 0, "perm": 0, "permanent": 0,
		i18n.Messages.Moderate.Duration.PermanentInput.For(i18n.LangZH): 0,
		"30s": 30, "30m": 1800, "2h": 7200, "7d": 604800, "3600": 3600,
	}
	for input, want := range valid {
		if got, ok := parseBanDuration(input); !ok || got != want {
			t.Errorf("parseBanDuration(%q) = (%d,%v), want (%d,true)", input, got, ok, want)
		}
	}
	for input, want := range map[string]int{"10s": 30, "29s": 30, "5": 30, "30s": 30, "366d": 366 * 86400} {
		if got, ok := parseBanDuration(input); !ok || got != want {
			t.Errorf("clamp parseBanDuration(%q) = (%d,%v), want (%d,true)", input, got, ok, want)
		}
	}
	for _, input := range []string{"400d", "367d", "999999999"} {
		if got, ok := parseBanDuration(input); !ok || got != 0 {
			t.Errorf("over-366d parseBanDuration(%q) = (%d,%v), want (0,true)", input, got, ok)
		}
	}
	for _, input := range []string{"", "abc", "-5", "5x", "m", "1.5h"} {
		if _, ok := parseBanDuration(input); ok {
			t.Errorf("parseBanDuration(%q) should be invalid", input)
		}
	}
}

func TestBareBanTimeReportsCurrentGroupPolicy(t *testing.T) {
	const (
		groupID    int64 = -1009000000501
		banSeconds int   = 7200
	)
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{
		7: &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator},
	}
	service := newTestService(t, &settings.Config{
		GroupIDs:         []int64{groupID},
		Groups:           []settings.GroupConfig{{ID: groupID}},
		BanSeconds:       banSeconds,
		NotifyTTLSeconds: -1,
		Lang:             "en",
	}, telegram, "")

	runFakeHandler(t, newAPITestBot(t, telegram), service.OnBanTime, telego.Update{Message: &telego.Message{
		MessageID: 11,
		Chat:      telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup},
		From:      &telego.User{ID: 7, FirstName: "Admin"},
		Text:      "/bantime",
	}})

	l := service.groupLanguage(groupID)
	want := i18n.Messages.Moderate.BanTime.Current.Render(
		l,
		tgfmt.ModerationBanDurationStatus(l, banSeconds),
		i18n.Messages.Moderate.BanTime.TemporaryDescription.For(l),
		i18n.Messages.Moderate.BanTime.Usage.For(l),
	)
	if got := telegram.lastSendText; got != want {
		t.Fatalf("bare /bantime notice = %q, want current policy %q; administrators would not learn the group's current ban duration", got, want)
	}
}

func TestBanDurationBeyondInt32LimitIsRefused(t *testing.T) {
	if got, ok := parseBanDuration("3600"); !ok || got != 3600 {
		t.Fatalf("valid ban duration = (%d, %t), want (3600, true)", got, ok)
	}
	if got, ok := parseBanDuration("99999999999"); ok {
		t.Fatalf("out-of-range /bantime duration was accepted as %d seconds; it becomes a permanent ban policy instead of being refused", got)
	}
}

func TestMuteDurationPolicy(t *testing.T) {
	if seconds, ok := parseBanDuration("30m"); !ok || seconds != 1800 {
		t.Errorf("inline /mute 30m parse = (%d,%v), want (1800,true)", seconds, ok)
	}
	if seconds, _ := parseBanDuration("0"); seconds != 0 {
		t.Errorf("parseBanDuration(0) = %d, want 0 so /mute rejects it", seconds)
	}
}
