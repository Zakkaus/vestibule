package moderate

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

const remoteGroupID int64 = -1009000000621
const remoteControlID int64 = -1009000000622
const remoteAuditID int64 = -1009000000623

type remoteAction struct {
	command     string
	group, user int64
	seconds     int
}

type remoteTelegram struct {
	*authorizationTrackingTelegram
	replies []telego.SendMessageParams
	actions []remoteAction
}

func (b *remoteTelegram) Call(_ context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	if !strings.HasSuffix(url, "/sendMessage") {
		return nil, errors.New("unexpected Telegram method")
	}
	var params telego.SendMessageParams
	if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
		return nil, err
	}
	b.replies = append(b.replies, params)
	return &ta.Response{Ok: true, Result: json.RawMessage(`{"message_id":99}`)}, nil
}

func (b *remoteTelegram) Ban(ctx context.Context, group, user int64, seconds int, revoke bool) error {
	b.actions = append(b.actions, remoteAction{"/ban", group, user, seconds})
	return b.fakeModBot.Ban(ctx, group, user, seconds, revoke)
}

func (b *remoteTelegram) Mute(ctx context.Context, group, user int64, seconds int) error {
	b.actions = append(b.actions, remoteAction{"/mute", group, user, seconds})
	return b.fakeModBot.Mute(ctx, group, user, seconds)
}

func (b *remoteTelegram) Unmute(ctx context.Context, group, user int64) error {
	b.actions = append(b.actions, remoteAction{"/unmute", group, user, 0})
	return b.fakeModBot.Unmute(ctx, group, user)
}

func remoteFixture(t *testing.T) (*Service, *remoteTelegram) {
	t.Helper()
	cfg := &settings.Config{Groups: []settings.GroupConfig{{ID: remoteGroupID, ControlChatID: new(remoteControlID), AdminLogChatID: new(remoteAuditID)}},
		Lang: "en", BanSeconds: 3600, MuteSeconds: 600, WarnLimit: 2, NotifyTTLSeconds: -1}
	telegram := &remoteTelegram{authorizationTrackingTelegram: &authorizationTrackingTelegram{
		fakeModBot: newFakeMod(), admins: map[int64]bool{7: true},
	}}
	service, err := New(testSettings(t, cfg), telegram, cfg, newWarningJSONStore(""))
	if err != nil {
		t.Fatal(err)
	}
	return service, telegram
}

func runRemote(t *testing.T, service *Service, telegram *remoteTelegram, text string, chatID int64) {
	t.Helper()
	message := moderationCommand(chatID, text)
	message.ReplyToMessage = nil
	command := strings.Fields(text)[0]
	command = strings.Split(command, "@")[0]
	handlers := map[string]th.Handler{"/ban": service.OnBan, "/mute": service.OnMute, "/unmute": service.OnUnmute, "/warn": service.OnWarn, "/sb": service.OnPurge}
	runFakeHandler(t, newAPITestBot(t, telegram), handlers[command], telego.Update{Message: message})
}

func assertRemoteReply(t *testing.T, telegram *remoteTelegram, text string) {
	t.Helper()
	if len(telegram.replies) != 1 {
		t.Fatalf("replies = %#v, want one", telegram.replies)
	}
	reply := telegram.replies[0]
	if reply.ChatID.ID != remoteControlID || reply.ReplyParameters == nil || reply.ReplyParameters.MessageID != 11 || reply.Text != text {
		t.Fatalf("reply = %#v, want control-chat reply to command with %q", reply, text)
	}
	if telegram.deletes != 0 {
		t.Fatalf("deleted %d messages in control chat", telegram.deletes)
	}
}

func TestControlModerationAuthorizationAndTargetProtection(t *testing.T) {
	for _, command := range []string{"/ban", "/mute", "/unmute", "/warn"} {
		for _, tc := range []struct {
			name           string
			caller, target bool
			lookupErr      error
		}{
			{name: "non-admin in protected group", target: false},
			{name: "administrator target", caller: true, target: true},
			{name: "live lookup unavailable", caller: true, lookupErr: errors.New("unavailable")},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				service, telegram := remoteFixture(t)
				telegram.admins = map[int64]bool{7: tc.caller, 8: tc.target}
				telegram.freshErr = tc.lookupErr
				runRemote(t, service, telegram, command+" 8", remoteControlID)
				allowed := tc.caller && tc.lookupErr == nil && command == "/unmute"
				if allowed {
					if telegram.unmutes != 1 || len(telegram.freshChecks) != 0 {
						t.Fatal("unmute lost in-group parity")
					}
				} else if len(telegram.actions) != 0 || service.warnings.counters[warningKey{groupID: remoteGroupID, userID: 8}] != 0 {
					t.Fatal("refused action changed the protected group")
				}
				assertProtectedAuthorization(t, telegram)
				if !allowed {
					want := i18n.Messages.Moderate.Common.CommandAdminOnly.Render(i18n.LangEN, command)
					if tc.lookupErr != nil {
						want = i18n.Messages.Moderate.Common.CallerAdminCheckFailed.For(i18n.LangEN)
					} else if tc.caller {
						want = i18n.Messages.Moderate.Common.TargetIsAdmin.For(i18n.LangEN)
					}
					assertRemoteReply(t, telegram, want)
				}
			})
		}
	}
}

func assertProtectedAuthorization(t *testing.T, telegram *remoteTelegram) {
	t.Helper()
	for _, check := range append(telegram.rightsChecks, telegram.freshChecks...) {
		if check.chatID != remoteGroupID {
			t.Fatalf("authorization used chat %d", check.chatID)
		}
	}
	if len(telegram.rightsChecks) != 1 {
		t.Fatal("caller was not checked live")
	}
}

