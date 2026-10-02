package panel

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
)

func TestHelpUsesEffectiveGroupModerationSettings(t *testing.T) {
	for _, projected := range []bool{false, true} {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		group := panelTestGroup(t, store)
		next := group.Overrides()
		limit := 5
		next.WarnLimit = &limit
		muteSeconds := 7200
		next.MuteSeconds = &muteSeconds
		if _, err := store.Update(panelTestGroupA, group.Revision(), next); err != nil {
			t.Fatal(err)
		}
		if projected {
			commands, err := telegram.NewCommandModules(telegram.CommandModule{Name: "moderation", Commands: []telegram.CommandDefinition{
				{Name: "warn", Description: i18n.Messages.Bot.Menu.Admin.Warn.For, Audience: telegram.CommandAdministrator, External: true},
				{Name: "mute", Description: i18n.Messages.Bot.Menu.Admin.Mute.For, Audience: telegram.CommandAdministrator, External: true},
			}})
			if err != nil {
				t.Fatal(err)
			}
			panel.SetCommandModules(commands)
		}
		runFakeHandler(t, bot, panel.OnHelp, telego.Update{Message: &telego.Message{
			Chat: telego.Chat{ID: panelTestGroupA, Type: telego.ChatTypeSupergroup},
			From: &telego.User{ID: panelTestUser, LanguageCode: "en"}, Text: "/help",
		}})
		if !strings.Contains(caller.lastSendText, "5 warnings") || strings.Contains(caller.lastSendText, "3 warnings") {
			t.Fatalf("projected=%v help has wrong effective threshold: %s", projected, caller.lastSendText)
		}
		if !strings.Contains(caller.lastSendText, "defaults to 2 hours") || strings.Contains(caller.lastSendText, "defaults to 1h") {
			t.Fatalf("projected=%v help has wrong effective mute duration: %s", projected, caller.lastSendText)
		}
	}
}

func TestPrivateRatePanelActionIsRefused(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "vp")
	before := session.revision
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "pr", "_")
	group := panelTestGroup(t, store)
	if session.pending != nil || group.Revision() != before || caller.lastAnswerText == "" {
		t.Fatalf("obsolete process editor accepted: pending=%v revision=%d answer=%q", session.pending, group.Revision(), caller.lastAnswerText)
	}
}
