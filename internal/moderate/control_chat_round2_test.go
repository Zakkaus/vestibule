package moderate

import (
	"context"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func TestControlChatRound2TopicRootsAreNotReplyTargets(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		topic, creation, root, local bool
	}{
		{"creation service reply", true, true, false, false},
		{"thread root reply", true, false, true, false},
		{"ordinary topic reply", true, false, false, true},
		{"thread root without topic flag", false, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, telegram := remoteFixture(t)
			service.cfg.Groups = append(service.cfg.Groups, settings.GroupConfig{ID: remoteControlID})
			service.settings = testSettings(t, service.cfg)
			message := moderationCommand(remoteControlID, "/ban 555")
			message.IsTopicMessage = tc.topic
			message.MessageThreadID = 64
			message.ReplyToMessage.MessageID = 70
			if tc.root {
				message.ReplyToMessage.MessageID = 64
			}
			if tc.creation {
				message.ReplyToMessage.ForumTopicCreated = &telego.ForumTopicCreated{Name: "Topic"}
			}
			runFakeHandler(t, newAPITestBot(t, telegram), service.OnBan, telego.Update{Message: message})
			wantGroup, wantUser, wantDeletes := remoteGroupID, int64(555), 0
			if tc.local {
				wantGroup, wantUser, wantDeletes = remoteControlID, message.ReplyToMessage.From.ID, 2
			}
			if len(telegram.actions) != 1 || telegram.actions[0].group != wantGroup || telegram.actions[0].user != wantUser || telegram.deletes != wantDeletes {
				t.Fatalf("actions=%+v deletes=%d, want group=%d user=%d deletes=%d", telegram.actions, telegram.deletes, wantGroup, wantUser, wantDeletes)
			}
		})
	}
}

type restrictOnlyTelegram struct{ *remoteTelegram }

func (b restrictOnlyTelegram) FreshRights(ctx context.Context, chatID, userID int64) (verification.GroupRights, error) {
	rights, err := b.remoteTelegram.FreshRights(ctx, chatID, userID)
	rights.CanDeleteMessages = false
	return rights, err
}

func TestControlChatRound2RemoteDoesNotRequireDeleteRights(t *testing.T) {
	for _, command := range []string{"/ban", "/mute"} {
		for _, remote := range []bool{true, false} {
			t.Run(command+"/"+map[bool]string{true: "remote", false: "local"}[remote], func(t *testing.T) {
				service, telegram := remoteFixture(t)
				service.telegram = restrictOnlyTelegram{telegram}
				if remote {
					runRemote(t, service, telegram, command+" 8", remoteControlID)
					if len(telegram.actions) != 1 || telegram.actions[0].group != remoteGroupID || telegram.actions[0].user != 8 || telegram.deletes != 0 {
						t.Fatalf("remote restriction without deletion rights: actions=%+v deletes=%d", telegram.actions, telegram.deletes)
					}
				} else {
					message := moderationCommand(remoteGroupID, command)
					handler := service.OnBan
					if command == "/mute" {
						handler = service.OnMute
					}
					runFakeHandler(t, newAPITestBot(t, telegram), handler, telego.Update{Message: message})
					if len(telegram.actions) != 0 || len(telegram.notifications) != 1 || telegram.notifications[0].text != i18n.Messages.Moderate.Common.CommandAdminOnly.Render(i18n.LangEN, command) {
						t.Fatal("local command lost its existing delete-right requirement")
					}
				}
			})
		}
	}
}

func TestControlChatRound2DurationGuidanceOnlyForMute(t *testing.T) {
	for _, language := range []string{"en", "zh", "zh-Hant", "ja", "ru"} {
		for _, command := range []string{"/ban", "/mute", "/unmute", "/warn"} {
			t.Run(language+command, func(t *testing.T) {
				service, telegram := remoteFixture(t)
				service.cfg.Lang = language
				service.settings = testSettings(t, service.cfg)
				runRemote(t, service, telegram, command, remoteControlID)
				if len(telegram.replies) != 1 {
					t.Fatal("usage reply missing")
				}
				text := telegram.replies[0].Text
				if !strings.Contains(text, command+" <user_id>") || strings.Contains(text, "s/m/h/d") != (command == "/mute") || strings.Contains(text, "[duration]") != (command == "/mute") {
					t.Fatalf("wrong command guidance: %q", text)
				}
			})
		}
	}
}
