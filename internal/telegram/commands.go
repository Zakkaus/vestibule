package telegram

import (
	"context"
	"fmt"
	"log"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

func (s *Updates) memberCommands(l i18n.Lang) []telego.BotCommand {
	return s.handlers.Commands.MemberMenu(l)
}

func (s *Updates) memberGroupCommands(groupID int64, l i18n.Lang) []telego.BotCommand {
	return s.handlers.Commands.MemberMenuFor(l, s.groupCapabilities(groupID))
}

func (s *Updates) ownerCommands(l i18n.Lang) []telego.BotCommand {
	return s.handlers.Commands.OwnerMenu(l)
}

func (s *Updates) adminCommands(l i18n.Lang) []telego.BotCommand {
	return s.handlers.Commands.AdministratorMenu(l)
}

func (s *Updates) adminGroupCommands(groupID int64, l i18n.Lang) []telego.BotCommand {
	return s.handlers.Commands.AdministratorMenuFor(l, s.groupCapabilities(groupID))
}

// SetupCommands registers member, administrator, and claimed-owner Telegram command menus.
func (s *Updates) SetupCommands(ctx context.Context, bot *telego.Bot) {
	type commandMenu struct {
		name         string
		commands     []telego.BotCommand
		scope        telego.BotCommandScope
		languageCode string
	}
	groupIDs := []int64(nil)
	ownerID := int64(0)
	if s.settings != nil {
		groupIDs = s.settings.ChatIDs()
		ownerID = s.settings.Registrations().OwnerID
	}
	menuCapacity := 10 + 2*len(groupIDs)
	if ownerID != 0 {
		menuCapacity += 5
	}
	menus := make([]commandMenu, 0, menuCapacity)
	for _, language := range []struct {
		name string
		lang i18n.Lang
		code string
	}{
		{name: "fallback", lang: i18n.LangZH},
		{name: "zh", lang: i18n.LangZH, code: "zh"},
		{name: "en", lang: i18n.LangEN, code: "en"},
		{name: "ja", lang: i18n.LangJA, code: "ja"},
		{name: "ru", lang: i18n.LangRU, code: "ru"},
	} {
		member := s.memberCommands(language.lang)
		admin := s.adminCommands(language.lang)
		menus = append(menus,
			commandMenu{name: "members/" + language.name, commands: member,
				scope: &telego.BotCommandScopeDefault{Type: "default"}, languageCode: language.code},
			commandMenu{name: "admins/" + language.name, commands: admin,
				scope: &telego.BotCommandScopeAllChatAdministrators{Type: "all_chat_administrators"}, languageCode: language.code},
		)
		if ownerID != 0 {
			menus = append(menus, commandMenu{
				name: "owner/" + language.name, commands: s.ownerCommands(language.lang),
				scope: &telego.BotCommandScopeChat{Type: "chat", ChatID: tu.ID(ownerID)}, languageCode: language.code,
			})
		}
	}
	for _, groupID := range groupIDs {
		language := s.groupLanguage(groupID)
		menus = append(menus,
			commandMenu{name: fmt.Sprintf("members/chat/%d", groupID), commands: s.memberGroupCommands(groupID, language),
				scope: &telego.BotCommandScopeChat{Type: "chat", ChatID: tu.ID(groupID)}},
			commandMenu{name: fmt.Sprintf("admins/chat/%d", groupID), commands: s.adminGroupCommands(groupID, language),
				scope: &telego.BotCommandScopeChatAdministrators{Type: "chat_administrators", ChatID: tu.ID(groupID)}},
		)
	}
	confirmedMenus := 0
	for _, menu := range menus {
		if err := bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
			Commands: menu.commands, Scope: menu.scope, LanguageCode: menu.languageCode,
		}); err != nil {
			log.Printf("setMyCommands(%s): %v", menu.name, err)
			continue
		}
		confirmedMenus++
	}
	log.Printf("bot command menus: confirmed=%d unconfirmed=%d", confirmedMenus, len(menus)-confirmedMenus)
}

func (s *Updates) groupLanguage(groupID int64) i18n.Lang {
	if s.settings != nil {
		if group, ok := s.settings.Settings(groupID); ok {
			return i18n.FromStored(group.Lang().Value)
		}
	}
	return i18n.FromStored(s.cfg.LangForGroup(groupID))
}

// RefreshGroupCommands replaces exactly the member and administrator menus for one group.
func (s *Updates) RefreshGroupCommands(ctx context.Context, bot *telego.Bot, groupID int64) {
	language := s.groupLanguage(groupID)
	menus := [...]struct {
		name     string
		commands []telego.BotCommand
		scope    telego.BotCommandScope
	}{
		{
			name:     fmt.Sprintf("members/chat/%d", groupID),
			commands: s.memberGroupCommands(groupID, language),
			scope:    &telego.BotCommandScopeChat{Type: "chat", ChatID: tu.ID(groupID)},
		},
		{
			name:     fmt.Sprintf("admins/chat/%d", groupID),
			commands: s.adminGroupCommands(groupID, language),
			scope:    &telego.BotCommandScopeChatAdministrators{Type: "chat_administrators", ChatID: tu.ID(groupID)},
		},
	}
	for _, menu := range menus {
		if err := bot.SetMyCommands(ctx, &telego.SetMyCommandsParams{
			Commands: menu.commands, Scope: menu.scope,
		}); err != nil {
			log.Printf("setMyCommands(%s): %v", menu.name, err)
		}
	}
}
