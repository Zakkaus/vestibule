package app

import (
	"context"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
)

type runtimeModule struct {
	optionalName string
	commands     telegram.CommandModule
	start        func(context.Context) <-chan struct{}
}

type runtimeModules struct {
	commands telegram.CommandModules
	starters []func(context.Context) <-chan struct{}
}

func newRuntimeModules(
	cfg *settings.Config,
	bot *telego.Bot,
	stateDirectory string,
	administration *panel.Panel,
	moderation *moderate.Service,
	lookups *lookup.Service,
) (*runtimeModules, error) {
	if cfg == nil {
		return nil, fmt.Errorf("command modules require config")
	}
	declared := []runtimeModule{
		coreHelpModule(administration),
		gentooModule(cfg, bot, stateDirectory, lookups),
		linuxModule(lookups),
		coreStatusModule(administration),
		coreAdministrationModule(cfg, administration, moderation),
		coreOwnerModule(),
	}
	commandModules := make([]telegram.CommandModule, 0, len(declared))
	starters := make([]func(context.Context) <-chan struct{}, 0, 1)
	for _, module := range declared {
		if module.optionalName != "" && !cfg.ModuleEnabled(module.optionalName) {
			continue
		}
		commandModules = append(commandModules, module.commands)
		if module.start != nil {
			starters = append(starters, module.start)
		}
	}
	commands, err := telegram.NewCommandModules(commandModules...)
	if err != nil {
		return nil, fmt.Errorf("build command modules: %w", err)
	}
	return &runtimeModules{commands: commands, starters: starters}, nil
}

func (m *runtimeModules) Start(ctx context.Context) <-chan struct{} {
	if m == nil {
		return nil
	}
	done := make([]<-chan struct{}, 0, len(m.starters))
	for _, start := range m.starters {
		if componentDone := start(ctx); componentDone != nil {
			done = append(done, componentDone)
		}
	}
	switch len(done) {
	case 0:
		return nil
	case 1:
		return done[0]
	}
	allDone := make(chan struct{})
	go func() {
		for _, componentDone := range done {
			<-componentDone
		}
		close(allDone)
	}()
	return allDone
}

func coreHelpModule(administration *panel.Panel) runtimeModule {
	menu := i18n.Messages.Bot.Menu.Member
	return runtimeModule{commands: telegram.CommandModule{
		Name: "core-help",
		Commands: []telegram.CommandDefinition{{
			Name: "help", Description: menu.Help.For, Audience: telegram.CommandMember,
			RouteName: "panel.help", Handler: administration.OnHelp,
		}},
	}}
}

func gentooModule(
	cfg *settings.Config,
	bot *telego.Bot,
	stateDirectory string,
	lookups *lookup.Service,
) runtimeModule {
	menu := i18n.Messages.Bot.Menu.Member
	return runtimeModule{
		optionalName: settings.ModuleGentoo,
		commands: telegram.CommandModule{
			Name:           settings.ModuleGentoo,
			PrivateQueries: true,
			Commands: []telegram.CommandDefinition{
				{Name: "pkg", Description: menu.Pkg.For, Audience: telegram.CommandMember, RouteName: "lookup.pkg", Handler: lookups.OnPkg},
				{Name: "use", Description: menu.Use.For, Audience: telegram.CommandMember, RouteName: "lookup.use", Handler: lookups.OnUse},
				{Name: "bug", Description: menu.Bug.For, Audience: telegram.CommandMember, RouteName: "lookup.bug", Handler: lookups.OnBug},
				{Name: "news", Description: menu.News.For, Audience: telegram.CommandMember, RouteName: "lookup.news", Handler: lookups.OnNews},
				{Name: "arm", Description: menu.Arm.For, Audience: telegram.CommandMember, RouteName: "lookup.arm", Handler: lookups.OnArm},
			},
		},
		start: func(ctx context.Context) <-chan struct{} {
			done := startFeeds(ctx, cfg, bot, stateDirectory)
			go lookups.Warm(ctx)
			return done
		},
	}
}

func linuxModule(lookups *lookup.Service) runtimeModule {
	menu := i18n.Messages.Bot.Menu.Member
	return runtimeModule{
		optionalName: settings.ModuleLinux,
		commands: telegram.CommandModule{
			Name:           settings.ModuleLinux,
			PrivateQueries: true,
			Commands: []telegram.CommandDefinition{
				{Name: "wiki", Description: menu.Wiki.For, Audience: telegram.CommandMember, RouteName: "lookup.wiki", Handler: lookups.OnWiki},
				{Name: "bbs", Description: menu.BBS.For, Audience: telegram.CommandMember, RouteName: "lookup.bbs", Handler: lookups.OnBbs},
				{Name: "pkgs", Description: menu.Pkgs.For, Audience: telegram.CommandMember, RouteName: "lookup.pkgs", Handler: lookups.OnPkgs},
				{Name: "distro", Description: menu.Distro.For, Audience: telegram.CommandMember, RouteName: "lookup.distro", Handler: lookups.OnPkgs},
				{Name: "armpkgs", Description: menu.ArmPkgs.For, Audience: telegram.CommandMember, RouteName: "lookup.armpkgs", Handler: lookups.OnArmpkgs},
				{Name: "kernel", Description: menu.Kernel.For, Audience: telegram.CommandMember, RouteName: "lookup.kernel", Handler: lookups.OnKernel},
				{Name: "man", Description: menu.Man.For, Audience: telegram.CommandMember, RouteName: "lookup.man", Handler: lookups.OnMan},
				{Name: "cve", Description: menu.CVE.For, Audience: telegram.CommandMember, RouteName: "lookup.cve", Handler: lookups.OnCVE},
				{Name: "repology", Description: menu.Repology.For, Audience: telegram.CommandMember, RouteName: "lookup.repology", Handler: lookups.OnRepology},
			},
		},
	}
}

