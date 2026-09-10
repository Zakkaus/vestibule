package telegram

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// CommandAudience selects the Telegram command-menu audience for one command.
type CommandAudience uint8

const (
	// CommandMember appears in every member command menu.
	CommandMember CommandAudience = iota
	// CommandAdministrator appears only in administrator command menus.
	CommandAdministrator
	// CommandOwner appears only in the claimed owner's private menu.
	CommandOwner
)

// CommandDefinition declares one menu entry and, unless External is true, its update route.
type CommandDefinition struct {
	Name        string
	Description func(i18n.Lang) string
	Audience    CommandAudience
	RouteName   string
	Handler     th.Handler
	External    bool
}

// CommandModule declares a coherent optional or core command surface.
type CommandModule struct {
	Name           string
	PrivateQueries bool
	Commands       []CommandDefinition
}

// CommandRoute is one command handler derived from a CommandDefinition.
type CommandRoute struct {
	Name    string
	Command string
	Handler th.Handler
}

// CommandModules is the validated, active command surface for one bot instance.
type CommandModules struct {
	commands       []CommandDefinition
	privateQueries bool
}

// NewCommandModules validates and combines the declarations selected for one bot instance.
func NewCommandModules(modules ...CommandModule) (CommandModules, error) {
	if len(modules) == 0 {
		return CommandModules{}, fmt.Errorf("no command modules declared")
	}

	moduleNames := make(map[string]bool, len(modules))
	commandNames := make(map[string]bool)
	routeNames := make(map[string]bool)
	commands := make([]CommandDefinition, 0)
	privateQueries := false
	for _, module := range modules {
		if strings.TrimSpace(module.Name) == "" {
			return CommandModules{}, fmt.Errorf("command module has no name")
		}
		if moduleNames[module.Name] {
			return CommandModules{}, fmt.Errorf("duplicate command module %q", module.Name)
		}
		moduleNames[module.Name] = true
		if len(module.Commands) == 0 {
			return CommandModules{}, fmt.Errorf("command module %q has no command coverage", module.Name)
		}
		for _, command := range module.Commands {
			if err := validateCommandDefinition(module.Name, command, commandNames, routeNames); err != nil {
				return CommandModules{}, err
			}
			commands = append(commands, command)
		}
		privateQueries = privateQueries || module.PrivateQueries
	}
	return CommandModules{commands: commands, privateQueries: privateQueries}, nil
}

func validateCommandDefinition(
	module string,
	command CommandDefinition,
	commandNames, routeNames map[string]bool,
) error {
	if strings.TrimSpace(command.Name) == "" {
		return fmt.Errorf("command module %q has a command without a name", module)
	}
	if commandNames[command.Name] {
		return fmt.Errorf("duplicate command %q", command.Name)
	}
	if command.Description == nil {
		return fmt.Errorf("command %q has no menu description", command.Name)
	}
	if command.Audience > CommandOwner {
		return fmt.Errorf("command %q has unknown menu audience %d", command.Name, command.Audience)
	}
	if command.External {
		if command.RouteName != "" || command.Handler != nil {
			return fmt.Errorf("external command %q declares an update route", command.Name)
		}
	} else {
		if strings.TrimSpace(command.RouteName) == "" {
			return fmt.Errorf("command %q has no route target", command.Name)
		}
		if command.Handler == nil {
			return fmt.Errorf("command %q targets %q without a handler", command.Name, command.RouteName)
		}
		if routeNames[command.RouteName] {
			return fmt.Errorf("duplicate command route %q", command.RouteName)
		}
		routeNames[command.RouteName] = true
	}
	commandNames[command.Name] = true
	return nil
}

// HasCommands reports whether this is an initialized command surface.
func (m CommandModules) HasCommands() bool { return len(m.commands) != 0 }

// Definitions returns a copy of the active command declarations.
func (m CommandModules) Definitions() []CommandDefinition {
	return append([]CommandDefinition(nil), m.commands...)
}

// HasPrivateQueries reports whether an active module has rate-limited lookup commands in DMs.
func (m CommandModules) HasPrivateQueries() bool { return m.privateQueries }

// MemberMenu returns commands visible to ordinary members.
func (m CommandModules) MemberMenu(l i18n.Lang) []telego.BotCommand {
	return m.menu(l, CommandMember)
}

// AdministratorMenu returns administrator commands followed by ordinary member commands.
func (m CommandModules) AdministratorMenu(l i18n.Lang) []telego.BotCommand {
	return m.menu(l, CommandAdministrator, CommandMember)
}

// OwnerMenu returns owner commands followed by ordinary member commands.
func (m CommandModules) OwnerMenu(l i18n.Lang) []telego.BotCommand {
	return m.menu(l, CommandOwner, CommandMember)
}

func (m CommandModules) menu(l i18n.Lang, audiences ...CommandAudience) []telego.BotCommand {
	out := make([]telego.BotCommand, 0, len(m.commands))
	for _, audience := range audiences {
		for _, command := range m.commands {
			if command.Audience != audience {
				continue
			}
			out = append(out, telego.BotCommand{Command: command.Name, Description: command.Description(l)})
		}
	}
	return out
}

// Routes returns all update routes declared by active modules.
func (m CommandModules) Routes() []CommandRoute {
	routes := make([]CommandRoute, 0, len(m.commands))
	for _, command := range m.commands {
		if command.External {
			continue
		}
		routes = append(routes, CommandRoute{
			Name: command.RouteName, Command: command.Name, Handler: command.Handler,
		})
	}
	return routes
}

// MemberCommandNames returns a set suitable for direct-message command admission.
func (m CommandModules) MemberCommandNames() map[string]bool {
	return m.commandNames(CommandMember)
}

// MemberHelp returns the member help body without commands from disabled modules.
func (m CommandModules) MemberHelp(l i18n.Lang) string {
	return filterCommandHelp(i18n.Messages.Panel.Help.Member.For(l), m.commandNames(CommandMember))
}

// AdministratorHelp returns the administrator help body without commands from disabled modules.
func (m CommandModules) AdministratorHelp(l i18n.Lang, warnLimit int) string {
	return filterCommandHelp(
		i18n.Messages.Panel.Help.Admin.Render(l, warnLimit),
		m.commandNames(CommandAdministrator, CommandMember),
	)
}

// OwnerHelp returns the private bot-owner help body.
func (m CommandModules) OwnerHelp(l i18n.Lang) string {
	return filterCommandHelp(i18n.Messages.Panel.Help.Owner.For(l), m.commandNames(CommandOwner))
}

func (m CommandModules) commandNames(audiences ...CommandAudience) map[string]bool {
	names := make(map[string]bool, len(m.commands))
	for _, audience := range audiences {
		for _, command := range m.commands {
			if command.Audience == audience {
				names[command.Name] = true
			}
		}
	}
	return names
}

var helpCommandReference = regexp.MustCompile(`/([a-z]+)\b`)

func filterCommandHelp(help string, allowed map[string]bool) string {
	lines := strings.Split(help, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		keep := true
		for _, match := range helpCommandReference.FindAllStringSubmatch(line, -1) {
			if !allowed[match[1]] {
				keep = false
				break
			}
		}
		if !keep || (line == "" && (len(out) == 0 || out[len(out)-1] == "")) {
			continue
		}
		out = append(out, line)
	}
	for len(out) != 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
