package telegram

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func testCommandHandler(_ *th.Context, _ telego.Update) error { return nil }

func commandDefinitionForTest(name, routeName string) CommandDefinition {
	return CommandDefinition{
		Name:        name,
		Description: func(i18n.Lang) string { return name },
		Audience:    CommandMember,
		RouteName:   routeName,
		Handler:     testCommandHandler,
	}
}

func commandModuleForTest(name string, privateQueries bool, commands ...CommandDefinition) CommandModule {
	return CommandModule{Name: name, PrivateQueries: privateQueries, Commands: commands}
}

func validCommandModules() []CommandModule {
	return []CommandModule{
		commandModuleForTest("core", false, commandDefinitionForTest("member", "route.member")),
	}
}

func assertCommandModulesReject(t *testing.T, harm string, modules ...CommandModule) {
	t.Helper()
	if _, err := NewCommandModules(validCommandModules()...); err != nil {
		t.Fatalf("positive command declaration was rejected: %v", err)
	}
	if _, err := NewCommandModules(modules...); err == nil {
		t.Fatal(harm)
	}
}

func testCommandModules(t *testing.T) CommandModules {
	t.Helper()
	member := i18n.Messages.Bot.Menu.Member
	admin := i18n.Messages.Bot.Menu.Admin
	owner := i18n.Messages.Bot.Menu.Owner
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
			{Name: "console", Description: owner.Console.For, Audience: CommandOwner, External: true},
			{Name: "enroll", Description: owner.Enroll.For, Audience: CommandOwner, External: true},
			{Name: "unregister", Description: owner.Unregister.For, Audience: CommandOwner, External: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return modules
}

func TestCommandModulesRejectInvalidModuleDeclarations(t *testing.T) {
	t.Run("a bot needs a command module", func(t *testing.T) {
		assertCommandModulesReject(t,
			"the bot accepted no command modules, so it can start without a command surface")
	})
	t.Run("a module identifies its command surface", func(t *testing.T) {
		assertCommandModulesReject(t,
			"an unnamed command module was accepted, so a configuration fault has no owner",
			commandModuleForTest("", false, commandDefinitionForTest("member", "route.member")))
	})
	t.Run("module names do not collide", func(t *testing.T) {
		assertCommandModulesReject(t,
			"duplicate command modules were accepted, so command ownership is ambiguous",
			commandModuleForTest("core", false, commandDefinitionForTest("member", "route.member")),
			commandModuleForTest("core", false, commandDefinitionForTest("other", "route.other")))
	})
	t.Run("an enabled module contributes commands", func(t *testing.T) {
		assertCommandModulesReject(t,
			"an empty command module was accepted, so an enabled feature can silently disappear",
			commandModuleForTest("empty", false))
	})
}

func TestCommandModulesRejectInvalidCommandDeclarations(t *testing.T) {
	t.Run("a command has a name", func(t *testing.T) {
		assertCommandModulesReject(t,
			"a nameless command was accepted, so Telegram can receive a malformed menu entry",
			commandModuleForTest("core", false, commandDefinitionForTest("", "route.member")))
	})
	t.Run("command names are unique", func(t *testing.T) {
		assertCommandModulesReject(t,
			"duplicate command names were accepted, so two handlers can claim the same update",
			commandModuleForTest("core", false,
				commandDefinitionForTest("member", "route.member"),
				commandDefinitionForTest("member", "route.other")))
	})
	t.Run("a menu command explains itself", func(t *testing.T) {
		command := commandDefinitionForTest("member", "route.member")
		command.Description = nil
		assertCommandModulesReject(t,
			"a command without a description was accepted, so users cannot understand the menu entry",
			commandModuleForTest("core", false, command))
	})
	t.Run("a command has a known audience", func(t *testing.T) {
		command := commandDefinitionForTest("member", "route.member")
		command.Audience = CommandOwner + 1
		assertCommandModulesReject(t,
			"an unknown command audience was accepted, so permission placement is undefined",
			commandModuleForTest("core", false, command))
	})
	t.Run("an external command cannot claim a route", func(t *testing.T) {
		command := commandDefinitionForTest("enroll", "route.enroll")
		command.External = true
		command.Handler = nil
		assertCommandModulesReject(t,
			"an external command claimed an update route, so it can conflict with its separate dispatch",
			commandModuleForTest("owner", false, command))
	})
	t.Run("an external command cannot install a handler", func(t *testing.T) {
		command := commandDefinitionForTest("enroll", "")
		command.External = true
		assertCommandModulesReject(t,
			"an external command installed a handler, so it can bypass its separate dispatch",
			commandModuleForTest("owner", false, command))
	})
	t.Run("a routed command names its target", func(t *testing.T) {
		assertCommandModulesReject(t,
			"a visible command had no route target, so its update cannot be dispatched",
			commandModuleForTest("core", false, commandDefinitionForTest("member", "")))
	})
	t.Run("a routed command has a handler", func(t *testing.T) {
		command := commandDefinitionForTest("member", "route.member")
		command.Handler = nil
		assertCommandModulesReject(t,
			"a visible command had no handler, so its update would be dropped",
			commandModuleForTest("core", false, command))
	})
	t.Run("route names are unique", func(t *testing.T) {
		assertCommandModulesReject(t,
			"duplicate route names were accepted, so command dispatch cannot be identified uniquely",
			commandModuleForTest("core", false,
				commandDefinitionForTest("first", "route.shared"),
				commandDefinitionForTest("second", "route.shared")))
	})
}

func TestCommandModuleDefinitionsCannotMutateRegisteredSurface(t *testing.T) {
	modules, err := NewCommandModules(validCommandModules()...)
	if err != nil {
		t.Fatal(err)
	}
	definitions := modules.Definitions()
	if len(definitions) != 1 {
		t.Fatalf("definition count = %d, want one", len(definitions))
	}
	definitions[0].Name = "renamed"
	definitions[0].RouteName = "route.renamed"
	if got := modules.Definitions()[0].Name; got != "member" {
		t.Fatalf("mutating a definitions copy renamed registered command %q", got)
	}
	if got := modules.Routes()[0].Command; got != "member" {
		t.Fatalf("mutating a definitions copy changed dispatched command %q", got)
	}
}

func TestPrivateQueryCapabilityIncludesEveryEnabledModule(t *testing.T) {
	withoutQueries, err := NewCommandModules(
		commandModuleForTest("core", false, commandDefinitionForTest("member", "route.member")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if withoutQueries.HasPrivateQueries() {
		t.Fatal("a command surface without private-query modules accepts private queries")
	}
	withQueries, err := NewCommandModules(
		commandModuleForTest("core", false, commandDefinitionForTest("member", "route.member")),
		commandModuleForTest("lookup", true, commandDefinitionForTest("lookup", "route.lookup")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !withQueries.HasPrivateQueries() {
		t.Fatal("an enabled private-query module did not enable private query handling")
	}
}

func TestExternalMenuCommandsAreNotDynamicDispatchRoutes(t *testing.T) {
	external := commandDefinitionForTest("enroll", "")
	external.Audience = CommandOwner
	external.External = true
	external.Handler = nil
	modules, err := NewCommandModules(commandModuleForTest("core", false,
		commandDefinitionForTest("member", "route.member"), external))
	if err != nil {
		t.Fatal(err)
	}
	routes := modules.Routes()
	if len(routes) != 1 {
		t.Fatalf("external menu command created %d dynamic routes, want one member route", len(routes))
	}
	if routes[0].Name != "route.member" || routes[0].Command != "member" || routes[0].Handler == nil {
		t.Fatalf("dynamic route = %#v, want the member command handler", routes[0])
	}
}

func TestOnlyMemberCommandsBypassDirectMessageReplies(t *testing.T) {
	owner := commandDefinitionForTest("owner", "")
	owner.Audience = CommandOwner
	owner.External = true
	owner.Handler = nil
	admin := commandDefinitionForTest("admin", "route.admin")
	admin.Audience = CommandAdministrator
	modules, err := NewCommandModules(commandModuleForTest("core", false,
		commandDefinitionForTest("member", "route.member"),
		admin,
		owner,
	))
	if err != nil {
		t.Fatal(err)
	}
	predicate := privateNonStart(modules.MemberCommandNames())
	for _, test := range []struct {
		command   string
		wantReply bool
		harm      string
	}{
		{command: "/member", wantReply: false, harm: "a member command did not reach its handler"},
		{command: "/admin", wantReply: true, harm: "an administrator command bypassed the direct-message safeguard"},
		{command: "/owner", wantReply: true, harm: "an owner command bypassed the direct-message safeguard"},
	} {
		t.Run(test.command, func(t *testing.T) {
			update := telego.Update{Message: &telego.Message{
				Chat: telego.Chat{Type: telego.ChatTypePrivate}, Text: test.command,
			}}
			if got := predicate(context.Background(), update); got != test.wantReply {
				t.Fatalf("%s reply route = %t, want %t: %s", test.command, got, test.wantReply, test.harm)
			}
		})
	}
}
