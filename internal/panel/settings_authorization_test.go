package panel

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

const (
	panelTestOtherUser      int64 = 78
	panelTestUnmanagedGroup int64 = -1009000000999
	panelTestSharedGroup    int64 = -1009000000777
	panelTestRotationTokenA       = "aaaaaaaaaaaaaaaa"
	panelTestRotationTokenB       = "bbbbbbbbbbbbbbbb"
	panelTestRotationTokenC       = "cccccccccccccccc"
)

func openPanelLink(t *testing.T, panel *Panel, bot *telego.Bot, userID int64, token string) {
	t.Helper()
	runFakeHandler(t, bot, panel.OnStart, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: userID, Type: "private"},
		From: &telego.User{ID: userID, LanguageCode: "en"},
		Text: "/start panel_" + token,
	}})
}

func invokePanelCallbackAs(
	t *testing.T,
	panel *Panel,
	bot *telego.Bot,
	session *panelSession,
	data callbackData,
	userID, chatID int64,
	chatType string,
	messageID int,
) {
	t.Helper()
	data.token = session.token
	encoded, err := encodeCallback(data)
	if err != nil {
		t.Fatal(err)
	}
	runFakeHandler(t, bot, panel.OnSettingsCallback, telego.Update{CallbackQuery: &telego.CallbackQuery{
		ID: "callback", From: telego.User{ID: userID, LanguageCode: "en"}, Data: encoded,
		Message: &telego.Message{MessageID: messageID, Chat: telego.Chat{ID: chatID, Type: chatType}},
	}})
}

func armTimeoutInput(t *testing.T, panel *Panel, bot *telego.Bot, session *panelSession) {
	t.Helper()
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "to", "_")
	if session.pending == nil || session.pending.promptMessageID == 0 {
		t.Fatal("timeout callback did not arm a ForceReply prompt")
	}
}

func armTrustedGroupInput(t *testing.T, panel *Panel, bot *telego.Bot, session *panelSession) {
	t.Helper()
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "tg", "_")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "ca", "tg")
	if session.pending == nil || session.pending.requestID == 0 {
		t.Fatal("trusted-group callback did not arm a chat picker")
	}
}

func panelTextReply(session *panelSession, chatID int64, chatType string, replyID int, text string) telego.Update {
	return telego.Update{Message: &telego.Message{
		MessageID:      session.pending.promptMessageID + 1000,
		Chat:           telego.Chat{ID: chatID, Type: chatType},
		From:           &telego.User{ID: panelTestUser, LanguageCode: "en"},
		Text:           text,
		ReplyToMessage: &telego.Message{MessageID: replyID},
	}}
}

func containsPanelChat(chats []int64, want int64) bool {
	for _, chatID := range chats {
		if chatID == want {
			return true
		}
	}
	return false
}

func newPanelSessionForLifecycleTest(t *testing.T) (*Panel, *panelSession) {
	t.Helper()
	panel, _, _, _ := newSettingsPanelTest(t, "")
	session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	return panel, session
}

func TestSettingsDeepLinksAreLimitedToTheirOwnerAndCurrentAdministrators(t *testing.T) {
	t.Run("allows the active owner", func(t *testing.T) {
		panel, _, _, bot := newSettingsPanelTest(t, "")
		session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
		if err != nil {
			t.Fatal(err)
		}

		openPanelLink(t, panel, bot, panelTestUser, session.token)
		if session.chatID != panelTestUser || session.messageID == 0 {
			t.Fatalf("owner could not open panel: chat=%d message=%d", session.chatID, session.messageID)
		}
	})

	t.Run("rejects another active administrator", func(t *testing.T) {
		panel, _, caller, bot := newSettingsPanelTest(t, "")
		session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
		if err != nil {
			t.Fatal(err)
		}

		openPanelLink(t, panel, bot, panelTestOtherUser, session.token)
		if session.chatID != panelTestUser || session.messageID != 0 {
			t.Fatalf("another administrator opened owner session: chat=%d message=%d", session.chatID, session.messageID)
		}
		want := i18n.Messages.Panel.Settings.Error.Expired.For(i18n.LangEN)
		if caller.lastSendText != want {
			t.Fatalf("other administrator notice = %q, want %q", caller.lastSendText, want)
		}
	})

	t.Run("rejects the owner after demotion", func(t *testing.T) {
		panel, _, caller, bot := newSettingsPanelTest(t, "")
		session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
		if err != nil {
			t.Fatal(err)
		}
		caller.admin = false

		openPanelLink(t, panel, bot, panelTestUser, session.token)
		if panel.sessionByUser(panelTestUser) != nil {
			t.Fatal("demoted owner retained a live settings session")
		}
		want := i18n.Messages.Panel.Settings.Error.AuthorizationLost.For(i18n.LangEN)
		if caller.lastSendText != want {
			t.Fatalf("demoted owner notice = %q, want %q", caller.lastSendText, want)
		}
	})
}

