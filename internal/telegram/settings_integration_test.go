package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
)

func botTestSettingsBaseline(t *testing.T, cfg *config.Config) store.SettingsBaseline {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.LoadBaseline(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return baseline
}

func TestDistroAliasVisible(t *testing.T) {
	var description string
	for _, command := range memberCommands(i18n.LangZH) {
		if command.Command == "distro" {
			description = command.Description
			break
		}
	}
	want := i18n.Messages.Bot.Menu.Member.Distro.For(i18n.LangZH)
	if description != want {
		t.Errorf("/distro menu description = %q, want catalogue text %q", description, want)
	}
	if !strings.Contains(description, "/pkgs") {
		t.Errorf("/distro menu description = %q, want an explicit /pkgs alias", description)
	}
}

func TestBuiltInPrivateReplyUsesCatalogue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"groups":[{"id":-1001,"verify_mode":"kernel"}]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !isBuiltInPrivateReply(cfg.PrivateReply) {
		t.Fatal("normalized built-in private reply was not recognized")
	}
	handler := dmHandler{cfg: cfg, catalogueReply: true}
	for _, language := range i18n.Languages() {
		got := handler.privateReply(language)
		want := i18n.Messages.Bot.DirectMessage.AutoReply.Render(language, cfg.PrivateQueryPerMin, i18n.Messages.Bot.DirectMessage.Who(language))
		if got != want {
			t.Errorf("private reply for %s = %q, want catalogue text %q", language, got, want)
		}
	}

	customReply := i18n.Messages.Bot.DirectMessage.AutoReply.Render(i18n.LangEN, 99, i18n.Messages.Bot.DirectMessage.Who(i18n.LangEN))
	if isBuiltInPrivateReply(customReply) {
		t.Fatal("custom private reply was recognized as built-in")
	}
	handler = dmHandler{cfg: &config.Config{PrivateReply: customReply}}
	if got := handler.privateReply(i18n.LangZHHant); got != customReply {
		t.Errorf("custom private reply = %q, want %q", got, customReply)
	}
}

func TestBuiltInPrivateReplyUsesLiveQueryRate(t *testing.T) {
	cfg := &config.Config{PrivateQueryPerMin: 3}
	settings, err := store.NewSettings("", botTestSettingsBaseline(t, cfg))
	if err != nil {
		t.Fatal(err)
	}
	service := NewUpdates(cfg, settings, nil, HandlerSet{})
	global := settings.Global()
	overrides := global.Overrides()
	rate := 5
	overrides.PrivateQueryPerMin = &rate
	if _, err := settings.CommitGlobal(global.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	got := service.dm.privateReply(i18n.LangEN)
	want := i18n.Messages.Bot.DirectMessage.AutoReply.Render(i18n.LangEN, rate, i18n.Messages.Bot.DirectMessage.Who(i18n.LangEN))
	if got != want {
		t.Errorf("private reply = %q, want catalogue text %q", got, want)
	}
}
