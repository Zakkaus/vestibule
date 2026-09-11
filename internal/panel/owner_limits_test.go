package panel

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func panelSetOwnerLimit(t *testing.T, store *settings.Store, field string, limit int64) {
	t.Helper()
	state := store.OwnerLimits()
	if _, err := store.UpdateOwnerLimits(state.Revision, settings.LimitChanges{field: &limit}); err != nil {
		t.Fatal(err)
	}
}

func assertPanelLimitNotice(t *testing.T, notice, field, value, limit string) {
	t.Helper()
	for _, want := range []string{field, value, limit} {
		if !strings.Contains(notice, want) {
			t.Fatalf("owner-limit notice %q omitted %q", notice, want)
		}
	}
}

func TestPanelStopRefusesExistingOwnerLimitViolation(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, filepath.Join(t.TempDir(), "settings.json"))
	panel, verifier := newAdminTestApplication(t, panel.cfg, store, bot)
	t.Cleanup(verifier.Shutdown)
	panelSetOwnerLimit(t, store, "timeout_seconds", 100)
	before, _ := store.Settings(panelTestGroupA)
	runFakeHandler(t, bot, panel.OnStop, telego.Update{Message: &telego.Message{
		MessageID: 11, Chat: telego.Chat{ID: panelTestGroupA, Type: telego.ChatTypeSupergroup},
		From: &telego.User{ID: panelTestUser}, Text: "/stop",
	}})
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != before.Revision() || after.Enabled().Value != before.Enabled().Value {
		t.Fatalf("/stop changed over-limit settings: before %d/%v, after %d/%v", before.Revision(), before.Enabled().Value, after.Revision(), after.Enabled().Value)
	}
	assertPanelLimitNotice(t, caller.lastSendText, "timeout_seconds", "240", "100")
}

func TestPanelCallbackToggleRefusesExistingOwnerLimitViolation(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, filepath.Join(t.TempDir(), "settings.json"))
	panelSetOwnerLimit(t, store, "timeout_seconds", 100)
	before, _ := store.Settings(panelTestGroupA)
	session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "en", "_")
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != before.Revision() || after.Enabled().Value != before.Enabled().Value {
		t.Fatalf("callback changed over-limit settings: before %d/%v, after %d/%v", before.Revision(), before.Enabled().Value, after.Revision(), after.Enabled().Value)
	}
	assertPanelLimitNotice(t, caller.lastAnswerText, "timeout_seconds", "240", "100")
}

func TestPanelNumericInputRefusesOwnerLimitViolation(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, filepath.Join(t.TempDir(), "settings.json"))
	panelSetOwnerLimit(t, store, "timeout_seconds", 100)
	before, _ := store.Settings(panelTestGroupA)
	session := addPanelSession(t, panel, store, panelTestGroupA, "in")
	session.pending = &pendingInput{kind: inputTimeout, parent: "vp", promptMessageID: 71, expectedRevision: session.revision}
	submitPanelText(t, panel, bot, session, "300")
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != before.Revision() || after.TimeoutSeconds().Value != before.TimeoutSeconds().Value {
		t.Fatalf("numeric input changed over-limit settings: before %d/%d, after %d/%d", before.Revision(), before.TimeoutSeconds().Value, after.Revision(), after.TimeoutSeconds().Value)
	}
	assertPanelLimitNotice(t, caller.lastEditText, "timeout_seconds", "300", "100")
}

func TestPanelSharedChatCountRefusesOwnerLimitViolation(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, filepath.Join(t.TempDir(), "settings.json"))
	group, _ := store.Settings(panelTestGroupA)
	known := []int64{-1009000000802}
	overrides := group.Overrides()
	overrides.KnownChatIDs = &known
	if _, err := store.Update(panelTestGroupA, group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	panelSetOwnerLimit(t, store, "known_chat_ids", 1)
	before, _ := store.Settings(panelTestGroupA)
	session := addPanelSession(t, panel, store, panelTestGroupA, "in")
	session.pending = &pendingInput{kind: inputKnownChat, parent: "li", promptMessageID: 72, requestID: 7, expectedRevision: session.revision}
	submitSharedChat(t, panel, bot, session, -1009000000803)
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != before.Revision() || len(after.KnownChatIDs().Value) != len(before.KnownChatIDs().Value) {
		t.Fatalf("shared-chat input changed over-limit settings: before %d/%v, after %d/%v", before.Revision(), before.KnownChatIDs().Value, after.Revision(), after.KnownChatIDs().Value)
	}
	assertPanelLimitNotice(t, caller.lastEditText, "known_chat_ids", "2", "1")
}
