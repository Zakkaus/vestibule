package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/edition"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verify"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)
func routeRecorder(name string, handled *[]string) th.Handler {
	return func(_ *th.Context, _ telego.Update) error {
		*handled = append(*handled, name)
		return nil
	}
}

func recordingHandlers(fixture *dispatchFixture, handled *[]string) telegram.HandlerSet {
	handlers := telegramHandlers(
		fixture.verification,
		fixture.administration,
		fixture.moderation,
		fixture.lookups,
	)
	recordVerificationHandlers(&handlers.Verification, handled)
	recordPanelHandlers(&handlers.Panel, handled)
	recordModerationHandlers(&handlers.Moderation, handled)
	recordLookupHandlers(&handlers.Lookup, handled)
	return handlers
}

func recordVerificationHandlers(handlers *telegram.VerificationHandlers, handled *[]string) {
	handlers.Answer = routeRecorder("verify.answer", handled)
	handlers.AdminAction = routeRecorder("verify.admin_action", handled)
	handlers.ChannelRecheck = routeRecorder("verify.channel_recheck", handled)
	handlers.JoinRequest = routeRecorder("verify.join_request", handled)
	handlers.MemberJoined = routeRecorder("verify.member_joined", handled)
	handlers.KernelAnswer = routeRecorder("verify.kernel_answer", handled)
}

func recordPanelHandlers(handlers *telegram.PanelHandlers, handled *[]string) {
	handlers.SettingsCallback = routeRecorder("panel.settings_callback", handled)
	handlers.ChatShared = routeRecorder("panel.chat_shared", handled)
	handlers.Input = routeRecorder("panel.input", handled)
	handlers.Ping = routeRecorder("panel.ping", handled)
	handlers.Start = routeRecorder("panel.start", handled)
	handlers.Settings = routeRecorder("panel.settings", handled)
	handlers.Stop = routeRecorder("panel.stop", handled)
	handlers.Stats = routeRecorder("panel.stats", handled)
	handlers.Rich = routeRecorder("panel.rich", handled)
	handlers.Spoiler = routeRecorder("panel.spoiler", handled)
	handlers.VerifyMode = routeRecorder("panel.vmode", handled)
	handlers.AutoDelete = routeRecorder("panel.autodel", handled)
	handlers.BanTime = routeRecorder("panel.bantime", handled)
	handlers.Help = routeRecorder("panel.help", handled)
}

func recordModerationHandlers(handlers *telegram.ModerationHandlers, handled *[]string) {
	handlers.Purge = routeRecorder("moderate.sb", handled)
	handlers.Ban = routeRecorder("moderate.ban", handled)
	handlers.Warn = routeRecorder("moderate.warn", handled)
	handlers.ClearWarn = routeRecorder("moderate.clearwarn", handled)
	handlers.BlockChannel = routeRecorder("moderate.bc", handled)
	handlers.Mute = routeRecorder("moderate.mute", handled)
	handlers.Unmute = routeRecorder("moderate.unmute", handled)
}

func recordLookupHandlers(handlers *telegram.LookupHandlers, handled *[]string) {
	handlers.Package = routeRecorder("lookup.pkg", handled)
	handlers.Use = routeRecorder("lookup.use", handled)
	handlers.Bug = routeRecorder("lookup.bug", handled)
	handlers.News = routeRecorder("lookup.news", handled)
	handlers.Wiki = routeRecorder("lookup.wiki", handled)
	handlers.Forum = routeRecorder("lookup.bbs", handled)
	handlers.Distros = routeRecorder("lookup.distros", handled)
	handlers.Arm = routeRecorder("lookup.arm", handled)
	handlers.ArmPkgs = routeRecorder("lookup.armpkgs", handled)
	handlers.Kernel = routeRecorder("lookup.kernel", handled)
	handlers.Man = routeRecorder("lookup.man", handled)
	handlers.CVE = routeRecorder("lookup.cve", handled)
	handlers.Repology = routeRecorder("lookup.repology", handled)
}