func coreStatusModule(administration *panel.Panel) runtimeModule {
	menu := i18n.Messages.Bot.Menu.Member
	return runtimeModule{commands: telegram.CommandModule{
		Name: "core-status",
		Commands: []telegram.CommandDefinition{
			{Name: "ping", Description: menu.Ping.For, Audience: telegram.CommandMember, RouteName: "panel.ping", Handler: administration.OnPing},
			{Name: "stats", Description: menu.Stats.For, Audience: telegram.CommandMember, RouteName: "panel.stats", Handler: administration.OnStats},
		},
	}}
}

func coreAdministrationModule(
	cfg *settings.Config,
	administration *panel.Panel,
	moderation *moderate.Service,
) runtimeModule {
	menu := i18n.Messages.Bot.Menu.Admin
	return runtimeModule{commands: telegram.CommandModule{
		Name: "core-administration",
		Commands: []telegram.CommandDefinition{
			{Name: "start", Description: menu.Start.For, Audience: telegram.CommandAdministrator, RouteName: "panel.start", Handler: administration.OnStart},
			{Name: "settings", Description: i18n.Messages.Panel.Menu.Settings.For, Audience: telegram.CommandAdministrator, RouteName: "panel.settings", Handler: administration.OnSettings},
			{Name: "stop", Description: menu.Stop.For, Audience: telegram.CommandAdministrator, RouteName: "panel.stop", Handler: administration.OnStop},
			{Name: "mute", Description: menu.Mute.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.mute", Handler: moderation.OnMute},
			{Name: "unmute", Description: menu.Unmute.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.unmute", Handler: moderation.OnUnmute},
			{Name: "sb", Description: menu.Purge.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.sb", Handler: moderation.OnPurge},
			{Name: "ban", Description: menu.Ban.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.ban", Handler: moderation.OnBan},
			{Name: "warn", Description: func(l i18n.Lang) string { return menu.Warn.Render(l, cfg.WarnLimit) }, Audience: telegram.CommandAdministrator, RouteName: "moderate.warn", Handler: moderation.OnWarn},
			{Name: "clearwarn", Description: menu.ClearWarn.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.clearwarn", Handler: moderation.OnClearWarn},
			{Name: "bc", Description: menu.Channel.For, Audience: telegram.CommandAdministrator, RouteName: "moderate.bc", Handler: telegram.NewBlockChannelHandler(moderation)},
			{Name: "rich", Description: menu.RichText.For, Audience: telegram.CommandAdministrator, RouteName: "panel.rich", Handler: administration.OnRich},
			{Name: "spoiler", Description: menu.NameSpoiler.For, Audience: telegram.CommandAdministrator, RouteName: "panel.spoiler", Handler: administration.OnSpoiler},
			{Name: "vmode", Description: menu.VerificationMode.For, Audience: telegram.CommandAdministrator, RouteName: "panel.vmode", Handler: administration.OnVMode},
			{Name: "autodel", Description: menu.AutoDelete.For, Audience: telegram.CommandAdministrator, RouteName: "panel.autodel", Handler: administration.OnAutoDel},
			{Name: "bantime", Description: menu.BanTime.For, Audience: telegram.CommandAdministrator, RouteName: "panel.bantime", Handler: administration.OnBanTime},
		},
	}}
}

func coreOwnerModule() runtimeModule {
	menu := i18n.Messages.Bot.Menu.Owner
	return runtimeModule{commands: telegram.CommandModule{
		Name: "core-owner",
		Commands: []telegram.CommandDefinition{
			{Name: "enroll", Description: menu.Enroll.For, Audience: telegram.CommandOwner, External: true},
			{Name: "unregister", Description: menu.Unregister.For, Audience: telegram.CommandOwner, External: true},
			// /console was implemented and listed nowhere: not in the menu, not in
			// help, and silent when it refused. The one person who needs it is the
			// one who just deployed the instance and has no other way in.
			{Name: "console", Description: menu.Console.For, Audience: telegram.CommandOwner, External: true},
		},
	}}
}
