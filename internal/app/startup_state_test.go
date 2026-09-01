package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/database"
)

func TestNewServicesFailsWhenPendingStateCannotLoad(t *testing.T) {
	ctx := context.Background()
	stateDirectory := t.TempDir()
	db, err := database.Open(ctx, database.Config{StateDirectory: stateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DROP TABLE challenge"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
	}))
	t.Cleanup(api.Close)

	runtime, err := newServices(ctx, Options{
		ConfigPath:     filepath.Join(stateDirectory, "missing-config.json"),
		StateDirectory: stateDirectory,
		Token:          "1:" + strings.Repeat("a", 35),
		TelegramAPIURL: api.URL,
	}, make(chan struct{}, 1))
	if runtime != nil {
		t.Cleanup(func() { _ = runtime.database.Close() })
	}
	t.Logf("newServices error=%v runtime_nil=%t", err, runtime == nil)
	if err == nil {
		t.Fatal("newServices continued after LoadPending failed")
	}
	if runtime != nil {
		t.Fatal("newServices returned a runnable service graph after LoadPending failed")
	}
	if !strings.Contains(err.Error(), "load pending challenges") {
		t.Fatalf("startup error %q does not identify the failed authoritative query", err)
	}
}
