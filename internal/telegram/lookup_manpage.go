package telegram

import (
	"context"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// OnMan answers with a manual page's name line and synopsis. Manual pages are the one reference
// every Linux community shares, whatever it runs.
func (v *LookupHandlers) OnMan(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	l := v.RequesterLanguage(msg)
	if !v.QueryAllowed(ctx, msg, l) {
		return nil
	}
	bot := ctx.Bot()
	c := ctx.Context()
	man := &i18n.Messages.LookupDistros.Man

	arg := strings.TrimSpace(lookup.CommandArg(msg.Text))
	parts := lookup.ManNameRe.FindStringSubmatch(arg)
	if parts == nil {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, man.Usage.For(l))
		return nil
	}
	name, section := parts[1], parts[2]

	hc, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()
	page, found, failed := lookup.FetchManPage(hc, name, section)
	switch {
	case failed:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, man.Unavailable.For(l))
	case !found:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, man.NotFound.Render(l, arg))
	default:
		v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderMan(l, page))
	}
	return nil
}
