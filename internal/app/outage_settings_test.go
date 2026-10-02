package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestRetentionAlertUsesEffectiveGroupSettings(t *testing.T) {
	const groupA, groupB, globalLog, groupLog = int64(-1009000000901), int64(-1009000000902), int64(-1009000000903), int64(-1009000000904)
	ctx := context.Background()
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.json")
	data := []byte(`{"lang":"en","admin_log_chat_id":-1009000000903,"groups":[{"id":-1009000000901,"admin_log_chat_id":-1009000000904},{"id":-1009000000902}]}`)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, store, err := loadRuntimeState(configPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		id   int64
		lang string
		log  *int64
	}{{groupA, "ru", nil}, {groupB, "ja", new(int64)}} {
		group, _ := store.Settings(change.id)
		next := group.Overrides()
		next.Lang, next.AdminLogChatID = &change.lang, change.log
		if _, err := store.Update(change.id, group.Revision(), next); err != nil {
			t.Fatal(err)
		}
	}
	_, restored, err := loadRuntimeState(configPath, directory)
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, database.Config{StateDirectory: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := database.NewVerificationStore(db)
	if err := state.SaveHeartbeat("", verification.HeartbeatRecord{LastOnline: time.Now().Add(-25 * time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	caller := &lifecycleCaller{botID: 950}
	bot := newOutageAwareBot(ctx, newLifecycleBot(t, caller), restored, state, nil)
	if _, err := bot.GetMe(ctx); err != nil {
		t.Fatal(err)
	}
	messages := caller.sentMessages()
	if len(messages) != 2 {
		t.Fatalf("recovery alerts = %#v, want one per group", messages)
	}
	for index, want := range []struct {
		group, target int64
		lang          i18n.Lang
	}{{groupA, groupLog, i18n.LangRU}, {groupB, groupB, i18n.LangJA}} {
		message := messages[index]
		text := i18n.Messages.Verification.Admin.OutageBacklog.Render(want.lang, want.group)
		t.Logf("recovery group=%d target=%d text=%s", want.group, message.ChatID.ID, message.Text)
		if message.ChatID.ID != want.target || message.Text != text {
			t.Errorf("group %d recovery alert = %d/%q, want %d/%q (not global log %d)", want.group, message.ChatID.ID, message.Text, want.target, text, globalLog)
		}
	}
}
