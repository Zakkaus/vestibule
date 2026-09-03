package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

const (
	gatewayTestChatID    int64 = -1009000004101
	gatewayTestOtherChat int64 = -1009000004102
	gatewayTestLogChat   int64 = -1009000004103
	gatewayTestAuditChat int64 = -1009000004104
	gatewayTestUserID    int64 = 4105
	gatewayTestOtherUser int64 = 4106
)

type gatewayLinkPreview struct {
	IsDisabled bool `json:"is_disabled"`
}

type gatewayInlineButton struct {
	Text         string `json:"text"`
	URL          string `json:"url"`
	CallbackData string `json:"callback_data"`
}

type gatewayReplyMarkup struct {
	InlineKeyboard [][]gatewayInlineButton `json:"inline_keyboard"`
}

type gatewaySendParams struct {
	ChatID             int64               `json:"chat_id"`
	Text               string              `json:"text"`
	ParseMode          string              `json:"parse_mode"`
	LinkPreviewOptions *gatewayLinkPreview `json:"link_preview_options"`
	ReplyMarkup        *gatewayReplyMarkup `json:"reply_markup"`
}

func decodeGatewayCall[T any](t *testing.T, calls []recordedCall, index int) T {
	t.Helper()
	if len(calls) <= index {
		t.Fatalf("recorded calls = %d, want index %d", len(calls), index)
	}
	var params T
	if err := json.Unmarshal(calls[index].body, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func TestVerificationGatewayRequiresAConnector(t *testing.T) {
	connector := newTestClient(t, &scriptedCaller{})
	if gateway := NewVerificationGateway(connector); gateway.connector != connector {
		t.Fatal("a valid connector was not retained by the verification gateway")
	}

	defer func() {
		if recover() == nil {
			t.Fatal("a nil connector produced a gateway that would panic on its first request")
		}
	}()
	NewVerificationGateway(nil)
}

func TestVerificationGatewaySendsPlainTextWithoutFormatting(t *testing.T) {
	caller := &scriptedCaller{}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	messageID, err := gateway.Send(context.Background(), verification.OutgoingMessage{
		ChatID: gatewayTestChatID,
		Text:   "plain result",
	})
	if err != nil || messageID != 101 {
		t.Fatalf("plain send = (%d, %v), want message 101", messageID, err)
	}
	calls := caller.methodCalls("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(calls))
	}
	got := decodeGatewayCall[gatewaySendParams](t, calls, 0)
	want := gatewaySendParams{ChatID: gatewayTestChatID, Text: "plain result"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("plain message shape = %#v, want %#v", got, want)
	}
}

func TestVerificationGatewaySendsHTMLButtonsWithoutPreviews(t *testing.T) {
	caller := &scriptedCaller{}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	messageID, err := gateway.Send(context.Background(), verification.OutgoingMessage{
		ChatID:             gatewayTestOtherChat,
		Text:               "<b>choose</b>",
		HTML:               true,
		DisableLinkPreview: true,
		Buttons: [][]verification.Button{
			{{Text: "docs", URL: "https://example.invalid/docs"}},
			{{Text: "accept", CallbackData: "verify:accept"}, {Text: "decline", CallbackData: "verify:decline"}},
		},
	})
	if err != nil || messageID != 101 {
		t.Fatalf("HTML send = (%d, %v), want message 101", messageID, err)
	}
	calls := caller.methodCalls("sendMessage")
	if len(calls) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(calls))
	}
	got := decodeGatewayCall[gatewaySendParams](t, calls, 0)
	want := gatewaySendParams{
		ChatID:             gatewayTestOtherChat,
		Text:               "<b>choose</b>",
		ParseMode:          telego.ModeHTML,
		LinkPreviewOptions: &gatewayLinkPreview{IsDisabled: true},
		ReplyMarkup: &gatewayReplyMarkup{InlineKeyboard: [][]gatewayInlineButton{
			{{Text: "docs", URL: "https://example.invalid/docs"}},
			{{Text: "accept", CallbackData: "verify:accept"}, {Text: "decline", CallbackData: "verify:decline"}},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("HTML message shape = %#v, want %#v", got, want)
	}
}

func TestVerificationGatewayUsesBothFallbackRenderings(t *testing.T) {
	parseErr := errors.New("Bad Request: can't parse entities")
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"sendMessage": {{err: parseErr}, {}},
	}}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	messageID, err := gateway.SendHTMLFallback(
		context.Background(), gatewayTestChatID, "<b>rich</b>", "<b>simpler</b>",
	)
	if err != nil || messageID != 101 {
		t.Fatalf("fallback send = (%d, %v), want message 101", messageID, err)
	}
	calls := caller.methodCalls("sendMessage")
	if len(calls) != 2 {
		t.Fatalf("fallback sendMessage calls = %d, want 2", len(calls))
	}
	first := decodeGatewayCall[gatewaySendParams](t, calls, 0)
	second := decodeGatewayCall[gatewaySendParams](t, calls, 1)
	if first.ChatID != gatewayTestChatID || first.Text != "<b>rich</b>" || first.ParseMode != telego.ModeHTML {
		t.Fatalf("first fallback attempt = %#v, want the rich rendering", first)
	}
	if second.ChatID != gatewayTestChatID || second.Text != "<b>simpler</b>" || second.ParseMode != telego.ModeHTML {
		t.Fatalf("second fallback attempt = %#v, want the simpler rendering", second)
	}
}

