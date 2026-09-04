package telegram

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
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
			return nil
		}
		if err != nil {
			return fmt.Errorf("issue console link: %w", err)
		}
		if _, err := bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID), consoleEntryURL(base, token))); err != nil {
			return fmt.Errorf("send console link: %w", err)
		}
		return nil
	}, nil
}

func consoleBaseURL(rawURL string) (*url.URL, error) {
	base, err := url.Parse(rawURL)
	if err != nil || base.Host == "" {
		return nil, errors.New("CONSOLE_URL must be an absolute HTTPS URL or an HTTP URL on a loopback address")
	}
	schemeAllowed := base.Scheme == "https"
	if base.Scheme == "http" {
		address, addressErr := netip.ParseAddr(base.Hostname())
		schemeAllowed = addressErr == nil && address.IsLoopback()
	}
	if !schemeAllowed {
		return nil, errors.New("CONSOLE_URL must be an absolute HTTPS URL or an HTTP URL on a loopback address")
	}
	base.RawQuery, base.Fragment = "", ""
	return base, nil
}

func consoleEntryURL(base *url.URL, token string) string {
	entry := *base
	entry.Path = "/" + path.Join(base.Path, "enter", token)
	entry.RawPath = ""
	return entry.String()
}
