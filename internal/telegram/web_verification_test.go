package telegram

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

func webGatewayFixture(t *testing.T, caller *scriptedCaller, configure ...func(*settings.GroupConfig)) (*verification.Service, *database.VerificationStore, *VerificationGateway) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := database.NewVerificationStore(db)
	cfg := &settings.Config{GroupIDs: []int64{gatewayTestChatID}, Groups: []settings.GroupConfig{{ID: gatewayTestChatID, VerifyMode: settings.ModePoW, DeliveryMode: settings.DeliveryDM}}}
	for _, apply := range configure {
		apply(&cfg.Groups[0])
	}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetWebCapabilities(settings.WebCapabilities{ConsoleURL: "https://console.example", TurnstileAvailable: true})
	gateway := NewVerificationGateway(newTestClient(t, caller))
	service, err := verification.New(runtime, gateway, store, cfg, &i18n.Messages, nil, verification.Identity{Username: "test_verification_bot"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	gateway.SetWebVerification(service, "https://console.example/")
	return service, store, gateway
}

func TestWebJoinUsesTemporaryPrivateChatAndRetainsDeliveryFacts(t *testing.T) {
	caller := &scriptedCaller{}
	service, store, gateway := webGatewayFixture(t, caller)
	requestDate := time.Now().Unix()
	update := verificationUpdate(telego.Update{ChatJoinRequest: &telego.ChatJoinRequest{Chat: telego.Chat{ID: gatewayTestChatID}, From: telego.User{ID: gatewayTestUserID, FirstName: "Applicant", LanguageCode: "ja"}, UserChatID: gatewayTestOtherUser, Date: requestDate}})
	if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), update); err != nil {
		t.Fatal(err)
	}
	calls := caller.methodCalls("sendMessage")
	message := decodeGatewayCall[gatewaySendParams](t, calls, 0)
	if message.ChatID != gatewayTestOtherUser || message.LinkPreviewOptions == nil || !message.LinkPreviewOptions.IsDisabled {
		t.Fatalf("temporary private delivery or preview suppression failed: %+v", message)
	}
	link := message.ReplyMarkup.InlineKeyboard[0][0].URL
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimPrefix(parsed.Path, "/verify/")
	record, _, valid, err := service.ResolveWebToken(context.Background(), raw)
	if err != nil || !valid || record.Mode != settings.ModePoW || record.UserChatID != gatewayTestOtherUser || record.RequestDate != requestDate || record.Lang != "ja" {
		t.Fatalf("delivery facts were lost: valid=%v record=%+v err=%v", valid, record, err)
	}
	persisted, err := store.LoadPending("")
	if err != nil || len(persisted) != 1 || persisted[0].UserChatID != gatewayTestOtherUser {
		t.Fatal("delivery metadata did not survive SQLite")
	}
}

