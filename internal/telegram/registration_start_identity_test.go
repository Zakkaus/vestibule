package telegram

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mymmrac/telego"
)

func TestRegistrationStartIdentityRepliesBeforeInstanceClaim(t *testing.T) {
	const (
		chatID = int64(-1009000000903)
		fromID = int64(1703)
	)

	tests := []struct {
		name     string
		text     string
		language string
		marker   string
	}{
		{name: "english with trailing whitespace", text: "/start \t\n", language: "en", marker: "deployer"},
		{name: "simplified Chinese with bot mention", text: "/start@VERIFY_TEST_BOT  ", language: "zh-CN", marker: "部署者"},
		{name: "traditional Chinese with bot mention", text: "/start@verify_test_bot\n", language: "zh-Hant", marker: "部署者"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, store := registrationFixture(t)
			if ownerID := store.Registrations().OwnerID; ownerID != 0 {
				t.Fatalf("fixture owner ID = %d, want unclaimed instance", ownerID)
			}
			caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
			service := newRegistrationService(
				t.Context(), newRegistrationBot(t, caller), store, cfg,
				"verify_test_bot", testBotID, nil, nil, nil,
			)
			runRegistrationUpdate(t, service.bot, service, telego.Update{Message: &telego.Message{
				Chat: telego.Chat{ID: chatID, Type: telego.ChatTypePrivate},
				From: &telego.User{ID: fromID, LanguageCode: test.language},
				Text: test.text,
			}})

			messages := caller.messagesTo(chatID)
			if len(messages) != 1 {
				t.Fatalf("replies to chat %d = %d, want one identity reply; all sends = %v", chatID, len(messages), messages)
			}
			if got := messages[0].ChatID.ID; got != chatID {
				t.Fatalf("identity reply destination = %d, want message.Chat.ID %d", got, chatID)
			}
			if !strings.Contains(messages[0].Text, strconv.FormatInt(fromID, 10)) {
				t.Errorf("identity reply %q does not contain sender From.ID %d", messages[0].Text, fromID)
			}
			if !strings.Contains(strings.ToLower(messages[0].Text), strings.ToLower(test.marker)) {
				t.Errorf("identity reply %q does not explain that the instance has no %s", messages[0].Text, test.marker)
			}
		})
	}
}

func TestRegistrationStartIdentityDoesNotInterceptOtherStarts(t *testing.T) {
	const (
		chatID = int64(-1009000000904)
		fromID = int64(1704)
	)

	tests := []struct {
		name  string
		text  string
		chat  string
		from  *telego.User
		claim bool
	}{
		{name: "already claimed", text: "/start", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}, claim: true},
		{name: "group chat", text: "/start", chat: telego.ChatTypeSupergroup, from: &telego.User{ID: fromID}},
		{name: "missing sender", text: "/start", chat: telego.ChatTypePrivate},
		{name: "unknown payload", text: "/start unknown", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}},
		{name: "multiple payloads", text: "/start one two", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}},
		{name: "different bot mention", text: "/start@another_bot", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}},
		{name: "command offset is not zero", text: "prefix /start", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}},
		{name: "command name has suffix", text: "/startx", chat: telego.ChatTypePrivate, from: &telego.User{ID: fromID}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, store := registrationFixture(t)
			caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
			service := newRegistrationService(
				t.Context(), newRegistrationBot(t, caller), store, cfg,
				"verify_test_bot", testBotID, nil, nil, nil,
			)
			if test.claim {
				bindTestOwner(t, store, service.now())
			}
			runRegistrationUpdate(t, service.bot, service, telego.Update{Message: &telego.Message{
				Chat: telego.Chat{ID: chatID, Type: test.chat},
				From: test.from,
				Text: test.text,
			}})

			if messages := caller.sendAttemptsTo(chatID); len(messages) != 0 {
				t.Fatalf("unexpected sends for %s: %v", test.name, messages)
			}
		})
	}
}