func TestSettingsCallbacksUseTheGroupTheyAuthorized(t *testing.T) {
	t.Run("allows a callback for its session group", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "rt")

		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "en", "_")
		group, _ := settings.Settings(panelTestGroupA)
		if group.Enabled().Value {
			t.Fatal("authorized callback did not update its session group")
		}
	})

	t.Run("rejects a foreign group on the group list", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "gl")

		oldToken := session.token
		invokePanelCallback(t, panel, bot, session, panelTestGroupB, "pg", "1")
		if session.token != oldToken {
			t.Fatal("foreign group-list callback refreshed the current panel")
		}
	})

	t.Run("rejects a foreign group before a settings write", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "rt")

		invokePanelCallback(t, panel, bot, session, panelTestGroupB, "en", "_")
		group, _ := settings.Settings(panelTestGroupA)
		if !group.Enabled().Value {
			t.Fatal("foreign authorization wrote to the session group")
		}
	})
}

func TestSettingsCallbacksRequireOwnerScreenAndPanelMessage(t *testing.T) {
	t.Run("allows the current private panel", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "rt")

		invokePanelCallbackAs(t, panel, bot, session,
			callbackData{screen: "rt", group: panelTestGroupA, field: "en", value: "_"},
			panelTestUser, panelTestUser, "private", session.messageID)
		group, _ := settings.Settings(panelTestGroupA)
		if group.Enabled().Value {
			t.Fatal("matching panel callback did not update settings")
		}
	})

	cases := []struct {
		name       string
		session    string
		dataScreen string
		userID     int64
		chatID     int64
		chatType   string
		messageID  int
	}{
		{"different owner", "rt", "rt", panelTestOtherUser, panelTestUser, "private", 90},
		{"different screen", "vp", "rt", panelTestUser, panelTestUser, "private", 90},
		{"non-private chat", "rt", "rt", panelTestUser, panelTestUser, "supergroup", 90},
		{"different private chat", "rt", "rt", panelTestUser, panelTestOtherUser, "private", 90},
		{"different panel message", "rt", "rt", panelTestUser, panelTestUser, "private", 91},
	}
	for _, testCase := range cases {
		t.Run("rejects "+testCase.name, func(t *testing.T) {
			panel, settings, _, bot := newSettingsPanelTest(t, "")
			session := addPanelSession(t, panel, settings, panelTestGroupA, testCase.session)

			invokePanelCallbackAs(t, panel, bot, session,
				callbackData{screen: testCase.dataScreen, group: panelTestGroupA, field: "en", value: "_"},
				testCase.userID, testCase.chatID, testCase.chatType, testCase.messageID)
			group, _ := settings.Settings(panelTestGroupA)
			if !group.Enabled().Value {
				t.Fatalf("%s callback changed settings", testCase.name)
			}
		})
	}
}

