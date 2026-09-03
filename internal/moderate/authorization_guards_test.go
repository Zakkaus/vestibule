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

const (
	guardedGroupID   int64 = -1009000000801
	unguardedChatID  int64 = -1009000000802
	allowedChannelID int64 = -1009000000803
	guardCallerID    int64 = 7
	guardTargetID    int64 = 8
)

func guardTestConfig(groups ...settings.GroupConfig) *settings.Config {
	chatIDs := make([]int64, 0, len(groups))
	for _, group := range groups {
		chatIDs = append(chatIDs, group.ID)
	}
	return &settings.Config{
		GroupIDs:         chatIDs,
		Groups:           groups,
		BanSeconds:       3600,
		MuteSeconds:      3600,
		WarnLimit:        3,
		Lang:             "en",
		NotifyTTLSeconds: -1,
	}
}

func newGuardService(t *testing.T, cfg *settings.Config, telegram Telegram) *Service {
	t.Helper()
	service, err := New(testSettings(t, cfg), telegram, cfg, newWarningJSONStore(""))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// Whoever may run /bc owns the group's advert filter: a bare /bc switches sender-chat filtering
// off, and /bc allow whitelists an advertising channel and lifts its ban. Neither belongs to an
// ordinary member.
func TestBlockChannelRefusesACallerWhoIsNotAGroupAdministrator(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "a bare bc would switch the filter off", text: "/bc"},
		{name: "bc allow would whitelist and unban a channel", text: "/bc allow -1009000000803"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID, AntispamEnabled: boolPtr(true)})
			telegram := newFakeMod()
			telegram.memberByID = map[int64]telego.ChatMember{guardCallerID: &telego.ChatMemberMember{}}
			service := newTestService(t, cfg, telegram, "")

			service.BlockChannel(context.Background(), ChannelSenderCommand{
				ChatID: guardedGroupID, MessageID: 11, CallerID: guardCallerID, Text: tc.text,
			})

			if !service.antispamEnabled(guardedGroupID) {
				t.Error("an ordinary member switched the group's channel-sender filter off")
			}
			if service.channelWhitelisted(guardedGroupID, allowedChannelID) {
				t.Error("an ordinary member added a channel to the group's whitelist")
			}
			if len(telegram.senderUnbans) != 0 {
				t.Errorf("sender-chat unbans = %v, want none: an ordinary member lifted a channel ban",
					telegram.senderUnbans)
			}
			assertModerationNotifications(t, telegram, fakeModNotification{
				chatID: guardedGroupID,
				text:   i18n.Messages.Moderate.Common.CommandAdminOnly.Render(i18n.LangEN, "/bc"),
			})
		})
	}
}

// The positive control for the refusal above: the same two commands from an administrator do
// change the group, so the refusal is a statement about the caller and not about the command.
func TestBlockChannelFromAnAdministratorChangesTheGroup(t *testing.T) {
	cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID, AntispamEnabled: boolPtr(true)})
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{guardCallerID: &telego.ChatMemberAdministrator{}}
	service := newTestService(t, cfg, telegram, "")

	service.BlockChannel(context.Background(), ChannelSenderCommand{
		ChatID: guardedGroupID, MessageID: 11, CallerID: guardCallerID, Text: "/bc",
	})
	if service.antispamEnabled(guardedGroupID) {
		t.Fatal("a bare /bc from an administrator left the channel-sender filter switched on")
	}
	service.BlockChannel(context.Background(), ChannelSenderCommand{
		ChatID: guardedGroupID, MessageID: 12, CallerID: guardCallerID, Text: "/bc allow -1009000000803",
	})
	if !service.channelWhitelisted(guardedGroupID, allowedChannelID) {
		t.Fatal("/bc allow from an administrator did not whitelist the channel")
	}
}

// /bantime writes the group's ban policy. An ordinary member running /bantime 0 turns every
// later /ban and /sb into a permanent ban; /bantime 30s makes every ban expire before it bites.
func TestBanTimeRefusesACallerWhoIsNotAGroupAdministrator(t *testing.T) {
	cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID})
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{guardCallerID: &telego.ChatMemberMember{}}
	service := newTestService(t, cfg, telegram, "")

	message := moderationCommand(guardedGroupID, "/bantime 0")
	runFakeHandler(t, newAPITestBot(t, telegram), service.OnBanTime, telego.Update{Message: message})

	if got := service.banDuration(guardedGroupID); got != 3600 {
		t.Errorf("ban duration = %d, want 3600: an ordinary member rewrote the group's ban policy", got)
	}
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: guardedGroupID,
		text:   i18n.Messages.Moderate.Common.AdminOnly.For(i18n.LangEN),
	})
}