func TestVerificationGatewaySkipsMissingMessagesAndDeletesExistingOnes(t *testing.T) {
	caller := &scriptedCaller{}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	ctx := context.Background()

	if err := gateway.Delete(ctx, gatewayTestChatID, 0); err != nil {
		t.Fatalf("delete absent message: %v", err)
	}
	if calls := caller.methodCalls("deleteMessage"); len(calls) != 0 {
		t.Fatalf("absent message caused %d Telegram deletes, want none", len(calls))
	}
	if err := gateway.Delete(ctx, gatewayTestChatID, 73); err != nil {
		t.Fatalf("delete existing message: %v", err)
	}
	calls := caller.methodCalls("deleteMessage")
	if len(calls) != 1 {
		t.Fatalf("existing message caused %d Telegram deletes, want one", len(calls))
	}
	params := decodeGatewayCall[telego.DeleteMessageParams](t, calls, 0)
	if params.ChatID.ID != gatewayTestChatID || params.MessageID != 73 {
		t.Fatalf("delete target = (%d, %d), want (%d, 73)", params.ChatID.ID, params.MessageID, gatewayTestChatID)
	}
}

func TestVerificationGatewayPreservesNotificationAndAuditDestinations(t *testing.T) {
	caller := &scriptedCaller{}
	connector := newTestClient(t, caller)
	gateway := NewVerificationGateway(connector)
	ctx := context.Background()

	gateway.Notify(ctx, gatewayTestChatID, "notice", 0)
	waitForMethodCalls(t, caller, "deleteMessage", 1)
	gateway.Alert(ctx, gatewayTestLogChat, "alert")
	gateway.AuditLog(ctx, gatewayTestAuditChat, "audit")
	gateway.FailAlert(ctx, 0, gatewayTestOtherChat, "failure")

	calls := caller.methodCalls("sendMessage")
	want := []struct {
		chatID int64
		text   string
	}{
		{gatewayTestChatID, "notice"},
		{gatewayTestLogChat, "alert"},
		{gatewayTestAuditChat, "audit"},
		{gatewayTestOtherChat, "failure"},
	}
	if len(calls) != len(want) {
		t.Fatalf("notification and audit sends = %d, want %d", len(calls), len(want))
	}
	for i, expected := range want {
		params := decodeGatewayCall[gatewaySendParams](t, calls, i)
		if params.ChatID != expected.chatID || params.Text != expected.text {
			t.Errorf("send %d = (%d, %q), want (%d, %q)", i, params.ChatID, params.Text, expected.chatID, expected.text)
		}
	}
}