func TestPanelInputsRequireCurrentAdministration(t *testing.T) {
	t.Run("allows an active administrator to submit text", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		armTimeoutInput(t, panel, bot, session)

		submitPanelText(t, panel, bot, session, "300")
		group, _ := settings.Settings(panelTestGroupA)
		if group.TimeoutSeconds().Value != 300 {
			t.Fatalf("active administrator timeout = %d, want 300", group.TimeoutSeconds().Value)
		}
	})

	t.Run("rejects text from a demoted administrator", func(t *testing.T) {
		panel, settings, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		before, _ := settings.Settings(panelTestGroupA)
		armTimeoutInput(t, panel, bot, session)
		caller.admin = false

		submitPanelText(t, panel, bot, session, "300")
		group, _ := settings.Settings(panelTestGroupA)
		if group.TimeoutSeconds().Value != before.TimeoutSeconds().Value {
			t.Fatal("demoted administrator changed timeout through a pending prompt")
		}
	})

	t.Run("allows an active administrator to submit a shared chat", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "ls")
		armTrustedGroupInput(t, panel, bot, session)

		submitSharedChat(t, panel, bot, session, panelTestSharedGroup)
		group, _ := settings.Settings(panelTestGroupA)
		if !containsPanelChat(group.TrustedMemberGroupIDs().Value, panelTestSharedGroup) {
			t.Fatal("active administrator shared chat was not saved")
		}
	})

	t.Run("rejects a shared chat from a demoted administrator", func(t *testing.T) {
		panel, settings, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "ls")
		armTrustedGroupInput(t, panel, bot, session)
		caller.admin = false

		submitSharedChat(t, panel, bot, session, panelTestSharedGroup)
		group, _ := settings.Settings(panelTestGroupA)
		if containsPanelChat(group.TrustedMemberGroupIDs().Value, panelTestSharedGroup) {
			t.Fatal("demoted administrator changed trusted groups through a chat picker")
		}
	})
}

func TestPanelSessionsExpireAtTheirLifetimeAndStayBounded(t *testing.T) {
	t.Run("expires after its configured lifetime", func(t *testing.T) {
		panel, session := newPanelSessionForLifecycleTest(t)
		if got := session.expiresAt.Sub(session.createdAt); got != panelSessionTTL {
			t.Fatalf("session lifetime = %s, want %s", got, panelSessionTTL)
		}
		if panel.sessionByToken(session.token) != session {
			t.Fatal("new session was not available before expiration")
		}
		session.expiresAt = time.Now().Add(-time.Second)

		if panel.sessionByToken(session.token) != nil || panel.sessionByUser(session.ownerID) != nil {
			t.Fatal("expired session remained redeemable")
		}
	})

	t.Run("refuses sessions beyond the capacity limit", func(t *testing.T) {
		panel, _, _, _ := newSettingsPanelTest(t, "")
		for index := range panelSessionCap {
			if _, err := panel.newSettingsSession(int64(index+1), panelTestGroupA, i18n.LangEN); err != nil {
				t.Fatalf("session %d: %v", index, err)
			}
		}
		if _, err := panel.newSettingsSession(int64(panelSessionCap+1), panelTestGroupA, i18n.LangEN); err == nil {
			t.Fatal("session table accepted a session beyond its capacity")
		}
		panel.panelState.mu.Lock()
		count := len(panel.panelState.byToken)
		panel.panelState.mu.Unlock()
		if count != panelSessionCap {
			t.Fatalf("session table size = %d, want %d", count, panelSessionCap)
		}
	})
}

