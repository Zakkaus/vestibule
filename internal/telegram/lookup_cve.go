package telegram

import (
	"context"
	"html"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// OnCVE reports one vulnerability from the National Vulnerability Database: how severe it is,
// when it was published, and what it is.
func (v *LookupHandlers) OnCVE(ctx *th.Context, update telego.Update) error {
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
	cve := &i18n.Messages.LookupDistros.CVE

	arg := strings.TrimSpace(lookup.CommandArg(msg.Text))
	match := lookup.CveIDRe.FindStringSubmatch(arg)
	if match == nil {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, cve.Usage.For(l))
		return nil
	}
	id := strings.ToUpper(match[1])

	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	record, found, failed := lookup.FetchCVE(hc, id)
	switch {
	case failed:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, cve.Unavailable.For(l))
	case !found:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, cve.NotFound.Render(l, html.EscapeString(id)))
	default:
		v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderCVE(l, id, record))
	}
	return nil
}
