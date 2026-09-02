package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
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

func TestNewServicesAllowsAllOptionalModulesDisabled(t *testing.T) {
	ctx := context.Background()
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"disabled_modules":["gentoo","linux"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
	}))
	t.Cleanup(api.Close)

	runtime, err := newServices(ctx, Options{
		ConfigPath:     configPath,
		StateDirectory: stateDirectory,
		Token:          "1:" + strings.Repeat("a", 35),
		TelegramAPIURL: api.URL,
	}, make(chan struct{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtime.verification.Shutdown()
		_ = runtime.database.Close()
	})
	if runtime.modules.commands.HasPrivateQueries() {
		t.Fatal("all-disabled service graph still accepts private lookup commands")
	}
	if done := runtime.modules.Start(ctx); done != nil {
		t.Fatal("all-disabled service graph started optional background services")
	}
	for module, names := range optionalModuleCommands {
		for _, name := range names {
			if routeCommandNames(runtime.modules.commands.Definitions())[name] {
				t.Errorf("all-disabled service graph registered %s command /%s", module, name)
			}
		}
	}
	if runtime.cfg.ModuleEnabled(settings.ModuleGentoo) || runtime.cfg.ModuleEnabled(settings.ModuleLinux) {
		t.Fatal("all-disabled configuration was not retained by the service graph")
	}
}
