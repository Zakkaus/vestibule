package panel

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

func TestSettingsStartConsoleWebApp(t *testing.T) {
	for _, test := range []struct {
		name, base, entry string
	}{
		{"https", "https://console.example.test", "https://console.example.test/groups?group=-1009000000502"},
		{"https path", "https://console.example.test/panel/?group=old#entry", "https://console.example.test/groups?group=-1009000000502"},
		{"https group route", "https://console.example.test/groups", "https://console.example.test/groups?group=-1009000000502"},
		{"https origin", "https://user:password@console.example.test:8443/other", "https://console.example.test:8443/groups?group=-1009000000502"},
		{"http", "http://localhost:8080", ""},
		{"empty", "", ""},
		{"malformed", "https://console.example.test/%zz", ""},
		{"missing host", "https:///console", ""},
		{"missing hostname", "https://:443", ""},
		{"relative", "/console", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel, _, caller, bot := newSettingsPanelTest(t, "")
			panel.SetConsoleURL(test.base)
			runFakeHandler(t, bot, panel.OnSettings, telego.Update{Message: &telego.Message{
				MessageID: 1, Chat: telego.Chat{ID: panelTestGroupB, Type: telego.ChatTypeSupergroup},
				From: &telego.User{ID: panelTestUser, LanguageCode: "en"}, Text: "/settings",
			}})
			if len(caller.lastInlineKeyboard) != 1 || len(caller.lastInlineKeyboard[0]) != 1 ||
				caller.lastInlineKeyboard[0][0].WebApp != nil {
				t.Fatalf("group launcher changed: %+v", caller.lastInlineKeyboard)
			}
			session := panel.sessionByUser(panelTestUser)
			if session == nil {
				t.Fatal("settings launcher did not create a session")
			}
			runFakeHandler(t, bot, panel.OnStart, telego.Update{Message: &telego.Message{
				MessageID: 2, Chat: telego.Chat{ID: panelTestUser, Type: telego.ChatTypePrivate},
				From: &telego.User{ID: panelTestUser, LanguageCode: "en"}, Text: "/start panel_" + session.token,
			}})
			assertSettingsStartKeyboard(t, caller.lastInlineKeyboard, test.entry)
			invokePanelCallback(t, panel, bot, session, session.groupID, "rf", "_")
			assertSettingsStartKeyboard(t, caller.lastEditKeyboard, test.entry)
		})
	}
}

func assertSettingsStartKeyboard(t *testing.T, rows [][]telego.InlineKeyboardButton, entry string) {
	t.Helper()
	var callbacks, webApps int
	for _, row := range rows {
		for _, button := range row {
			if button.CallbackData != "" {
				callbacks++
			}
			if button.WebApp != nil {
				webApps++
				if button.WebApp.URL != entry || button.URL != "" || button.CallbackData != "" {
					t.Errorf("Web App URL = %q, button = %+v, want entry %q without another action", button.WebApp.URL, button, entry)
				}
				if button.Text != i18n.Messages.Bot.Menu.Owner.ConsoleOpen.For(i18n.LangEN) {
					t.Errorf("Web App label = %q", button.Text)
				}
			}
		}
	}
	wantWebApps := 0
	if entry != "" {
		wantWebApps = 1
	}
	if callbacks != 4 || webApps != wantWebApps || len(rows) != 3+wantWebApps {
		t.Fatalf("private keyboard: callbacks=%d web_apps=%d rows=%d, want 4/%d/%d", callbacks, webApps, len(rows), wantWebApps, 3+wantWebApps)
	}
}
