package panel

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
)

func TestAutoDelReportsEffectiveSeconds(t *testing.T) {
	for _, language := range i18n.Languages() {
		for _, test := range []struct {
			seconds int
			english string
		}{{45, "45 seconds"}, {60, "1 minute"}, {90, "1 minute 30 seconds"}, {3600, "1 hour"}, {86400, "1 day"}} {
			seconds := test.seconds
			t.Run(fmt.Sprintf("%s/%d", language, seconds), func(t *testing.T) {
				cfg := runtimeSettingsTestConfig()
				cfg.NotifyTTLSeconds = -1
				store, err := settings.NewStore("", testSettingsBaseline(t, cfg), nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				group, _ := store.Settings(cfg.GroupIDs[0])
				next := group.Overrides()
				tag := language.String()
				next.Lang, next.LookupTTLSeconds = &tag, &seconds
				if _, err := store.Update(group.ID(), group.Revision(), next); err != nil {
					t.Fatal(err)
				}
				fake := newFakeAdminBot()
				fake.member = &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator, CanRestrictMembers: true}
				bot := newAPITestBot(t, fake)
				panel, verifier := newAdminTestApplication(t, cfg, store, bot)
				t.Cleanup(verifier.Shutdown)
				panel.lookups = lookup.New(store, cfg, "")
				wantDuration := tgfmt.ModerationBanDurationText(language, seconds)
				if seconds == 90 {
					wantDuration = tgfmt.ModerationBanDurationText(language, 60) + " " + tgfmt.ModerationBanDurationText(language, 30)
				}
				if language == i18n.LangEN {
					wantDuration = test.english
				}
				for _, command := range []string{"/autodel", "/autodel off", "/autodel on"} {
					runFakeHandler(t, bot, panel.OnAutoDel, telego.Update{Message: &telego.Message{
						MessageID: 1, Chat: telego.Chat{ID: group.ID(), Type: telego.ChatTypeSupergroup},
						From: &telego.User{ID: 7, LanguageCode: "en"}, Text: command,
					}})
					if command == "/autodel off" {
						continue
					}
					t.Logf("%s duration=%s response=%s", command, wantDuration, fake.lastSendText)
					wantText := wantDuration
					if language == i18n.LangEN {
						wantText += "."
					}
					if !strings.Contains(fake.lastSendText, wantText) || strings.Contains(fake.lastSendText, "%!") {
						t.Errorf("%s response = %q, want effective duration %q", command, fake.lastSendText, wantText)
					}
					if duration, enabled := panel.lookups.AutoDelete(group.ID()); !enabled || duration != time.Duration(seconds)*time.Second {
						t.Errorf("%s cleanup = %s/%t, want %ds/enabled", command, duration, enabled, seconds)
					}
				}
			})
		}
	}
}
