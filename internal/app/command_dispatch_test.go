package app

import (
	"testing"

	"github.com/mymmrac/telego"
)

// Every advertised canonical command reaches exactly one handler.
func TestCanonicalCommandsReachTheirHandlers(t *testing.T) {
	fixture := newDispatchFixture(t, 0)
	for _, command := range []string{
		"/pkg", "/use", "/bug", "/news", "/bbs", "/arm",
		"/pkgs", "/distro", "/armpkgs", "/wiki", "/kernel",
	} {
		t.Run(command, func(t *testing.T) {
			got := dispatchRouteNames(t, fixture, telego.Update{Message: &telego.Message{
				Chat: telego.Chat{ID: fixture.groupID, Type: telego.ChatTypeSupergroup},
				From: &telego.User{ID: 801},
				Text: command + " portage",
			}})
			if len(got) != 1 {
				t.Fatalf("%s reached %v, want exactly one handler", command, got)
			}
		})
	}
}
