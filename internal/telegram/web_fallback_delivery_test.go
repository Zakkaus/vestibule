package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
	ta "github.com/mymmrac/telego/telegoapi"
)

func TestWebFallbackUsesPrivateRouteAndDeliveryRecovery(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		resend bool
	}{
		{"temporary chat", nil, false},
		{"after private start", nil, true},
		{"rejected", &ta.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"}, false},
		{"uncertain", errors.New("connection reset by peer"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedCaller{}
			service, store, gateway := webGatewayFixture(t, caller, func(group *settings.GroupConfig) {
				group.VerifyMode = settings.ModeCaptcha
				group.Questions = []settings.Question{{Q: "Which shape has three sides?", Options: []string{"triangle", "square"}, Answer: 0}}
			})
			ctx := context.Background()
			update := verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: gatewayTestChatID}, From: verification.User{ID: gatewayTestUserID, FirstName: "Applicant"}, UserChatID: gatewayTestOtherUser, Date: time.Now().Unix()}}
			if err := service.OnJoinRequest(verification.NewHandlerContext(ctx, gateway), update); err != nil {
				t.Fatal(err)
			}
			if test.resend {
				service.SendDMChallenge(ctx, gatewayTestUserID, "en", gatewayTestChatID)
			}
			before := len(caller.methodCalls("sendMessage"))
			records, err := store.LoadPending("")
			if err != nil || len(records) != 1 {
				t.Fatalf("pending before fallback: %v/%v", records, err)
			}
			id := verification.ChallengeID(records[0].Ref())
			if changed, err := service.FallbackWeb(ctx, id); err != nil || !changed {
				t.Fatalf("fallback: %v/%v", changed, err)
			}
			caller.responses["sendMessage"] = []scriptedResult{{err: test.err}, {}}
			service.DeliverWebFallback(ctx, id)
			calls := caller.methodCalls("sendMessage")
			question := decodeGatewayCall[gatewaySendParams](t, calls, before)
			wantChat := gatewayTestOtherUser
			if test.resend {
				wantChat = gatewayTestUserID
			}
			if question.ChatID != wantChat || !strings.Contains(question.Text, "Which shape has three sides?") {
				t.Fatalf("replacement did not use the web link's route: %+v", question)
			}
			records, err = store.LoadPending("")
			if err != nil || len(records) != 1 || records[0].Mode != settings.ModeQuiz || records[0].FallbackPending != (test.err != nil) {
				t.Fatalf("fallback delivery state: %v/%v", records, err)
			}
			assertWebFallbackRecovery(t, service, caller, before, test.name, test.err, records[0])
		})
	}
}

func assertWebFallbackRecovery(t *testing.T, service *verification.Service, caller *scriptedCaller, before int, name string, sendErr error, record verification.PendingRecord) {
	t.Helper()
	calls := caller.methodCalls("sendMessage")
	if sendErr == nil {
		if record.Deadline < time.Now().Add(3*time.Minute).Unix() {
			t.Fatal("confirmed question delivery did not start a full answer window")
		}
	} else if name == "rejected" {
		recovery := decodeGatewayCall[gatewaySendParams](t, calls, before+1)
		if recovery.ChatID != gatewayTestChatID || !strings.Contains(recovery.ReplyMarkup.InlineKeyboard[0][0].URL, "?start=") {
			t.Fatal("rejected C5 delivery did not publish the group /start recovery link")
		}
		service.SendDMChallenge(context.Background(), gatewayTestUserID, "en", gatewayTestChatID)
		last := caller.methodCalls("sendMessage")
		recovered := decodeGatewayCall[gatewaySendParams](t, last, len(last)-1)
		if recovered.ChatID != gatewayTestUserID || !strings.Contains(recovered.Text, "Which shape has three sides?") {
			t.Fatal("/start failed to deliver the committed replacement question")
		}
	} else if len(calls) != before+1 {
		t.Fatal("uncertain delivery was treated as a definite rejection")
	}
}

func TestWebFallbackChannelInvitationUsesWebDestination(t *testing.T) {
	caller := &scriptedCaller{}
	service, store, gateway := webGatewayFixture(t, caller, func(group *settings.GroupConfig) {
		group.VerifyMode = settings.ModeCaptcha
		group.Questions = []settings.Question{{Q: "Which shape has three sides?", Options: []string{"triangle", "square"}, Answer: 0}}
		channelID := gatewayTestOtherChat
		group.RequiredChannelID = &channelID
		group.ChannelDisplay = "Private channel"
		group.ChannelInviteURL = "https://t.me/+test_required_channel"
	})
	ctx := context.Background()
	update := verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: gatewayTestChatID}, From: verification.User{ID: gatewayTestUserID}, UserChatID: gatewayTestOtherUser, Date: time.Now().Unix()}}
	if err := service.OnJoinRequest(verification.NewHandlerContext(ctx, gateway), update); err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadPending("")
	if err != nil || len(records) != 1 {
		t.Fatalf("pending: %v/%v", records, err)
	}
	id := verification.ChallengeID(records[0].Ref())
	if changed, err := service.FallbackWeb(ctx, id); err != nil || !changed {
		t.Fatalf("fallback: %v/%v", changed, err)
	}
	caller.responses["getChatMember"] = []scriptedResult{{value: map[string]any{"status": "left", "user": map[string]any{"id": gatewayTestUserID, "first_name": "Applicant", "is_bot": false}}}}
	service.DeliverWebFallback(ctx, id)
	message := decodeGatewayCall[gatewaySendParams](t, caller.methodCalls("sendMessage"), 1)
	if message.ChatID != gatewayTestOtherUser || !strings.Contains(message.Text, "https://t.me/+test_required_channel") {
		t.Fatalf("C5 channel invitation did not reach the web link's private route: %+v", message)
	}
	member := scriptedResult{value: map[string]any{"status": "member", "user": map[string]any{"id": gatewayTestUserID, "first_name": "Applicant", "is_bot": false}}}
	caller.responses["getChatMember"] = []scriptedResult{member, member}
	callback := verification.Update{CallbackQuery: &verification.CallbackQuery{ID: "channel-recheck", From: verification.User{ID: gatewayTestUserID}, Data: fmt.Sprintf("%s%d:%d", verification.ChannelRecheckCallbackPrefix, gatewayTestChatID, gatewayTestUserID)}}
	if err := service.OnChannelRecheck(verification.NewHandlerContext(ctx, gateway), callback); err != nil {
		t.Fatal(err)
	}
	question := decodeGatewayCall[gatewaySendParams](t, caller.methodCalls("sendMessage"), 2)
	if question.ChatID != gatewayTestOtherUser || !strings.Contains(question.Text, "Which shape has three sides?") {
		t.Fatalf("channel continuation did not deliver the C5 question through the original private route: %+v", question)
	}
}
