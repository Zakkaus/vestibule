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

// OnPkg handles package searches across the Gentoo tree and configured overlays.
func (v *LookupHandlers) OnPkg(ctx *th.Context, update telego.Update) error {
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
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupPackages.Pkg.Usage.For(l))
		return nil
	}
	q = lookup.NormalizeQuery(q)

	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	result := lookup.QueryPackages(hc, q)
	plain := tgfmt.RenderPkg(l, q, result)
	rich := ""
	if v.RichEnabled(msg.Chat.ID) {
		rich = tgfmt.RenderPkgRich(l, q, result)
	}
	v.sendRichOrHTML(c, bot, msg.Chat.ID, msg.MessageID, rich, plain)
	return nil
}

func (v *LookupHandlers) OnUse(ctx *th.Context, update telego.Update) error {
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
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupPackages.Use.Usage.For(l))
		return nil
	}
	q = lookup.NormalizeQuery(q)
	hc, cancel := context.WithTimeout(c, 25*time.Second)
	defer cancel()
	result := lookup.QueryUse(hc, q)
	switch len(result.Atoms) {
	case 0:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderUseLookupMiss(l, q, result.Availability))
		return nil
	case 1:
	default:
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderUseMultipleMatches(l, result.Atoms, result.Availability))
		return nil
	}
	if !result.Found {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupPackages.Use.InfoUnavailable.Render(l, result.Atom))
		return nil
	}
	out := tgfmt.RenderUse(l, result.Info, result.Source, result.URL, result.Overlay, result.AlsoIn)
	outRich := ""
	if v.RichEnabled(msg.Chat.ID) {
		outRich = tgfmt.RenderUseRich(l, result.Info, result.Source, result.URL, result.Overlay, result.AlsoIn)
	}
	out, outRich = tgfmt.AppendUseAvailabilityNote(l, out, outRich, result.Availability)
	v.sendRichOrHTML(c, bot, msg.Chat.ID, msg.MessageID, outRich, out)
	return nil
}

// OnArm handles Gentoo arm64 keyword lookups.
func (v *LookupHandlers) OnArm(ctx *th.Context, update telego.Update) error {
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
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, i18n.Messages.LookupPackages.Arm.Usage.For(l))
		return nil
	}
	hc, cancel := context.WithTimeout(c, 20*time.Second)
	defer cancel()
	result := lookup.LookupArm(hc, name, lookup.SearchMainTree, lookup.ArmStatus)
	body, useHTML := tgfmt.RenderArm(l, name, result)
	if useHTML {
		v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, body)
	} else {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, body)
	}
	return nil
}