func requireGatewaySuccess(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireGatewayValue[T any](t *testing.T, name string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
}

func TestVerificationGatewayPreservesEveryMembershipActionArgument(t *testing.T) {
	defaults := telego.ChatPermissions{CanSendMessages: telego.ToPtr(true), CanInviteUsers: telego.ToPtr(false)}
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat": {{value: &telego.ChatFullInfo{Permissions: &defaults}}},
	}}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	ctx := context.Background()

	requireGatewaySuccess(t, gateway.ApproveJoin(ctx, gatewayTestChatID, gatewayTestUserID))
	requireGatewaySuccess(t, gateway.DeclineJoin(ctx, gatewayTestChatID, gatewayTestUserID))
	requireGatewaySuccess(t, gateway.Ban(ctx, gatewayTestChatID, gatewayTestUserID, 0, true))
	requireGatewaySuccess(t, gateway.Unban(ctx, gatewayTestChatID, gatewayTestUserID, true))
	beforeMute := time.Now().Unix()
	requireGatewaySuccess(t, gateway.Mute(ctx, gatewayTestChatID, gatewayTestUserID, 3600))
	afterMute := time.Now().Unix()
	requireGatewaySuccess(t, gateway.Unmute(ctx, gatewayTestChatID, gatewayTestUserID))

	approve := decodeGatewayCall[telego.ApproveChatJoinRequestParams](t, caller.methodCalls("approveChatJoinRequest"), 0)
	requireGatewayValue(t, "approve parameters", approve, telego.ApproveChatJoinRequestParams{
		ChatID: telego.ChatID{ID: gatewayTestChatID}, UserID: gatewayTestUserID,
	})
	decline := decodeGatewayCall[telego.DeclineChatJoinRequestParams](t, caller.methodCalls("declineChatJoinRequest"), 0)
	requireGatewayValue(t, "decline parameters", decline, telego.DeclineChatJoinRequestParams{
		ChatID: telego.ChatID{ID: gatewayTestChatID}, UserID: gatewayTestUserID,
	})
	ban := decodeGatewayCall[telego.BanChatMemberParams](t, caller.methodCalls("banChatMember"), 0)
	requireGatewayValue(t, "ban parameters", ban, telego.BanChatMemberParams{
		ChatID: telego.ChatID{ID: gatewayTestChatID}, UserID: gatewayTestUserID, RevokeMessages: true,
	})
	unban := decodeGatewayCall[telego.UnbanChatMemberParams](t, caller.methodCalls("unbanChatMember"), 0)
	requireGatewayValue(t, "unban parameters", unban, telego.UnbanChatMemberParams{
		ChatID: telego.ChatID{ID: gatewayTestChatID}, UserID: gatewayTestUserID, OnlyIfBanned: true,
	})
	restrictions := caller.methodCalls("restrictChatMember")
	if len(restrictions) != 2 {
		t.Fatalf("restriction calls = %d, want mute and unmute", len(restrictions))
	}
	mute := decodeGatewayCall[telego.RestrictChatMemberParams](t, restrictions, 0)
	if mute.ChatID.ID != gatewayTestChatID || mute.UserID != gatewayTestUserID ||
		mute.UntilDate < beforeMute+3600 || mute.UntilDate > afterMute+3600 {
		t.Fatalf("mute parameters changed: %#v", mute)
	}
	unmute := decodeGatewayCall[telego.RestrictChatMemberParams](t, restrictions, 1)
	requireGatewayValue(t, "unmute parameters", unmute, telego.RestrictChatMemberParams{
		ChatID: telego.ChatID{ID: gatewayTestChatID}, UserID: gatewayTestUserID, Permissions: defaults,
	})
}

