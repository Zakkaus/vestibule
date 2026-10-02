package panel

import (
	"net/url"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

// SetConsoleURL supplies the Mini App address before polling and reports whether it is usable.
func (v *Panel) SetConsoleURL(rawURL string) bool {
	v.consoleURL = nil
	base, err := url.Parse(rawURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		return false
	}
	base.RawQuery, base.Fragment, base.RawFragment = "", "", ""
	v.consoleURL = base
	return true
}

func (v *Panel) settingsStartKeyboard(keyboard *telego.InlineKeyboardMarkup, session *panelSession) *telego.InlineKeyboardMarkup {
	if v.consoleURL == nil || session.screen != "gl" || session.chatID != session.ownerID {
		return keyboard
	}
	entry := *v.consoleURL
	entry.Path = "/groups"
	entry.RawPath = ""
	entry.User = nil
	entry.RawQuery = url.Values{"group": {strconv.FormatInt(session.anchorGroupID, 10)}}.Encode()
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []telego.InlineKeyboardButton{{
		Text:   i18n.Messages.Bot.Menu.Owner.ConsoleOpen.For(session.language),
		WebApp: &telego.WebAppInfo{URL: entry.String()},
	}})
	return keyboard
}
