package telegram

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// ConsoleLinkIssuer keeps the Telegram adapter independent of credential storage.
type ConsoleLinkIssuer interface {
	IssueOperatorLink(int64) (string, time.Time, error)
}

// NewConsoleLinkHandler makes /console send the configured owner a one-use browser link.
// An empty URL intentionally disables delivery while keeping the API available to Mini App users.
func NewConsoleLinkHandler(bot *telego.Bot, issuer ConsoleLinkIssuer, rawURL string) (th.Handler, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, nil
	}
	base, err := consoleBaseURL(rawURL)
	if err != nil {
		return nil, err
	}
	if bot == nil || issuer == nil {
		return nil, errors.New("console link handler requires bot and issuer")
	}
	return func(ctx *th.Context, update telego.Update) error {
		message := update.Message
		if message == nil || message.From == nil || message.Chat.Type != telego.ChatTypePrivate {
			return nil
		}
		token, _, err := issuer.IssueOperatorLink(message.From.ID)
		if errors.Is(err, auth.ErrOperatorNotAllowed) {
			// Silence was the old answer, and it is indistinguishable from a bot
			// that is down, a command that does not exist, and a message that
			// never arrived. The one person who needs this command is the one who
			// just deployed the instance and has no other way in.
			denied := i18n.Messages.Bot.Menu.Owner.ConsoleDenied.For(i18n.FromTelegram(message.From.LanguageCode))
			if _, sendErr := bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID), denied)); sendErr != nil {
				return fmt.Errorf("send console refusal: %w", sendErr)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("issue console link: %w", err)
		}
		language := i18n.FromTelegram(message.From.LanguageCode)
		if _, err := bot.SendMessage(ctx.Context(), consoleLinkMessage(message.Chat.ID, base, token, language)); err != nil {
			return fmt.Errorf("send console link: %w", err)
		}
		return nil
	}, nil
}

// consoleLinkMessage puts the address behind a button. The link carries a
// one-time credential in its path, and a credential pasted as message text is a
// credential sitting in the chat: selectable, forwardable, and shown in every
// preview of that conversation. A loopback console cannot be reached from a
// phone at all, so there the address is the text, next to the reason it is.
func consoleLinkMessage(chatID int64, base *url.URL, token string, language i18n.Lang) *telego.SendMessageParams {
	entry := consoleEntryURL(base, token)
	catalog := i18n.Messages.Bot.Menu.Owner
	if isLoopbackHost(base.Hostname()) {
		return tu.Message(tu.ID(chatID), catalog.ConsoleLocal.For(language)+"\n\n"+entry)
	}
	return tu.Message(tu.ID(chatID), catalog.ConsoleSent.For(language)).
		WithReplyMarkup(&telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{{{
			Text: catalog.ConsoleOpen.For(language), URL: entry,
		}}}})
}

// consoleBaseURL accepts plain HTTP only on the loopback interface. A console
// bound to localhost is a supported deployment -- it is what an operator gets
// without a domain, reached over a port forward -- and requiring HTTPS there
// meant the instance refused to activate at all rather than admit it has no
// certificate for a host that cannot have one.
func consoleBaseURL(rawURL string) (*url.URL, error) {
	base, err := url.Parse(rawURL)
	if err != nil || base.Host == "" {
		return nil, errors.New("CONSOLE_URL must be an absolute URL")
	}
	if base.Scheme != "https" && !(base.Scheme == "http" && isLoopbackHost(base.Hostname())) {
		return nil, errors.New("CONSOLE_URL must be an absolute HTTPS URL, or HTTP on the loopback interface")
	}
	base.RawQuery, base.Fragment = "", ""
	return base, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func consoleEntryURL(base *url.URL, token string) string {
	entry := *base
	entry.Path = "/" + path.Join(base.Path, "enter", token)
	entry.RawPath = ""
	return entry.String()
}
