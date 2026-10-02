package panel

import (
	"net/url"
	"path"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

// SetConsoleURL supplies the Mini App address before update polling starts.
func (v *Panel) SetConsoleURL(rawURL string) {
	v.consoleURL = nil
	base, err := url.Parse(rawURL)
	if err != nil || base.Scheme != "https" || base.Hostname() == "" {
		return
	}
	base.RawQuery, base.Fragment, base.RawFragment = "", "", ""
	v.consoleURL = base
}

func (v *Panel) settingsStartKeyboard(keyboard *telego.InlineKeyboardMarkup, session *panelSession) *telego.InlineKeyboardMarkup {
	if v.consoleURL == nil {
		return keyboard
	}
	entry := *v.consoleURL
	entry.Path = path.Join("/", entry.Path, "groups")
	entry.RawPath = ""
	entry.RawQuery = url.Values{"group": {strconv.FormatInt(session.anchorGroupID, 10)}}.Encode()
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []telego.InlineKeyboardButton{{
		Text:   i18n.Messages.Bot.Menu.Owner.ConsoleOpen.For(session.language),
		WebApp: &telego.WebAppInfo{URL: entry.String()},
	}})
	return keyboard
}
