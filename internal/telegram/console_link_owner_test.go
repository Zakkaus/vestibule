package telegram

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

type recordingIssuer struct {
	owner int64
	asked []int64
}

func (r *recordingIssuer) IssueOperatorLink(telegramID int64) (string, time.Time, error) {
	r.asked = append(r.asked, telegramID)
	if telegramID != r.owner {
		return "", time.Time{}, auth.ErrOperatorNotAllowed
	}
	return "issued-token", time.Now().Add(time.Minute), nil
}

// The console link is the whole of an operator session: whoever receives one controls the
// instance. Its only gate is the issuer's owner check -- the handler has none of its own, and
// answers a refusal with silence. Nothing tested that a stranger gets that silence.
func TestConsoleLinkGoesOnlyToTheOwner(t *testing.T) {
	const owner, stranger = int64(7), int64(8)
	for _, tc := range []struct {
		name      string
		from      int64
		chatType  string
		wantSends int
	}{
		{"the owner in a private chat", owner, telego.ChatTypePrivate, 1},
		{"a stranger in a private chat", stranger, telego.ChatTypePrivate, 0},
		{"the owner asking in a group", owner, telego.ChatTypeSupergroup, 0},
		{"a stranger asking in a group", stranger, telego.ChatTypeSupergroup, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedCaller{responses: map[string][]scriptedResult{}}
			bot, err := telego.NewBot("1:"+strings.Repeat("a", 35),
				telego.WithAPICaller(caller), telego.WithDiscardLogger())
			if err != nil {
				t.Fatal(err)
			}
			issuer := &recordingIssuer{owner: owner}
			handler, err := NewConsoleLinkHandler(bot, issuer, "https://console.example.test")
			if err != nil {
				t.Fatal(err)
			}
			update := telego.Update{Message: &telego.Message{
				From: &telego.User{ID: tc.from},
				Chat: telego.Chat{ID: tc.from, Type: tc.chatType},
				Text: "/console",
			}}
			if err := handler(&th.Context{}, update); err != nil {
				t.Fatalf("handler: %v", err)
			}

			sends := caller.methodCalls("sendMessage")
			if len(sends) != tc.wantSends {
				t.Fatalf("sendMessage calls = %d, want %d; a link reaching anyone but the "+
					"owner hands them the instance", len(sends), tc.wantSends)
			}
			for _, call := range sends {
				if !strings.Contains(string(call.body), "issued-token") {
					t.Errorf("the message sent carries no token: %s", call.body)
				}
			}
		})
	}
}

// The issuer refuses when it has no way to tell who the owner is, rather than treating an
// absent answer as a yes.
func TestOperatorLinkIssuerFailsClosed(t *testing.T) {
	manager, err := auth.New(auth.Config{BotToken: "123:token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.IssueOperatorLink(7); !errors.Is(err, auth.ErrOperatorNotAllowed) {
		t.Fatalf("with no owner predicate configured, IssueOperatorLink = %v, want %v",
			err, auth.ErrOperatorNotAllowed)
	}
}
