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

// OnRepology lists one project's newest version in every repository family Repology tracks.
// /pkgs answers the same question for a handful of distributions and adds their release status;
// this is the wide view, for when the question is which repository has it at all.
func (v *LookupHandlers) OnRepology(ctx *th.Context, update telego.Update) error {
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
	repology := &i18n.Messages.LookupDistros.Repology

	name := strings.ToLower(strings.TrimSpace(lookup.CommandArg(msg.Text)))
	if !lookup.RepologyNameRe.MatchString(name) {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, repology.Usage.For(l))
		return nil
	}

	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	proj, pkgs, _, _, available := lookup.FetchRepology(hc, name)
	switch {
	case !available:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, repology.Unavailable.For(l))
	case len(pkgs) == 0:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, repology.NotFound.Render(l, html.EscapeString(name)))
	default:
		entries, others := lookup.RepologyByFamily(pkgs)
		v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderRepology(l, proj, entries, others))
	}
	return nil
}
