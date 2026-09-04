package telegram

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

var (
	helpLocales    = []i18n.Lang{i18n.LangEN, i18n.LangZH, i18n.LangZHHant}
	helpCommandRe  = regexp.MustCompile(`/([a-z]+)`)
	helpLocaleName = map[i18n.Lang]string{i18n.LangEN: "en", i18n.LangZH: "zh", i18n.LangZHHant: "zh-Hant"}
)

func namesOf(cmds []telego.BotCommand) map[string]bool {
	out := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		out[c.Command] = true
	}
	return out
}

// A command reaches users through two independent surfaces: the Telegram menu and the /help
// text. Nothing links them, so adding a command and forgetting the help text is silent. These
// tests are that link.
func TestMemberHelpListsEveryMenuCommand(t *testing.T) {
	modules := testCommandModules(t)
	for _, l := range helpLocales {
		help := modules.MemberHelp(l)
		for _, c := range modules.MemberMenu(l) {
			if !regexp.MustCompile(`/` + c.Command + `\b`).MatchString(help) {
				t.Errorf("%s: /%s is in the member menu but not in the /help text", helpLocaleName[l], c.Command)
			}
		}
	}
}

func TestMemberHelpMentionsNoUnknownCommand(t *testing.T) {
	modules := testCommandModules(t)
	for _, l := range helpLocales {
		known := namesOf(modules.MemberMenu(l))
		for _, m := range helpCommandRe.FindAllStringSubmatch(modules.MemberHelp(l), -1) {
			if !known[m[1]] {
				t.Errorf("%s: /help lists /%s, which the bot does not register", helpLocaleName[l], m[1])
			}
		}
	}
}

func TestAdminHelpMatchesAdminMenu(t *testing.T) {
	modules := testCommandModules(t)
	for _, l := range helpLocales {
		help := modules.AdministratorHelp(l, 3)
		member := namesOf(modules.MemberMenu(l))
		known := namesOf(modules.AdministratorMenu(l))
		for _, c := range modules.AdministratorMenu(l) {
			if member[c.Command] {
				continue // member commands are documented by the member help
			}
			if !regexp.MustCompile(`/` + c.Command + `\b`).MatchString(help) {
				t.Errorf("%s: /%s is in the administrator menu but not in the administrator help", helpLocaleName[l], c.Command)
			}
		}
		for _, m := range helpCommandRe.FindAllStringSubmatch(help, -1) {
			if !known[m[1]] {
				t.Errorf("%s: administrator help lists /%s, which the bot does not register", helpLocaleName[l], m[1])
			}
		}
	}
}

func TestOwnerHelpMatchesOwnerMenu(t *testing.T) {
	modules := testCommandModules(t)
	for _, l := range helpLocales {
		help := modules.OwnerHelp(l)
		known := modules.commandNames(CommandOwner)
		for command := range known {
			if !regexp.MustCompile(`/` + command + `\b`).MatchString(help) {
				t.Errorf("%s: /%s is in the owner menu but not in the owner help", helpLocaleName[l], command)
			}
		}
		for _, m := range helpCommandRe.FindAllStringSubmatch(help, -1) {
			if !known[m[1]] {
				t.Errorf("%s: owner help lists /%s, which the bot does not register", helpLocaleName[l], m[1])
			}
		}
	}
}

// The auto-reply used to repeat the command list, which is how it went stale. It must now
// point at /help and name no lookup command of its own.
func TestAutoReplyNamesNoLookupCommand(t *testing.T) {
	for _, l := range helpLocales {
		reply := i18n.Messages.Bot.DirectMessage.AutoReply.Render(l, 5, i18n.Messages.Bot.DirectMessage.Identity.For(l))
		if !strings.Contains(reply, "/help") {
			t.Errorf("%s: the direct-message reply must point at /help", helpLocaleName[l])
		}
		for _, m := range helpCommandRe.FindAllStringSubmatch(reply, -1) {
			if m[1] != "help" {
				t.Errorf("%s: the direct-message reply names /%s; list commands in /help only", helpLocaleName[l], m[1])
			}
		}
	}
}

// Member commands and the direct-message allow-list are derived from one declaration.
func TestDMAllowsEveryMemberCommand(t *testing.T) {
	modules := testCommandModules(t)
	predicate := privateNonStart(modules.MemberCommandNames())
	for _, c := range modules.MemberMenu(i18n.LangEN) {
		update := telego.Update{Message: &telego.Message{
			Chat: telego.Chat{Type: "private"},
			Text: "/" + c.Command + " something",
		}}
		if predicate(context.Background(), update) {
			t.Errorf("/%s does not reach its handler in a direct message", c.Command)
		}
	}
}

func TestDMRejectsUnregisteredCommand(t *testing.T) {
	predicate := privateNonStart(testCommandModules(t).MemberCommandNames())
	for _, cmd := range []string{"/nosuchcommand", "/mute", "/settings"} {
		update := telego.Update{Message: &telego.Message{
			Chat: telego.Chat{Type: "private"},
			Text: cmd,
		}}
		if !predicate(context.Background(), update) {
			t.Errorf("%s should fall through to the direct-message reply", cmd)
		}
	}
}

// A multi-tenant product describes its configured groups without claiming one community.
func TestProductCopyClaimsNoCommunity(t *testing.T) {
	for _, l := range helpLocales {
		dm := i18n.Messages.Bot.DirectMessage
		reply := dm.AutoReply.Render(l, 5, dm.Identity.For(l))
		for _, claim := range []string{"Gentoo-zh", "Gentoo 中文社区", "Gentoo 中文社群", "gentoozh"} {
			if strings.Contains(reply, claim) {
				t.Errorf("%s: direct-message reply claims %q: %q", helpLocaleName[l], claim, reply)
			}
		}
	}
}
