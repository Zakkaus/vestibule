package moderate

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

const authorizationTestGroupID int64 = -1009000000501

type authorizationCheck struct {
	chatID int64
	userID int64
}

type authorizationTrackingTelegram struct {
	*fakeModBot
	freshAdmin  bool
	freshErr    error
	cachedAdmin bool
	cachedErr   error
	// Both lookups are fresh now, so a single answer for all of them cannot tell the
	// caller from the target: everyone an administrator means the command refuses to
	// touch its target, and nobody one means it refuses its caller.
	admins       map[int64]bool
	freshChecks  []authorizationCheck
	cachedChecks []authorizationCheck
}

func (b *authorizationTrackingTelegram) FreshAdmin(_ context.Context, chatID, userID int64) (bool, error) {
	b.freshChecks = append(b.freshChecks, authorizationCheck{chatID: chatID, userID: userID})
	if b.admins != nil {
		return b.admins[userID], b.freshErr
	}
	return b.freshAdmin, b.freshErr
}

func (b *authorizationTrackingTelegram) CachedAdmin(_ context.Context, chatID, userID int64) (bool, error) {
	b.cachedChecks = append(b.cachedChecks, authorizationCheck{chatID: chatID, userID: userID})
	return b.cachedAdmin, b.cachedErr
}

type authorizationAction struct {
	name    string
	text    string
	handler func(*Service, *th.Context, telego.Update) error
	calls   func(*fakeModBot) int
}

func authorizationActions() []authorizationAction {
	return []authorizationAction{
		{name: "ban", text: "/ban", handler: (*Service).OnBan, calls: func(bot *fakeModBot) int { return bot.bans }},
		{name: "mute", text: "/mute", handler: (*Service).OnMute, calls: func(bot *fakeModBot) int { return bot.mutes }},
	}
}

func newAuthorizationTestService(t *testing.T, telegram Telegram) *Service {
	t.Helper()
	cfg := &settings.Config{
		GroupIDs:         []int64{authorizationTestGroupID},
		Groups:           []settings.GroupConfig{{ID: authorizationTestGroupID}},
		BanSeconds:       3600,
		MuteSeconds:      3600,
		Lang:             "en",
		NotifyTTLSeconds: -1,
	}
	service, err := New(testSettings(t, cfg), telegram, cfg, newWarningJSONStore(""))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestBanAndMuteAuthorizationChecksCallerAndTargetFresh(t *testing.T) {
	for _, action := range authorizationActions() {
		t.Run(action.name, func(t *testing.T) {
			message := moderationCommand(authorizationTestGroupID, action.text)
			telegram := &authorizationTrackingTelegram{
				fakeModBot: newFakeMod(),
				admins:     map[int64]bool{message.From.ID: true},
			}
			service := newAuthorizationTestService(t, telegram)
			handler := func(ctx *th.Context, update telego.Update) error {
				return action.handler(service, ctx, update)
			}
			runFakeHandler(t, newAPITestBot(t, telegram), handler, telego.Update{Message: message})

			wantCaller := authorizationCheck{chatID: authorizationTestGroupID, userID: message.From.ID}
			wantTarget := authorizationCheck{chatID: authorizationTestGroupID, userID: message.ReplyToMessage.From.ID}
			want := []authorizationCheck{wantCaller, wantTarget}
			if len(telegram.freshChecks) != len(want) ||
				telegram.freshChecks[0] != want[0] || telegram.freshChecks[1] != want[1] {
				t.Errorf("fresh admin checks = %v, want %v: a sensitive command rechecks the caller "+
					"and the target, in that order", telegram.freshChecks, want)
			}
			if len(telegram.cachedChecks) != 0 {
				t.Errorf("cached admin checks = %v, want none: a revoked administrator would stay "+
					"protected for the life of the cache entry", telegram.cachedChecks)
			}
			if got := action.calls(telegram.fakeModBot); got != 1 {
				t.Errorf("%s calls = %d, want 1 after an administrator authorized the command", action.name, got)
			}
		})
	}
}

func TestBanAndMuteAuthorizationLookupFailureLeavesTargetEvidence(t *testing.T) {
	for _, action := range authorizationActions() {
		t.Run(action.name, func(t *testing.T) {
			telegram := &authorizationTrackingTelegram{
				fakeModBot: newFakeMod(),
				freshErr:   errors.New("membership lookup unavailable"),
			}
			service := newAuthorizationTestService(t, telegram)
			message := moderationCommand(authorizationTestGroupID, action.text)
			handler := func(ctx *th.Context, update telego.Update) error {
				return action.handler(service, ctx, update)
			}
			runFakeHandler(t, newAPITestBot(t, telegram), handler, telego.Update{Message: message})

			if got := action.calls(telegram.fakeModBot); got != 0 {
				t.Errorf("%s calls = %d, want 0: an unreadable caller check must fail closed", action.name, got)
			}
			if len(telegram.cachedChecks) != 0 {
				t.Errorf("target checks = %v, want none after caller authorization failed", telegram.cachedChecks)
			}
			if len(telegram.deletedMessageIDs) != 1 || telegram.deletedMessageIDs[0] != message.MessageID {
				t.Errorf("deleted message IDs = %v, want only command %d: failed authorization must leave target evidence", telegram.deletedMessageIDs, message.MessageID)
			}
			wantNotice := i18n.Messages.Moderate.Common.CallerAdminCheckFailed.For(i18n.LangEN)
			if len(telegram.notifications) != 1 || telegram.notifications[0] != (fakeModNotification{chatID: authorizationTestGroupID, text: wantNotice}) {
				t.Errorf("notifications = %#v, want caller-check failure notice", telegram.notifications)
			}
		})
	}
}