// The positive control for the refusal above.
func TestBanTimeFromAnAdministratorWritesTheGroupPolicy(t *testing.T) {
	cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID})
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{guardCallerID: &telego.ChatMemberAdministrator{}}
	service := newTestService(t, cfg, telegram, "")

	message := moderationCommand(guardedGroupID, "/bantime 30m")
	runFakeHandler(t, newAPITestBot(t, telegram), service.OnBanTime, telego.Update{Message: message})

	if got := service.banDuration(guardedGroupID); got != 1800 {
		t.Fatalf("ban duration = %d, want 1800 after an administrator set it", got)
	}
}

// targetLookupTelegram answers the caller's and the target's admin lookup separately, which a
// single-answer fake cannot: the property here is about the second lookup failing on its own.
type targetLookupTelegram struct {
	*fakeModBot
	admins    map[int64]bool
	errByUser map[int64]error
}

func (b *targetLookupTelegram) FreshAdmin(_ context.Context, _, userID int64) (bool, error) {
	if err := b.errByUser[userID]; err != nil {
		return false, err
	}
	return b.admins[userID], nil
}

// sensitiveAction is one command that punishes the person it is replying to.
type sensitiveAction struct {
	name  string
	text  string
	run   func(*Service, *th.Context, telego.Update) error
	acted func(*Service, *fakeModBot) bool
}

func sensitiveActions() []sensitiveAction {
	banned := func(_ *Service, bot *fakeModBot) bool { return bot.bans > 0 }
	return []sensitiveAction{
		{name: "ban", text: "/ban", run: (*Service).OnBan, acted: banned},
		{name: "purge", text: "/sb", run: (*Service).OnPurge, acted: banned},
		{name: "mute", text: "/mute", run: (*Service).OnMute, acted: func(_ *Service, bot *fakeModBot) bool {
			return bot.mutes > 0
		}},
		{name: "warn", text: "/warn", run: (*Service).OnWarn, acted: func(service *Service, _ *fakeModBot) bool {
			return service.warnings.counters[warningKey{groupID: guardedGroupID, userID: guardTargetID}] > 0
		}},
	}
}

func runSensitiveAction(t *testing.T, service *Service, telegram *targetLookupTelegram, action sensitiveAction) {
	t.Helper()
	message := moderationCommand(guardedGroupID, action.text)
	handler := func(ctx *th.Context, update telego.Update) error {
		return action.run(service, ctx, update)
	}
	runFakeHandler(t, newAPITestBot(t, telegram), handler, telego.Update{Message: message})
}

// The caller's admin lookup succeeds and the target's fails — a rate limit, a network blip. The
// command must stop there. Falling through treats "could not read" as "not an administrator",
// and a fellow administrator gets banned, muted or warned because Telegram was slow.
func TestSensitiveCommandsRefuseWhenTheTargetAdminStatusCannotBeRead(t *testing.T) {
	for _, action := range sensitiveActions() {
		t.Run(action.name, func(t *testing.T) {
			telegram := &targetLookupTelegram{
				fakeModBot: newFakeMod(),
				admins:     map[int64]bool{guardCallerID: true},
				errByUser:  map[int64]error{guardTargetID: errors.New("too many requests")},
			}
			service := newGuardService(t, guardTestConfig(settings.GroupConfig{ID: guardedGroupID}), telegram)
			runSensitiveAction(t, service, telegram, action)

			if action.acted(service, telegram.fakeModBot) {
				t.Errorf("%s punished a member whose administrator status could not be read", action.text)
			}
			assertModerationNotifications(t, telegram.fakeModBot, fakeModNotification{
				chatID: guardedGroupID,
				text:   i18n.Messages.Moderate.Common.TargetAdminCheckFailed.For(i18n.LangEN),
			})
			assertModerationCommandCleanup(t, telegram.fakeModBot)
		})
	}
}

// The positive control for the refusal above: the same commands, with the target's status
// readable, do punish. A refusal only means something once the readable case is seen to act.
func TestSensitiveCommandsActWhenTheTargetAdminStatusReadsBack(t *testing.T) {
	for _, action := range sensitiveActions() {
		t.Run(action.name, func(t *testing.T) {
			telegram := &targetLookupTelegram{
				fakeModBot: newFakeMod(),
				admins:     map[int64]bool{guardCallerID: true, guardTargetID: false},
			}
			service := newGuardService(t, guardTestConfig(settings.GroupConfig{ID: guardedGroupID}), telegram)
			runSensitiveAction(t, service, telegram, action)

			if !action.acted(service, telegram.fakeModBot) {
				t.Errorf("%s did not act on a readable non-administrator target", action.text)
			}
		})
	}
}