func TestWebRejectedDMKeepsModeAndStartPrivatelyRotatesToken(t *testing.T) {
	rejection := &ta.Error{ErrorCode: 403, Description: "Forbidden: bot was blocked by the user"}
	caller := &scriptedCaller{responses: map[string][]scriptedResult{"sendMessage": {{err: rejection}, {}}}}
	service, store, gateway := webGatewayFixture(t, caller)
	update := verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: gatewayTestChatID}, From: verification.User{ID: gatewayTestUserID, FirstName: "Applicant"}, UserChatID: gatewayTestOtherUser, Date: time.Now().Unix()}}
	if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), update); err != nil {
		t.Fatal(err)
	}
	calls := caller.methodCalls("sendMessage")
	failed := decodeGatewayCall[gatewaySendParams](t, calls, 0)
	initial := strings.TrimPrefix(strings.TrimPrefix(failed.ReplyMarkup.InlineKeyboard[0][0].URL, "https://console.example"), "/verify/")
	recovery := decodeGatewayCall[gatewaySendParams](t, calls, 1)
	if recovery.ChatID != gatewayTestChatID || strings.Contains(recovery.Text, "/verify/") || !strings.Contains(recovery.ReplyMarkup.InlineKeyboard[0][0].URL, "?start=") {
		t.Fatal("failed DM exposed a web bearer to the group instead of a start link")
	}
	records, err := store.LoadPending("")
	if err != nil || len(records) != 1 || records[0].Mode != settings.ModePoW {
		t.Fatal("DM rejection changed the verification method")
	}
	service.SendDMChallenge(context.Background(), gatewayTestUserID, "zh-CN", gatewayTestChatID)
	calls = caller.methodCalls("sendMessage")
	delivered := decodeGatewayCall[gatewaySendParams](t, calls, len(calls)-1)
	replacement := strings.TrimPrefix(delivered.ReplyMarkup.InlineKeyboard[0][0].URL, "https://console.example/verify/")
	if delivered.ChatID != gatewayTestUserID || replacement == initial {
		t.Fatal("start did not privately resend with explicit rotation")
	}
	if _, _, valid, err := service.ResolveWebToken(context.Background(), initial); err != nil || valid {
		t.Fatalf("old bearer survived resend: %v/%v", valid, err)
	}
	if _, _, valid, err := service.ResolveWebToken(context.Background(), replacement); err != nil || !valid {
		t.Fatalf("replacement bearer is unusable: %v/%v", valid, err)
	}
}

func TestWebDeliveryIncludesRequiredPrivateChannel(t *testing.T) {
	for _, mode := range []string{settings.ModePoW, settings.ModeCaptcha} {
		t.Run(mode, func(t *testing.T) {
			caller := &scriptedCaller{}
			service, _, gateway := webGatewayFixture(t, caller, func(group *settings.GroupConfig) {
				group.VerifyMode = mode
				channelID := gatewayTestOtherChat
				group.RequiredChannelID = &channelID
				group.ChannelDisplay = "Private channel"
				group.ChannelInviteURL = "https://t.me/+test_required_channel"
			})
			update := verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: gatewayTestChatID}, From: verification.User{ID: gatewayTestUserID}, UserChatID: gatewayTestOtherUser, Date: time.Now().Unix()}}
			if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), update); err != nil {
				t.Fatal(err)
			}
			message := decodeGatewayCall[gatewaySendParams](t, caller.methodCalls("sendMessage"), 0)
			if !strings.Contains(message.Text, "Private channel") || !strings.Contains(message.Text, "https://t.me/+test_required_channel") {
				t.Fatalf("web DM omitted the private-channel invitation: %+v", message)
			}
		})
	}
}

func TestWebDeliveryURLClearsQueryAndFragment(t *testing.T) {
	for _, base := range []string{"https://console.example/#console", "https://console.example/?view=console", "https://console.example/base/?view=console#console"} {
		t.Run(base, func(t *testing.T) {
			caller := &scriptedCaller{}
			service, _, gateway := webGatewayFixture(t, caller)
			gateway.SetWebVerification(service, base)
			update := verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: gatewayTestChatID}, From: verification.User{ID: gatewayTestUserID}}}
			if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), update); err != nil {
				t.Fatal(err)
			}
			message := decodeGatewayCall[gatewaySendParams](t, caller.methodCalls("sendMessage"), 0)
			link, err := url.Parse(message.ReplyMarkup.InlineKeyboard[0][0].URL)
			if err != nil {
				t.Fatal(err)
			}
			raw := link.Path[strings.LastIndex(link.Path, "/")+1:]
			if link.RawQuery != "" || link.Fragment != "" || !strings.HasSuffix(link.Path, "/verify/"+raw) {
				t.Fatalf("verification route absorbed by query/fragment: %s", link)
			}
			if _, _, valid, err := service.ResolveWebToken(context.Background(), raw); err != nil || !valid {
				t.Fatalf("URL does not address the issued token: %s, %v", link, err)
			}
		})
	}
}