func dispatchRouteNames(t *testing.T, fixture *dispatchFixture, update telego.Update) []string {
	t.Helper()
	var handled []string
	application := telegram.NewUpdates(
		fixture.cfg,
		fixture.settings,
		fixture.connector,
		recordingHandlers(fixture, &handled),
	)
	handler, err := th.NewBotHandler(fixture.bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	application.Register(handler)
	if err := handler.BaseGroup().HandleUpdate(context.Background(), fixture.bot, update); err != nil {
		t.Fatal(err)
	}
	return handled
}

type dispatchCase struct {
	name   string
	update telego.Update
	want   string
}

func callbackUpdate(prefix string) telego.Update {
	return telego.Update{CallbackQuery: &telego.CallbackQuery{Data: prefix + "payload"}}
}

func privateCommand(userID int64, text string) telego.Update {
	return telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: userID},
		Text: text,
	}}
}

func groupCommand(groupID, userID int64, text string) telego.Update {
	return telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup},
		From: &telego.User{ID: userID},
		Text: text,
	}}
}

func globalDispatchCases(
	fixture *dispatchFixture,
	panelInput telego.Update,
	kernelAnswer telego.Update,
	panelUser int64,
) []dispatchCase {
	return []dispatchCase{
		{name: "join request", update: fixture.joinRequest(803), want: "verify.join_request"},
		{name: "answer callback", update: callbackUpdate(verify.AnswerCallbackPrefix), want: "verify.answer"},
		{name: "admin callback", update: callbackUpdate(verify.AdminCallbackPrefix), want: "verify.admin_action"},
		{name: "channel recheck callback", update: callbackUpdate(verify.ChannelRecheckCallbackPrefix), want: "verify.channel_recheck"},
		{name: "settings callback", update: callbackUpdate(panel.SettingsCallbackPrefix), want: "panel.settings_callback"},
		{name: "panel input", update: panelInput, want: "panel.input"},
		{name: "kernel answer", update: kernelAnswer, want: "verify.kernel_answer"},
		{name: "panel start payload", update: privateCommand(panelUser, "/start panel_token"), want: "panel.start"},
		{name: "scoped verification start payload", update: privateCommand(panelUser, "/start verify_-100"), want: "panel.start"},
		{name: "bare verification start payload", update: privateCommand(panelUser, "/start verify"), want: "panel.start"},
		{name: "start without payload", update: privateCommand(panelUser, "/start"), want: "panel.start"},
		{
			name: "group command",
			update: groupCommand(
				fixture.groupID,
				panelUser,
				"/"+edition.CommandPrefix+"pkg sys-apps/portage",
			),
			want: "lookup.pkg",
		},
	}
}

func TestGlobalDispatchRunsOnlyTheIntendedHandler(t *testing.T) {
	const panelUser int64 = 801
	const kernelUser int64 = 802
	fixture := newDispatchFixture(t, 0)
	panelInput := fixture.preparePanelInput(t, panelUser)
	runDirectHandler(t, fixture.bot, fixture.verification.OnJoinRequest, fixture.joinRequest(kernelUser))
	kernelAnswer := privateCommand(kernelUser, "6.12.3")
	if !fixture.verification.KernelAnswerDM(context.Background(), kernelAnswer) {
		t.Fatal("kernel fixture did not establish a gradeable private answer")
	}
	for _, test := range globalDispatchCases(fixture, panelInput, kernelAnswer, panelUser) {
		t.Run(test.name, func(t *testing.T) {
			got := dispatchRouteNames(t, fixture, test.update)
			if len(got) != 1 || got[0] != test.want {
				t.Fatalf("handlers = %v, want only %q", got, test.want)
			}
		})
	}
	assertModerationStopsRoutes(t, fixture)
}

func assertModerationStopsRoutes(t *testing.T, fixture *dispatchFixture) {
	t.Helper()
	beforeDelete := fixture.caller.methodCount("deleteMessage")
	beforeBan := fixture.caller.methodCount("banChatSenderChat")
	moderated := telego.Update{Message: &telego.Message{
		MessageID: 99,
		Chat:      telego.Chat{ID: fixture.groupID, Type: telego.ChatTypeSupergroup},
		SenderChat: &telego.Chat{
			ID:    -1009000000999,
			Type:  telego.ChatTypeChannel,
			Title: "Untrusted sender",
		},
		Text: "channel post",
	}}
	if got := dispatchRouteNames(t, fixture, moderated); len(got) != 0 {
		t.Fatalf("moderated sender-channel post reached route handlers %v", got)
	}
	if got := fixture.caller.methodCount("deleteMessage") - beforeDelete; got != 1 {
		t.Fatalf("moderation delete calls = %d, want 1", got)
	}
	if got := fixture.caller.methodCount("banChatSenderChat") - beforeBan; got != 1 {
		t.Fatalf("moderation ban calls = %d, want 1", got)
	}
}

