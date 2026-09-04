package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
)

var optionalModuleCommands = map[string][]string{
	settings.ModuleGentoo: {"pkg", "use", "bug", "news", "arm"},
	settings.ModuleLinux:  {"wiki", "bbs", "pkgs", "distro", "armpkgs", "kernel", "man", "cve", "repology"},
}

func TestDisabledModulesDisappearFromCommandSurface(t *testing.T) {
	cfg := &settings.Config{DisabledModules: settings.OptionalModuleNames()}
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

func TestDisabledModulesDoNotReachTelegramMenus(t *testing.T) {
	cfg := &settings.Config{DisabledModules: settings.OptionalModuleNames()}
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
			modules, err := newRuntimeModules(&settings.Config{
				DisabledModules: []string{disabled},
			}, nil, t.TempDir(), nil, nil, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			active := routeCommandNames(modules.commands.Definitions())
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
				&settings.Config{DisabledModules: settings.OptionalModuleNames()},
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
