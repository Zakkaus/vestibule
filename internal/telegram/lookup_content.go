package telegram

import (
	"context"
	"log"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/ids"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// OnBug handles Gentoo Bugzilla lookups.
func (v *LookupHandlers) OnBug(ctx *th.Context, update telego.Update) error {
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
	id := lookup.CommandArg(msg.Text)
	if !lookup.BugIDRe.MatchString(id) {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupContent.Bug.Usage.For(l))
		return nil
	}
	link := "https://bugs.gentoo.org/" + id

	hc, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()
	info, state := lookup.FetchBug(hc, id)
	if state != lookup.BugLookupFound {
		// Keep unsuccessful lookups on the reply-linked cleanup path.
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.BugLookupFailureMessage(l, id, link, state))
		return nil
	}

	v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderBug(l, id, link, info))
	return nil
}

// OnNews handles Gentoo news lookups.
func (v *LookupHandlers) OnNews(ctx *th.Context, update telego.Update) error {
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
	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	items, available := lookup.GetNews(hc)
	arg := lookup.CommandArg(msg.Text)
	b := tgfmt.RenderNews(l, arg, items, available)
	v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, b)
	return nil
}

// OnWiki handles Gentoo Wiki and ArchWiki searches.
func (v *LookupHandlers) OnWiki(ctx *th.Context, update telego.Update) error {
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
	q := lookup.CommandArg(msg.Text)
	if q == "" {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupContent.Wiki.Usage.For(l))
		return nil
	}
	hc, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()

	results := lookup.QueryWiki(hc, l, q)
	v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderWiki(l, q, results))
	return nil
}

// OnBbs handles Linux forum searches.
func (v *LookupHandlers) OnBbs(ctx *th.Context, update telego.Update) error {
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
	q := lookup.CommandArg(msg.Text)
	if q == "" {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupContent.BBS.Usage.For(l))
		return nil
	}
	hc, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()

	hits, available := lookup.SearchArchcn(hc, q, 5)
	text, rows := tgfmt.RenderBBS(l, q, hits, available)
	sent, err := bot.SendMessage(c, tgfmt.HTMLMessage(msg.Chat.ID, text).
		WithReplyMarkup(tu.InlineKeyboard(rows...)).
		WithReplyParameters(ids.ReplyParameters(msg.MessageID)))
	if err != nil {
		// Preserve inline results when Telegram rejects the buttons.
		log.Printf("/gbbs send with buttons failed (%v) — retrying text-only", err)
		sent, _ = bot.SendMessage(c, tgfmt.HTMLMessage(msg.Chat.ID, text).WithReplyParameters(ids.ReplyParameters(msg.MessageID)))
	}
	v.scheduleLookupCleanup(bot, msg.Chat.ID, msg.MessageID, ids.MessageID(sent))
	return nil
}
