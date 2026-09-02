package telegram

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func testCommandHandler(_ *th.Context, _ telego.Update) error { return nil }

func testCommandModules(t *testing.T) CommandModules {
	t.Helper()
	member := i18n.Messages.Bot.Menu.Member
	admin := i18n.Messages.Bot.Menu.Admin
	modules, err := NewCommandModules(CommandModule{
		Name:           "test",
		PrivateQueries: true,
		Commands: []CommandDefinition{
			{Name: "help", Description: member.Help.For, Audience: CommandMember, RouteName: "panel.help", Handler: testCommandHandler},
			{Name: "pkg", Description: member.Pkg.For, Audience: CommandMember, RouteName: "lookup.pkg", Handler: testCommandHandler},
			{Name: "use", Description: member.Use.For, Audience: CommandMember, RouteName: "lookup.use", Handler: testCommandHandler},
			{Name: "bug", Description: member.Bug.For, Audience: CommandMember, RouteName: "lookup.bug", Handler: testCommandHandler},
			{Name: "news", Description: member.News.For, Audience: CommandMember, RouteName: "lookup.news", Handler: testCommandHandler},
			{Name: "wiki", Description: member.Wiki.For, Audience: CommandMember, RouteName: "lookup.wiki", Handler: testCommandHandler},
			{Name: "bbs", Description: member.BBS.For, Audience: CommandMember, RouteName: "lookup.bbs", Handler: testCommandHandler},
			{Name: "pkgs", Description: member.Pkgs.For, Audience: CommandMember, RouteName: "lookup.pkgs", Handler: testCommandHandler},
			{Name: "distro", Description: member.Distro.For, Audience: CommandMember, RouteName: "lookup.distro", Handler: testCommandHandler},
			{Name: "arm", Description: member.Arm.For, Audience: CommandMember, RouteName: "lookup.arm", Handler: testCommandHandler},
			{Name: "armpkgs", Description: member.ArmPkgs.For, Audience: CommandMember, RouteName: "lookup.armpkgs", Handler: testCommandHandler},
			{Name: "kernel", Description: member.Kernel.For, Audience: CommandMember, RouteName: "lookup.kernel", Handler: testCommandHandler},
			{Name: "man", Description: member.Man.For, Audience: CommandMember, RouteName: "lookup.man", Handler: testCommandHandler},
			{Name: "cve", Description: member.CVE.For, Audience: CommandMember, RouteName: "lookup.cve", Handler: testCommandHandler},
			{Name: "repology", Description: member.Repology.For, Audience: CommandMember, RouteName: "lookup.repology", Handler: testCommandHandler},
			{Name: "ping", Description: member.Ping.For, Audience: CommandMember, RouteName: "panel.ping", Handler: testCommandHandler},
			{Name: "stats", Description: member.Stats.For, Audience: CommandMember, RouteName: "panel.stats", Handler: testCommandHandler},
			{Name: "start", Description: admin.Start.For, Audience: CommandAdministrator, RouteName: "panel.start", Handler: testCommandHandler},
			{Name: "settings", Description: i18n.Messages.Panel.Menu.Settings.For, Audience: CommandAdministrator, RouteName: "panel.settings", Handler: testCommandHandler},
			{Name: "stop", Description: admin.Stop.For, Audience: CommandAdministrator, RouteName: "panel.stop", Handler: testCommandHandler},
			{Name: "mute", Description: admin.Mute.For, Audience: CommandAdministrator, RouteName: "moderate.mute", Handler: testCommandHandler},
			{Name: "unmute", Description: admin.Unmute.For, Audience: CommandAdministrator, RouteName: "moderate.unmute", Handler: testCommandHandler},
			{Name: "sb", Description: admin.Purge.For, Audience: CommandAdministrator, RouteName: "moderate.sb", Handler: testCommandHandler},
			{Name: "ban", Description: admin.Ban.For, Audience: CommandAdministrator, RouteName: "moderate.ban", Handler: testCommandHandler},
			{Name: "warn", Description: func(l i18n.Lang) string { return admin.Warn.Render(l, 3) }, Audience: CommandAdministrator, RouteName: "moderate.warn", Handler: testCommandHandler},
			{Name: "clearwarn", Description: admin.ClearWarn.For, Audience: CommandAdministrator, RouteName: "moderate.clearwarn", Handler: testCommandHandler},
			{Name: "bc", Description: admin.Channel.For, Audience: CommandAdministrator, RouteName: "moderate.bc", Handler: testCommandHandler},
			{Name: "rich", Description: admin.RichText.For, Audience: CommandAdministrator, RouteName: "panel.rich", Handler: testCommandHandler},
			{Name: "spoiler", Description: admin.NameSpoiler.For, Audience: CommandAdministrator, RouteName: "panel.spoiler", Handler: testCommandHandler},
			{Name: "vmode", Description: admin.VerificationMode.For, Audience: CommandAdministrator, RouteName: "panel.vmode", Handler: testCommandHandler},
			{Name: "autodel", Description: admin.AutoDelete.For, Audience: CommandAdministrator, RouteName: "panel.autodel", Handler: testCommandHandler},
			{Name: "bantime", Description: admin.BanTime.For, Audience: CommandAdministrator, RouteName: "panel.bantime", Handler: testCommandHandler},
			{Name: "enroll", Description: i18n.Messages.Bot.Menu.Owner.Enroll.For, Audience: CommandOwner, External: true},
			{Name: "unregister", Description: i18n.Messages.Bot.Menu.Owner.Unregister.For, Audience: CommandOwner, External: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return modules
}

func TestCommandModulesRejectMissingRouteTarget(t *testing.T) {
	_, err := NewCommandModules(CommandModule{
		Name: "broken",
		Commands: []CommandDefinition{{
			Name: "missing", Description: func(i18n.Lang) string { return "missing" },
			Audience: CommandMember, RouteName: "lookup.missing",
		}},
	})
	if err == nil {
		t.Fatal("module command with no handler was accepted")
	}
}

func TestCommandModulesRejectZeroCoverage(t *testing.T) {
	_, err := NewCommandModules(CommandModule{Name: "empty"})
	if err == nil {
		t.Fatal("empty command module was accepted")
	}
}
