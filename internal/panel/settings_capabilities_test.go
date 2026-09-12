package panel

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

type panelCapabilitySettings struct {
	*settings.Store
	afterUpdate func()
}

func (s panelCapabilitySettings) Update(
	groupID int64,
	revision uint64,
	next settings.GroupOverrides,
) (settings.CommitResult, error) {
	result, err := s.Store.Update(groupID, revision, next)
	if err == nil && s.afterUpdate != nil {
		s.afterUpdate()
	}
	return result, err
}

func TestPanelRuntimeExposesAndCommitsLookupCapabilities(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	panel.settings = panelCapabilitySettings{
		Store:       store,
		afterUpdate: func() { caller.commandMenus.Add(2) },
	}
	const wantScreen = "rt"

	for _, language := range i18n.Languages() {
		session := addPanelSession(t, panel, store, panelTestGroupA, wantScreen)
		session.language = language
		text, keyboard, err := panel.buildScreen(context.Background(), bot, session, panelTestGroupA, session.token)
		if err != nil {
			t.Fatalf("%s runtime screen: %v", language.String(), err)
		}
		lower := strings.ToLower(text)
		for _, module := range []string{"gentoo", "linux"} {
			if !strings.Contains(lower, module) {
				t.Errorf("%s runtime screen omits %s lookup capability", language.String(), module)
			}
		}
		actions := panelCallbackActions(t, keyboard, session.token, wantScreen)
		for _, field := range []string{"gt", "lx"} {
			found := false
			for _, action := range actions {
				if strings.Contains(action, ":"+field+":") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s runtime screen omits %s lookup toggle callback", language.String(), field)
			}
		}
	}

	session := addPanelSession(t, panel, store, panelTestGroupA, wantScreen)
	beforeMenus := caller.commandMenus.Load()
	wantMenus := beforeMenus
	for _, field := range []string{"gt", "lx"} {
		before, ok := store.Settings(panelTestGroupA)
		if !ok {
			t.Fatal("missing panel test group")
		}
		encoded := fmt.Sprintf("p1:%s:%s:%s:%s:_",
			session.token, wantScreen, encodeSigned(panelTestGroupA), field)
		runFakeHandler(t, bot, panel.OnSettingsCallback, telego.Update{
			CallbackQuery: &telego.CallbackQuery{
				ID:   "capability-" + field,
				From: telego.User{ID: panelTestUser, LanguageCode: "en"},
				Data: encoded,
				Message: &telego.Message{
					MessageID: session.messageID,
					Chat:      telego.Chat{ID: panelTestUser, Type: "private"},
				},
			},
		})
		after, ok := store.Settings(panelTestGroupA)
		if !ok {
			t.Fatal("missing panel test group after callback")
		}
		if after.Revision() != before.Revision()+1 {
			t.Errorf("%s lookup callback revision = %d, want %d", field, after.Revision(), before.Revision()+1)
		}
		wantMenus += 2
		if got := caller.commandMenus.Load(); got != wantMenus {
			t.Errorf("%s lookup callback command-menu refreshes = %d, want one member/admin pair", field, got-beforeMenus)
		}
	}
}