func TestVerificationGatewayKeepsCachedAndFreshAdminReadsDistinct(t *testing.T) {
	member := &telego.ChatMemberMember{Status: telego.MemberStatusMember, User: telego.User{ID: gatewayTestUserID}}
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChatMember": {{value: member}},
	}}
	connector := newTestClient(t, caller)
	connector.adminCache[adminKey{chatID: gatewayTestChatID, userID: gatewayTestUserID}] = time.Now().Add(time.Minute)
	gateway := NewVerificationGateway(connector)

	cached, err := gateway.CachedAdmin(context.Background(), gatewayTestChatID, gatewayTestUserID)
	if err != nil || !cached {
		t.Fatalf("cached admin read = (%v, %v), want cached true", cached, err)
	}
	if calls := caller.methodCalls("getChatMember"); len(calls) != 0 {
		t.Fatalf("cached admin read made %d Telegram calls, want none", len(calls))
	}
	fresh, err := gateway.FreshAdmin(context.Background(), gatewayTestChatID, gatewayTestUserID)
	if err != nil || fresh {
		t.Fatalf("fresh admin read = (%v, %v), want live false", fresh, err)
	}
	if calls := caller.methodCalls("getChatMember"); len(calls) != 1 {
		t.Fatalf("fresh admin read made %d Telegram calls, want one", len(calls))
	}
}

func TestVerificationGatewayAcknowledgesWithTheRequestedResult(t *testing.T) {
	caller := &scriptedCaller{}
	gateway := NewVerificationGateway(newTestClient(t, caller))
	ctx := context.Background()

	if err := gateway.AckFast(ctx, "fast-callback"); err != nil {
		t.Fatal(err)
	}
	if err := gateway.AckResult(ctx, "toast-callback", verification.AckResult{Text: "toast"}); err != nil {
		t.Fatal(err)
	}
	if err := gateway.AckResult(ctx, "alert-callback", verification.AckResult{Text: "alert", Alert: true}); err != nil {
		t.Fatal(err)
	}

	calls := caller.methodCalls("answerCallbackQuery")
	if len(calls) != 3 {
		t.Fatalf("callback acknowledgements = %d, want 3", len(calls))
	}
	fast := decodeGatewayCall[telego.AnswerCallbackQueryParams](t, calls, 0)
	toast := decodeGatewayCall[telego.AnswerCallbackQueryParams](t, calls, 1)
	alert := decodeGatewayCall[telego.AnswerCallbackQueryParams](t, calls, 2)
	if fast.CallbackQueryID != "fast-callback" || fast.Text != "" || fast.ShowAlert {
		t.Fatalf("fast acknowledgement changed: %#v", fast)
	}
	if toast.CallbackQueryID != "toast-callback" || toast.Text != "toast" || toast.ShowAlert {
		t.Fatalf("toast acknowledgement changed: %#v", toast)
	}
	if alert.CallbackQueryID != "alert-callback" || alert.Text != "alert" || !alert.ShowAlert {
		t.Fatalf("alert acknowledgement changed: %#v", alert)
	}
}

func TestVerificationErrorPreservesPlatformFailureMeaning(t *testing.T) {
	if err := verificationError(nil); err != nil {
		t.Fatalf("nil platform error became %v", err)
	}
	tests := []struct {
		name       string
		cause      error
		kind       verification.FailureKind
		code       int
		retryAfter time.Duration
	}{
		{name: "join request gone", cause: errors.New("HIDE_REQUESTER_MISSING"), kind: verification.FailureJoinRequestGone},
		{name: "applicant gone", cause: &ta.Error{ErrorCode: 403, Description: "user is deactivated"}, kind: verification.FailureApplicantGone, code: 403},
		{name: "conversation unavailable", cause: &ta.Error{ErrorCode: 403, Description: "bot can't initiate conversation with a user"}, kind: verification.FailureCannotInitiateConversation, code: 403},
		{name: "blocked by user", cause: &ta.Error{ErrorCode: 403, Description: "bot was blocked by the user"}, kind: verification.FailureBlockedByUser, code: 403},
		{name: "group unreachable", cause: errors.New("bot was kicked from the supergroup chat"), kind: verification.FailureGroupUnreachable},
		{name: "rate limited", cause: &ta.Error{ErrorCode: 429, Description: "Too Many Requests", Parameters: &ta.ResponseParameters{RetryAfter: 7}}, kind: verification.FailureRateLimited, code: 429, retryAfter: 7 * time.Second},
		{name: "message gone", cause: errors.New("message to delete not found"), kind: verification.FailureMessageGone},
		{name: "unclassified", cause: errors.New("temporary network failure")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verificationError(test.cause)
			var translated *verification.GatewayError
			if !errors.As(err, &translated) {
				t.Fatalf("translated error type = %T, want *verification.GatewayError", err)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("translated error lost platform cause %v", test.cause)
			}
			if translated.Kinds != test.kind || translated.Code != test.code || translated.RetryAfter != test.retryAfter {
				t.Fatalf("translated error = {kinds:%d code:%d retry:%s}, want {%d %d %s}",
					translated.Kinds, translated.Code, translated.RetryAfter, test.kind, test.code, test.retryAfter)
			}
		})
	}
}