func TestVerificationPublicHandlerBoundaries(t *testing.T) {
	const applicantID int64 = 811
	requiredChannel := int64(-1009000000812)
	channelFixture := newDispatchFixture(t, requiredChannel)
	channelFixture.caller.setMember(requiredChannel, applicantID, &telego.ChatMemberLeft{
		Status: telego.MemberStatusLeft,
		User:   telego.User{ID: applicantID},
	})
	runDirectHandler(t, channelFixture.bot, channelFixture.verification.OnJoinRequest, channelFixture.joinRequest(applicantID))
	beforeMessages := len(channelFixture.caller.sentTexts())
	channelFixture.caller.setMember(requiredChannel, applicantID, &telego.ChatMemberMember{
		Status: telego.MemberStatusMember,
		User:   telego.User{ID: applicantID},
	})
	channelCallback := telego.Update{CallbackQuery: &telego.CallbackQuery{
		ID:   "channel-recheck",
		From: telego.User{ID: applicantID, LanguageCode: "en"},
		Data: fmt.Sprintf("%s%d:%d", verify.ChannelRecheckCallbackPrefix, channelFixture.groupID, applicantID),
	}}
	channelHandler, err := th.NewBotHandler(channelFixture.bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	channelFixture.application.Register(channelHandler)
	if err := channelHandler.BaseGroup().HandleUpdate(context.Background(), channelFixture.bot, channelCallback); err != nil {
		t.Fatal(err)
	}
	answers := channelFixture.caller.callbackAnswers()
	if len(answers) != 1 ||
		answers[0].Text != i18n.Messages.Verification.Channel.ContinueOK.For(i18n.LangEN) {
		t.Fatalf("channel recheck answers = %#v, want one catalogue continuation acknowledgement", answers)
	}
	messages := channelFixture.caller.sentTexts()
	if len(messages) != beforeMessages+1 ||
		!strings.Contains(messages[len(messages)-1], i18n.Messages.Verification.Challenge.KernelQuestion.For(i18n.LangEN)) {
		t.Fatalf("channel recheck messages = %v, want one catalogue kernel challenge", messages[beforeMessages:])
	}

	kernelFixture := newDispatchFixture(t, 0)
	kernelHandler, err := th.NewBotHandler(kernelFixture.bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	kernelFixture.application.Register(kernelHandler)
	if err := kernelHandler.BaseGroup().HandleUpdate(
		context.Background(), kernelFixture.bot, kernelFixture.joinRequest(applicantID),
	); err != nil {
		t.Fatal(err)
	}
	answer := telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: applicantID, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: applicantID, LanguageCode: "en"},
		Text: "6.12.3",
	}}
	if !kernelFixture.verification.KernelAnswerDM(context.Background(), answer) {
		t.Fatal("kernel public-handler fixture did not establish a gradeable answer")
	}
	beforeApprove := kernelFixture.caller.methodCount("approveChatJoinRequest")
	beforeTexts := len(kernelFixture.caller.sentTexts())
	if err := kernelHandler.BaseGroup().HandleUpdate(context.Background(), kernelFixture.bot, answer); err != nil {
		t.Fatal(err)
	}
	if got := kernelFixture.caller.methodCount("approveChatJoinRequest") - beforeApprove; got != 1 {
		t.Fatalf("kernel answer approvals = %d, want 1", got)
	}
	autoReply := i18n.Messages.Bot.DirectMessage.AutoReply.Render(i18n.LangEN, 4)
	for _, text := range kernelFixture.caller.sentTexts()[beforeTexts:] {
		if text == autoReply {
			t.Fatalf("kernel answer fell through to the generic private responder: %q", text)
		}
	}
}

func boolPtr(v bool) *bool { return &v }