func TestControlModerationBadArgumentsAndUnmatchedChats(t *testing.T) {
	for _, text := range []string{"/ban", "/ban -8", "/ban 0", "/ban +8", "/ban user", "/ban 8 extra", "/ban 9223372036854775808", "/mute 8 perm", "/mute 8 bad", "/mute 8 1h extra", "/unmute", "/warn abc"} {
		t.Run(text, func(t *testing.T) {
			service, telegram := remoteFixture(t)
			runRemote(t, service, telegram, text, remoteControlID)
			if len(telegram.replies) != 1 || telegram.replies[0].ChatID.ID != remoteControlID ||
				telegram.replies[0].ReplyParameters == nil || telegram.replies[0].ReplyParameters.MessageID != 11 || telegram.deletes != 0 {
				t.Fatal("bad arguments did not reply to the control command")
			}
			if len(telegram.actions) != 0 || len(telegram.rightsChecks) != 0 {
				t.Fatal("bad arguments reached Telegram action checks")
			}
		})
	}
	for _, chat := range []int64{0, -1009000000624} {
		service, telegram := remoteFixture(t)
		runRemote(t, service, telegram, "/ban 8", chat)
		if len(telegram.replies) != 0 || len(telegram.rightsChecks) != 0 || len(telegram.actions) != 0 {
			t.Fatal("unmatched chat was not silent")
		}
	}
	service, telegram := remoteFixture(t)
	runRemote(t, service, telegram, "/sb 8", remoteControlID)
	if len(telegram.replies) != 0 || len(telegram.actions) != 0 {
		t.Fatal("control chat admitted an unsupported command")
	}
}

func TestControlModerationActionsUseProtectedGroupAndDurations(t *testing.T) {
	for _, tc := range []struct {
		text, action string
		seconds      int
		audit        bool
	}{
		{"/ban@bot 8", "/ban", 3600, true},
		{"/mute 8", "/mute", 600, true},
		{"/mute 8 2h", "/mute", 7200, true},
		{"/unmute 8", "/unmute", 0, false},
		{"/warn 8", "", 0, false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			service, telegram := remoteFixture(t)
			runRemote(t, service, telegram, tc.text, remoteControlID)
			if tc.action != "" {
				want := []remoteAction{{tc.action, remoteGroupID, 8, tc.seconds}}
				if !reflect.DeepEqual(telegram.actions, want) {
					t.Fatalf("actions = %#v, want %#v", telegram.actions, want)
				}
			} else if service.warnings.counters[warningKey{groupID: remoteGroupID, userID: 8}] != 1 {
				t.Fatal("warning not recorded against protected group")
			}
			if len(telegram.replies) != 1 || telegram.replies[0].ReplyParameters == nil || telegram.replies[0].ChatID.ID != remoteControlID {
				t.Fatal("result did not reply in control chat")
			}
			if tc.audit && (len(telegram.notifications) != 1 || telegram.notifications[0].chatID != remoteAuditID) {
				t.Fatal("audit was not routed to protected group's log")
			}
			if telegram.deletes != 0 {
				t.Fatal("command message deleted")
			}
		})
	}
}

func TestControlModerationFailureAlertsAndWarnLimit(t *testing.T) {
	for _, command := range []string{"/ban", "/mute", "/warn"} {
		t.Run(command, func(t *testing.T) {
			service, telegram := remoteFixture(t)
			telegram.banErr = errors.New("restriction rejected")
			telegram.muteErr = telegram.banErr
			if command == "/warn" {
				service.warnings.increment(remoteGroupID, 8)
			}
			runRemote(t, service, telegram, command+" 8", remoteControlID)
			if len(telegram.failAlerts) != 1 || telegram.failAlerts[0].groupID != remoteGroupID || telegram.failAlerts[0].adminLogChatID != remoteAuditID {
				t.Fatalf("alerts = %#v", telegram.failAlerts)
			}
			if len(telegram.replies) != 1 || telegram.replies[0].ChatID.ID != remoteControlID {
				t.Fatal("failure did not reply in control chat")
			}
			if command == "/warn" && service.warnings.counters[warningKey{groupID: remoteGroupID, userID: 8}] != 2 {
				t.Fatal("failed limit kick lost warnings")
			}
		})
	}
	service, telegram := remoteFixture(t)
	runRemote(t, service, telegram, "/warn 8", remoteControlID)
	runRemote(t, service, telegram, "/warn 8", remoteControlID)
	if telegram.bans != 1 || telegram.unbans != 1 || service.warnings.counters[warningKey{groupID: remoteGroupID, userID: 8}] != 0 {
		t.Fatal("successful limit kick did not clear protected-group warnings")
	}
}

func TestControlChatAlsoProtectedPreservesReplyEntry(t *testing.T) {
	for _, reply := range []bool{false, true} {
		t.Run(strconv.FormatBool(reply), func(t *testing.T) {
			service, telegram := remoteFixture(t)
			service.cfg.Groups = append(service.cfg.Groups, settings.GroupConfig{ID: remoteControlID})
			service.settings = testSettings(t, service.cfg)
			message := moderationCommand(remoteControlID, "/ban 8")
			expected := remoteGroupID
			if reply {
				expected = remoteControlID
			} else {
				message.ReplyToMessage = nil
			}
			runFakeHandler(t, newAPITestBot(t, telegram), service.OnBan, telego.Update{Message: message})
			if len(telegram.actions) != 1 || telegram.actions[0].group != expected {
				t.Fatalf("actions = %#v, want group %d", telegram.actions, expected)
			}
			if reply && telegram.deletes != 2 {
				t.Fatal("reply-based command lost its existing cleanup")
			}
		})
	}
}