// guardedAction is one moderation entry point plus the text that drives it.
type guardedAction struct {
	name string
	text string
	run  func(*Service, *th.Context, telego.Update) error
}

func guardedActions() []guardedAction {
	return []guardedAction{
		{name: "ban", text: "/ban", run: (*Service).OnBan},
		{name: "purge", text: "/sb", run: (*Service).OnPurge},
		{name: "mute", text: "/mute", run: (*Service).OnMute},
		{name: "warn", text: "/warn", run: (*Service).OnWarn},
		{name: "clearwarn", text: "/clearwarn", run: (*Service).OnClearWarn},
		{name: "unmute", text: "/unmute", run: (*Service).OnUnmute},
		{name: "bantime", text: "/bantime 30m", run: (*Service).OnBanTime},
		{name: "bc", text: "/bc", run: func(service *Service, ctx *th.Context, update telego.Update) error {
			service.BlockChannel(ctx.Context(), ChannelSenderCommand{
				ChatID:    update.Message.Chat.ID,
				MessageID: update.Message.MessageID,
				CallerID:  update.Message.From.ID,
				Text:      update.Message.Text,
			})
			return nil
		}},
	}
}

func runGuardedAction(t *testing.T, service *Service, telegram *fakeModBot, action guardedAction, chatID int64) {
	t.Helper()
	message := moderationCommand(chatID, action.text)
	handler := func(ctx *th.Context, update telego.Update) error {
		return action.run(service, ctx, update)
	}
	runFakeHandler(t, newAPITestBot(t, telegram), handler, telego.Update{Message: message})
}

// This bot moderates the groups an operator registered, and nothing else. In a chat it was
// merely added to, that chat's own administrators must not be able to drive this bot's /ban,
// /sb, /mute, /warn, /clearwarn, /unmute, /bantime or /bc against its members — and every such
// ban would be permanent, because the ban duration is read from a group record that does not
// exist and comes back as zero.
func TestModerationCommandsAreInertInAChatThatIsNotAGuardedGroup(t *testing.T) {
	for _, action := range guardedActions() {
		t.Run(action.name, func(t *testing.T) {
			telegram := newFakeMod()
			telegram.memberByID = map[int64]telego.ChatMember{
				guardCallerID: &telego.ChatMemberAdministrator{},
				guardTargetID: &telego.ChatMemberMember{},
			}
			cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID})
			service := newTestService(t, cfg, telegram, "")
			runGuardedAction(t, service, telegram, action, unguardedChatID)

			if actions := telegram.bans + telegram.mutes + telegram.unmutes + telegram.unbans +
				telegram.senderBans + len(telegram.senderUnbans); actions != 0 {
				t.Errorf("%s moderated an unregistered chat: bans=%d mutes=%d unmutes=%d unbans=%d "+
					"sender bans=%d sender unbans=%d", action.text, telegram.bans, telegram.mutes,
					telegram.unmutes, telegram.unbans, telegram.senderBans, len(telegram.senderUnbans))
			}
			if telegram.deletes != 0 {
				t.Errorf("%s deleted %d messages in an unregistered chat, want none",
					action.text, telegram.deletes)
			}
			if len(telegram.notifications) != 0 {
				t.Errorf("%s answered in an unregistered chat: %#v", action.text, telegram.notifications)
			}
			if got := service.warnings.counters[warningKey{groupID: unguardedChatID, userID: guardTargetID}]; got != 0 {
				t.Errorf("%s recorded a warning in an unregistered chat: count = %d", action.text, got)
			}
		})
	}
}

// The positive control for the refusal above: in the group the operator did register, the same
// commands are handled, so silence in an unregistered chat is about the chat and not the input.
func TestModerationCommandsAreHandledInAGuardedGroup(t *testing.T) {
	for _, action := range guardedActions() {
		t.Run(action.name, func(t *testing.T) {
			telegram := newFakeMod()
			telegram.memberByID = map[int64]telego.ChatMember{
				guardCallerID: &telego.ChatMemberAdministrator{},
				guardTargetID: &telego.ChatMemberMember{},
			}
			cfg := guardTestConfig(settings.GroupConfig{ID: guardedGroupID})
			service := newTestService(t, cfg, telegram, "")
			runGuardedAction(t, service, telegram, action, guardedGroupID)

			if telegram.deletes == 0 {
				t.Errorf("%s was not handled in a guarded group: the command message survived", action.text)
			}
		})
	}
}
