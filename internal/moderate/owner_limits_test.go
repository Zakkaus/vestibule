package moderate

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestBanTimeOwnerLimitRefusalKeepsSettings(t *testing.T) {
	const groupID int64 = -1009000000801
	cfg := &settings.Config{
		GroupIDs:         []int64{groupID},
		Groups:           []settings.GroupConfig{{ID: groupID}},
		BanSeconds:       30,
		NotifyTTLSeconds: -1,
		Lang:             "en",
	}
	baseline, err := settings.LoadBaseline(filepath.Join(t.TempDir(), "missing-config.json"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(60)
	if _, err := store.UpdateOwnerLimits(store.OwnerLimits().Revision, settings.LimitChanges{"ban_seconds": &limit}); err != nil {
		t.Fatal(err)
	}
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{7: fullRightsAdministrator()}
	service, err := New(store, telegram, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := store.Settings(groupID)
	if !ok {
		t.Fatal("missing test group")
	}

	runFakeHandler(t, newAPITestBot(t, telegram), service.OnBanTime, telego.Update{Message: moderationCommand(groupID, "/bantime 120")})

	after, _ := store.Settings(groupID)
	if after.Revision() != before.Revision() || after.BanSeconds().Value != before.BanSeconds().Value {
		t.Fatalf("owner-limit refusal changed settings: before revision/value %d/%d, after %d/%d", before.Revision(), before.BanSeconds().Value, after.Revision(), after.BanSeconds().Value)
	}
	for _, want := range []string{"ban_seconds", "120", "60"} {
		if !strings.Contains(telegram.lastSendText, want) {
			t.Fatalf("owner-limit notice %q omitted %q", telegram.lastSendText, want)
		}
	}
}