func TestVerificationGatewayReturnsEveryCoreMembershipShape(t *testing.T) {
	user := telego.User{
		ID: gatewayTestOtherUser, IsBot: true, FirstName: "Member",
		Username: "member_name", LanguageCode: "en",
	}
	tests := []struct {
		name           string
		platform       telego.ChatMember
		coreType       any
		status         string
		isMember       bool
		restriction    int64
		hasRestriction bool
	}{
		{name: "owner", platform: &telego.ChatMemberOwner{Status: telego.MemberStatusCreator, User: user}, coreType: &verification.ChatMemberOwner{}, status: verification.MemberStatusCreator, isMember: true},
		{name: "administrator", platform: &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator, User: user}, coreType: &verification.ChatMemberAdministrator{}, status: verification.MemberStatusAdministrator, isMember: true},
		{name: "member", platform: &telego.ChatMemberMember{Status: telego.MemberStatusMember, User: user}, coreType: &verification.ChatMemberMember{}, status: verification.MemberStatusMember, isMember: true},
		{name: "restricted member", platform: &telego.ChatMemberRestricted{Status: telego.MemberStatusRestricted, User: user, IsMember: true, UntilDate: 9001}, coreType: &verification.ChatMemberRestricted{}, status: verification.MemberStatusRestricted, isMember: true, restriction: 9001, hasRestriction: true},
		{name: "restricted nonmember", platform: &telego.ChatMemberRestricted{Status: telego.MemberStatusRestricted, User: user, IsMember: false, UntilDate: 9002}, coreType: &verification.ChatMemberRestricted{}, status: verification.MemberStatusRestricted, restriction: 9002, hasRestriction: true},
		{name: "left", platform: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: user}, coreType: &verification.ChatMemberLeft{}, status: verification.MemberStatusLeft},
		{name: "banned", platform: &telego.ChatMemberBanned{Status: telego.MemberStatusBanned, User: user}, coreType: &verification.ChatMemberBanned{}, status: verification.MemberStatusBanned},
	}
	wantUser := verification.User{
		ID: user.ID, IsBot: user.IsBot, FirstName: user.FirstName,
		Username: user.Username, LanguageCode: user.LanguageCode,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedCaller{responses: map[string][]scriptedResult{
				"getChatMember": {{value: test.platform}},
			}}
			gateway := NewVerificationGateway(newTestClient(t, caller))
			member, err := gateway.Member(context.Background(), gatewayTestChatID, gatewayTestOtherUser)
			if err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(member) != reflect.TypeOf(test.coreType) {
				t.Fatalf("core membership type = %T, want %T", member, test.coreType)
			}
			until, comparable := member.RestrictionUntil()
			if member.MemberStatus() != test.status || member.MemberIsMember() != test.isMember ||
				member.MemberUser() != wantUser || until != test.restriction || comparable != test.hasRestriction {
				t.Fatalf("core membership = {%q member:%v user:%#v restriction:(%d,%v)}, want {%q %v %#v (%d,%v)}",
					member.MemberStatus(), member.MemberIsMember(), member.MemberUser(), until, comparable,
					test.status, test.isMember, wantUser, test.restriction, test.hasRestriction)
			}
			request := decodeGatewayCall[telego.GetChatMemberParams](t, caller.methodCalls("getChatMember"), 0)
			if request.ChatID.ID != gatewayTestChatID || request.UserID != gatewayTestOtherUser {
				t.Fatalf("member lookup target = (%d, %d), want (%d, %d)",
					request.ChatID.ID, request.UserID, gatewayTestChatID, gatewayTestOtherUser)
			}
		})
	}
	if member := verificationMember(nil); member != nil {
		t.Fatalf("nil Telegram membership became %T", member)
	}
}