func TestPanelTokenRotationRequiresLiveRegistration(t *testing.T) {
	t.Run("rotates a live session", func(t *testing.T) {
		panel, session := newPanelSessionForLifecycleTest(t)
		if !panel.rotateSessionToken(session, panelTestRotationTokenA) {
			t.Fatal("live session token did not rotate")
		}
		if panel.sessionByToken(panelTestRotationTokenA) != session {
			t.Fatal("rotated token did not resolve its live session")
		}
	})

	t.Run("does not restore a session missing its token registration", func(t *testing.T) {
		panel, session := newPanelSessionForLifecycleTest(t)
		oldToken := session.token
		panel.panelState.mu.Lock()
		delete(panel.panelState.byToken, oldToken)
		panel.panelState.mu.Unlock()

		if panel.rotateSessionToken(session, panelTestRotationTokenB) {
			t.Fatal("rotation restored a session without its token registration")
		}
		if session.token != oldToken {
			t.Fatalf("rejected token rotation changed token from %q to %q", oldToken, session.token)
		}
		panel.panelState.mu.Lock()
		_, restored := panel.panelState.byToken[panelTestRotationTokenB]
		panel.panelState.mu.Unlock()
		if restored {
			t.Fatal("rejected token rotation registered a replacement token")
		}
	})

	t.Run("does not replace a session with another owner registration", func(t *testing.T) {
		panel, session := newPanelSessionForLifecycleTest(t)
		oldToken := session.token
		panel.panelState.mu.Lock()
		panel.panelState.byUser[session.ownerID] = &panelSession{}
		panel.panelState.mu.Unlock()

		if panel.rotateSessionToken(session, panelTestRotationTokenC) {
			t.Fatal("rotation accepted a session no longer registered to its owner")
		}
		if session.token != oldToken {
			t.Fatalf("rejected owner rotation changed token from %q to %q", oldToken, session.token)
		}
		panel.panelState.mu.Lock()
		_, restored := panel.panelState.byToken[panelTestRotationTokenC]
		panel.panelState.mu.Unlock()
		if restored {
			t.Fatal("rejected owner rotation registered a replacement token")
		}
	})
}

func TestSettingsCallbacksRejectUnmanagedGroups(t *testing.T) {
	t.Run("allows a managed group", func(t *testing.T) {
		panel, settings, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "rt")

		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "en", "_")
		group, _ := settings.Settings(panelTestGroupA)
		if group.Enabled().Value || caller.memberCalls.Load() == 0 {
			t.Fatal("managed callback was not authorized and applied")
		}
	})

	t.Run("does not inspect membership for an unmanaged group", func(t *testing.T) {
		panel, settings, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "rt")

		invokePanelCallback(t, panel, bot, session, panelTestUnmanagedGroup, "en", "_")
		group, _ := settings.Settings(panelTestGroupA)
		if !group.Enabled().Value {
			t.Fatal("unmanaged callback changed the session group")
		}
		if calls := caller.memberCalls.Load(); calls != 0 {
			t.Fatalf("unmanaged callback performed %d membership lookups", calls)
		}
	})
}

func TestPanelInputRequiresPrivateExactPromptReplies(t *testing.T) {
	t.Run("matches an exact private prompt reply", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		armTimeoutInput(t, panel, bot, session)
		update := panelTextReply(session, panelTestUser, "private", session.pending.promptMessageID, "300")

		if !panel.PanelInputDM(context.Background(), update) {
			t.Fatal("exact private prompt reply did not match")
		}
	})

	t.Run("rejects an exact prompt reply from a group", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		armTimeoutInput(t, panel, bot, session)
		update := panelTextReply(session, panelTestGroupA, "supergroup", session.pending.promptMessageID, "300")

		if panel.PanelInputDM(context.Background(), update) {
			t.Fatal("group reply matched a private settings prompt")
		}
	})

	t.Run("applies a text reply to its exact prompt", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		armTimeoutInput(t, panel, bot, session)
		update := panelTextReply(session, panelTestUser, "private", session.pending.promptMessageID, "300")

		runFakeHandler(t, bot, panel.OnPanelInput, update)
		group, _ := settings.Settings(panelTestGroupA)
		if group.TimeoutSeconds().Value != 300 {
			t.Fatalf("exact prompt reply timeout = %d, want 300", group.TimeoutSeconds().Value)
		}
	})

	t.Run("ignores a reply to another message", func(t *testing.T) {
		panel, settings, _, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, settings, panelTestGroupA, "vp")
		before, _ := settings.Settings(panelTestGroupA)
		armTimeoutInput(t, panel, bot, session)
		update := panelTextReply(session, panelTestUser, "private", session.pending.promptMessageID+1, "300")

		runFakeHandler(t, bot, panel.OnPanelInput, update)
		group, _ := settings.Settings(panelTestGroupA)
		if group.TimeoutSeconds().Value != before.TimeoutSeconds().Value {
			t.Fatal("reply to another message changed settings")
		}
	})
}
