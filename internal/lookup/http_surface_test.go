package lookup_test

import (
	"path/filepath"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestRequesterLanguageFallbackChain(t *testing.T) {
	const groupID int64 = -100
	service := newLookupTestService(nil, nil, &settings.Config{
		Groups:   []settings.GroupConfig{{ID: groupID, Lang: "zh-Hant"}},
		GroupIDs: []int64{groupID},
	}, "")
	tests := []struct {
		Name     string
		Chat     telego.Chat
		Code     string
		Expected i18n.Lang
	}{
		{Name: "requester overrides group", Chat: telego.Chat{ID: groupID, Type: "supergroup"}, Code: "en", Expected: i18n.LangEN},
		{Name: "supported Chinese overrides group", Chat: telego.Chat{ID: groupID, Type: "supergroup"}, Code: "zh-CN", Expected: i18n.LangZH},
		{Name: "unsupported falls back to group", Chat: telego.Chat{ID: groupID, Type: "supergroup"}, Code: "fr", Expected: i18n.LangZHHant},
		{Name: "missing falls back to group", Chat: telego.Chat{ID: groupID, Type: "supergroup"}, Expected: i18n.LangZHHant},
		{Name: "unsupported DM falls back to English", Chat: telego.Chat{ID: 7, Type: "private"}, Code: "fr", Expected: i18n.LangEN},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			msg := &telego.Message{Chat: test.Chat, From: &telego.User{ID: 7, LanguageCode: test.Code}}
			if got := service.RequesterLanguage(msg); got != test.Expected {
				t.Fatalf("requester language = %s, want %s", got, test.Expected)
			}
		})
	}
}

func TestRuntimeRegisteredGroupUsesLiveMembership(t *testing.T) {
	const groupID int64 = -1009000000401
	cfg := &settings.Config{Lang: "zh-Hant"}
	baseline, err := settings.LoadBaseline(filepath.Join(t.TempDir(), "missing-config.json"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := newLookupTestService(store, nil, cfg, "")
	registration := store.Registrations()
	registration.RegisteredGroups = []settings.RegisteredGroup{{ID: groupID, RegisteredBy: 42}}
	if _, err := store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	group, _ := store.Settings(groupID)
	overrides := group.Overrides()
	language := "zh-Hant"
	overrides.Lang = &language
	if _, err := store.Update(groupID, group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	msg := &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup},
		From: &telego.User{ID: 7, LanguageCode: "fr"},
	}
	if got := service.RequesterLanguage(msg); got != i18n.LangZHHant {
		t.Errorf("runtime group requester language = %s, want %s", got, i18n.LangZHHant)
	}
	if !service.QueryAllowed(nil, msg, i18n.LangZHHant) {
		t.Error("runtime group lookup was not exempted from the private-query rate limit")
	}
}