func TestVerificationUpdatePreservesEveryCoreEvent(t *testing.T) {
	requestUser := telego.User{ID: 4201, IsBot: false, FirstName: "Request", Username: "requester", LanguageCode: "en"}
	actor := telego.User{ID: 4202, IsBot: true, FirstName: "Actor", Username: "actor", LanguageCode: "zh-hans"}
	memberUser := telego.User{ID: 4203, FirstName: "Member", Username: "member", LanguageCode: "zh-hant"}
	callbackUser := telego.User{ID: 4204, FirstName: "Callback", Username: "callback", LanguageCode: "en"}
	messageUser := telego.User{ID: 4205, FirstName: "Message", Username: "message", LanguageCode: "en"}
	update := telego.Update{
		ChatJoinRequest: &telego.ChatJoinRequest{
			Chat: telego.Chat{ID: gatewayTestChatID, Type: telego.ChatTypeSupergroup}, From: requestUser,
		},
		ChatMember: &telego.ChatMemberUpdated{
			Chat:          telego.Chat{ID: gatewayTestOtherChat, Type: telego.ChatTypeSupergroup},
			From:          actor,
			OldChatMember: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: memberUser},
			NewChatMember: &telego.ChatMemberRestricted{
				Status: telego.MemberStatusRestricted, User: memberUser, IsMember: true, UntilDate: 777,
			},
		},
		CallbackQuery: &telego.CallbackQuery{ID: "callback-4204", From: callbackUser, Data: "verify:data"},
		Message: &telego.Message{
			MessageID: 88,
			Chat:      telego.Chat{ID: gatewayTestAuditChat, Type: telego.ChatTypePrivate},
			From:      &messageUser,
			Text:      "kernel answer",
		},
	}
	got := verificationUpdate(update)
	want := verification.Update{
		ChatJoinRequest: &verification.ChatJoinRequest{
			Chat: verification.Chat{ID: gatewayTestChatID, Type: verification.ChatTypeSupergroup},
			From: verification.User{ID: 4201, FirstName: "Request", Username: "requester", LanguageCode: "en"},
		},
		ChatMember: &verification.ChatMemberUpdated{
			Chat: verification.Chat{ID: gatewayTestOtherChat, Type: verification.ChatTypeSupergroup},
			From: verification.User{ID: 4202, IsBot: true, FirstName: "Actor", Username: "actor", LanguageCode: "zh-hans"},
			OldChatMember: &verification.ChatMemberLeft{
				Status: verification.MemberStatusLeft,
				User:   verification.User{ID: 4203, FirstName: "Member", Username: "member", LanguageCode: "zh-hant"},
			},
			NewChatMember: &verification.ChatMemberRestricted{
				Status:   verification.MemberStatusRestricted,
				User:     verification.User{ID: 4203, FirstName: "Member", Username: "member", LanguageCode: "zh-hant"},
				IsMember: true, UntilDate: 777,
			},
		},
		CallbackQuery: &verification.CallbackQuery{
			ID: "callback-4204", From: verification.User{ID: 4204, FirstName: "Callback", Username: "callback", LanguageCode: "en"},
			Data: "verify:data",
		},
		Message: &verification.Message{
			MessageID: 88,
			Chat:      verification.Chat{ID: gatewayTestAuditChat, Type: verification.ChatTypePrivate},
			From:      &verification.User{ID: 4205, FirstName: "Message", Username: "message", LanguageCode: "en"},
			Text:      "kernel answer",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("converted update = %#v, want %#v", got, want)
	}

	withoutSender := verificationUpdate(telego.Update{Message: &telego.Message{
		MessageID: 89, Chat: telego.Chat{ID: gatewayTestAuditChat, Type: telego.ChatTypePrivate}, Text: "anonymous",
	}})
	if withoutSender.Message == nil || withoutSender.Message.From != nil {
		t.Fatalf("message without sender converted as %#v", withoutSender.Message)
	}
}
