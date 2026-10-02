package telegram

import (
	"context"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// OnPkgs handles cross-distribution package version lookups.
func (v *LookupHandlers) OnPkgs(ctx *th.Context, update telego.Update) error {
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
	name := lookup.CommandArg(msg.Text)
	if name == "" {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupDistros.Pkgs.Usage.For(l))
		return nil
	}
	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	result := lookup.QueryDistros(hc, name)
	if !result.Found {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderRepologyLookupMiss(l, name, result.Available))
		return nil
	}
	if len(result.Rows) == 0 {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupDistros.Pkgs.NoSupportedDistro.Render(l, result.Project))
		return nil
	}
	rich, plain := tgfmt.RenderDistros(l, name, result)
	v.sendRichOrHTML(c, bot, msg.Chat.ID, msg.MessageID, rich, plain)
	return nil
}

// OnArmpkgs handles cross-distribution arm64 support lookups.
func (v *LookupHandlers) OnArmpkgs(ctx *th.Context, update telego.Update) error {
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
	name := lookup.CommandArg(msg.Text)
	if name == "" {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupDistros.Armpkgs.Usage.For(l))
		return nil
	}
	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	results := lookup.QueryArmPackages(hc, name)
	v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderArmPackages(l, name, results))
	return nil
}
