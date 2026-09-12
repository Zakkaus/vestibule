package app

import (
	"context"
	"encoding/json"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var optionalModuleCommands = map[string][]string{
	settings.ModuleGentoo: {"pkg", "use", "bug", "news", "arm"},
	settings.ModuleLinux:  {"wiki", "bbs", "pkgs", "distro", "armpkgs", "kernel", "man", "cve", "repology"},
}

func TestEmptyModulesDisappearFromCommandSurface(t *testing.T) {
	cfg := &settings.Config{Modules: []string{}}
	modules, err := newRuntimeModules(cfg, nil, t.TempDir(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if modules.commands.HasPrivateQueries() {
		t.Fatal("disabled lookup modules still allow private queries")
	}
	if done := modules.Start(context.Background()); done != nil {
		t.Fatal("disabled lookup modules still started a background service")
	}

	member := commandNames(modules.commands.MemberMenu(i18n.LangEN))
	routes := routeCommandNames(modules.commands.Definitions())
	memberHelp := modules.commands.MemberHelp(i18n.LangEN)
	adminHelp := modules.commands.AdministratorHelp(i18n.LangEN, 3)
	for module, names := range optionalModuleCommands {
		for _, name := range names {
			if member[name] || routes[name] {
				t.Errorf("disabled %s command /%s remains registered", module, name)
			}
			if strings.Contains(memberHelp, "/"+name) || strings.Contains(adminHelp, "/"+name) {
				t.Errorf("disabled %s command /%s remains in /help", module, name)
			}
		}
	}

}

// A zero-value process configuration intentionally has no lookup modules. The
// process can still expose core administration commands, but an operator must
// opt in to Gentoo and Linux lookup surfaces explicitly.
func TestRuntimeModulesDefaultToNoOptionalModules(t *testing.T) {
	modules, err := newRuntimeModules(&settings.Config{}, nil, t.TempDir(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	active := routeCommandNames(modules.commands.Definitions())
	for module, names := range optionalModuleCommands {
		for _, name := range names {
			if active[name] {
				t.Errorf("zero-value config registered %s command /%s", module, name)
			}
		}
	}
	if modules.commands.HasPrivateQueries() {
		t.Fatal("zero-value config enabled private lookup admission")
	}
}

type lookupWarmRewriteTransport struct {
	target string
	base   http.RoundTripper
	hits   *atomic.Int32
}

func (t lookupWarmRewriteTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.hits.Add(1)
	rewritten := request.Clone(request.Context())
	rewritten.URL.Scheme = "http"
	rewritten.URL.Host = strings.TrimPrefix(t.target, "http://")
	return t.base.RoundTrip(rewritten)
}

func TestEnabledGentooRuntimeDoesNotWarmLookupCacheAtStartup(t *testing.T) {
	var requests atomic.Int32
	overlay := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tree":[]}`))
	}))
	t.Cleanup(overlay.Close)
	originalTransport := http.DefaultTransport
	http.DefaultTransport = lookupWarmRewriteTransport{
		target: overlay.URL, base: originalTransport, hits: &requests,
	}
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	cfg := &settings.Config{
		Modules:  []string{settings.ModuleGentoo},
		Overlays: []settings.OverlayCfg{{Name: settings.ModuleGentoo, Repo: "gentoo/gentoo", Branch: "master"}},
	}
	lookups := lookup.New(nil, nil, cfg, "")
	modules, err := newRuntimeModules(cfg, nil, t.TempDir(), nil, nil, lookups, false)
	if err != nil {
		t.Fatal(err)
	}
	if done := modules.Start(context.Background()); done != nil {
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for requests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("Gentoo startup issued %d lookup-cache requests before any group or command enabled it", got)
	}
}
func TestGroupCommandMenusDefaultToNoLookupCapabilities(t *testing.T) {
	const (
		groupA int64 = -1009000000611
		groupB int64 = -1009000000612
	)
	cfg := &settings.Config{
		Groups:   []settings.GroupConfig{{ID: groupA}, {ID: groupB}},
		GroupIDs: []int64{groupA, groupB},
	}
	store, err := settings.NewStore("", botTestSettingsBaseline(t, cfg), nil)
	if err != nil {
		t.Fatal(err)
	}
	modules, err := newRuntimeModules(cfg, nil, t.TempDir(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	caller := &dispatchCaller{members: make(map[[2]int64]telego.ChatMember)}
	updates := telegram.NewUpdates(cfg, store, nil, telegram.HandlerSet{Commands: modules.commands})
	updates.SetupCommands(context.Background(), testBot(t, caller))
	observed := make(map[struct {
		group int64
		admin bool
	}]bool)

	for _, call := range caller.snapshotCalls() {
		if call.method != "setMyCommands" {
			continue
		}
		var request struct {
			Commands []telego.BotCommand `json:"commands"`
			Scope    struct {
				Type   string `json:"type"`
				ChatID int64  `json:"chat_id"`
			} `json:"scope"`
		}
		if err := json.Unmarshal(call.body, &request); err != nil {
			t.Fatal(err)
		}
		if request.Scope.Type != "chat" && request.Scope.Type != "chat_administrators" {
			continue
		}
		if request.Scope.ChatID != groupA && request.Scope.ChatID != groupB {
			continue
		}
		for module, names := range optionalModuleCommands {
			for _, name := range names {
				if commandNames(request.Commands)[name] {
					t.Errorf("group %d default menu exposed %s command /%s", request.Scope.ChatID, module, name)
				}
			}
		}
		observed[struct {
			group int64
			admin bool
		}{group: request.Scope.ChatID, admin: request.Scope.Type == "chat_administrators"}] = true
	}
	if len(observed) != 4 {
		t.Fatalf("observed %d group command scopes, want both member/admin menus for both groups", len(observed))
	}
}

func TestEmptyModulesDoNotReachTelegramMenus(t *testing.T) {
	cfg := &settings.Config{Modules: []string{}}
	modules, err := newRuntimeModules(cfg, nil, t.TempDir(), nil, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	caller := &dispatchCaller{members: make(map[[2]int64]telego.ChatMember)}
	updates := telegram.NewUpdates(cfg, nil, nil, telegram.HandlerSet{Commands: modules.commands})
	updates.SetupCommands(context.Background(), testBot(t, caller))
	menuCalls := 0
	for _, call := range caller.snapshotCalls() {
		if call.method != "setMyCommands" {
			continue
		}
		menuCalls++
		var request struct {
			Commands []telego.BotCommand `json:"commands"`
		}
		if err := json.Unmarshal(call.body, &request); err != nil {
			t.Fatal(err)
		}
		menu := commandNames(request.Commands)
		for module, names := range optionalModuleCommands {
			for _, name := range names {
				if menu[name] {
					t.Errorf("disabled %s command /%s remains in a Telegram menu", module, name)
				}
			}
		}
	}
	if menuCalls == 0 {
		t.Fatal("disabled-module command surface did not set any Telegram menus")
	}
}

func TestRuntimeModuleSelectionMatchesConfiguration(t *testing.T) {
	for _, disabled := range settings.OptionalModuleNames() {
		t.Run(disabled, func(t *testing.T) {
			var modules []string
			if disabled == settings.ModuleGentoo {
				modules = []string{settings.ModuleLinux}
			} else {
				modules = []string{settings.ModuleGentoo}
			}
			runtimeModules, err := newRuntimeModules(&settings.Config{Modules: modules},
				nil, t.TempDir(), nil, nil, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			active := routeCommandNames(runtimeModules.commands.Definitions())
			for module, names := range optionalModuleCommands {
				for _, name := range names {
					if active[name] != (module != disabled) {
						t.Errorf("command /%s active = %t, want %t", name, active[name], module != disabled)
					}
				}
			}
		})
	}
}
func TestRuntimeOwnerConsoleSurfaceMatchesAvailability(t *testing.T) {
	for _, tc := range []struct {
		name             string
		consoleAvailable bool
		wantConsole      bool
	}{
		{name: "console configured", consoleAvailable: true, wantConsole: true},
		{name: "console disabled", consoleAvailable: false, wantConsole: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modules, err := newRuntimeModules(
				&settings.Config{Modules: []string{settings.ModuleGentoo, settings.ModuleLinux}},
				nil, t.TempDir(), nil, nil, nil, tc.consoleAvailable,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got := commandNames(modules.commands.OwnerMenu(i18n.LangEN))["console"]; got != tc.wantConsole {
				t.Errorf("owner menu includes /console = %t, want %t", got, tc.wantConsole)
			}
			if got := strings.Contains(modules.commands.OwnerHelp(i18n.LangEN), "/console"); got != tc.wantConsole {
				t.Errorf("owner help includes /console = %t, want %t", got, tc.wantConsole)
			}
		})
	}
}

func commandNames(commands []telego.BotCommand) map[string]bool {
	names := make(map[string]bool, len(commands))
	for _, command := range commands {
		names[command.Command] = true
	}
	return names
}

func routeCommandNames(commands []telegram.CommandDefinition) map[string]bool {
	names := make(map[string]bool, len(commands))
	for _, command := range commands {
		if !command.External {
			names[command.Name] = true
		}
	}
	return names
}
