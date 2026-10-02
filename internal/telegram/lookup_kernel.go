package telegram

import (
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// OnKernel lists the kernel versions kernel.org currently publishes. Every Linux community asks
// this, and here it also answers the question the verification challenge raises.
func (v *LookupHandlers) OnKernel(ctx *th.Context, update telego.Update) error {
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
	kernel := &i18n.Messages.LookupDistros.Kernel

	releases, ok := lookup.FetchKernelReleases(c)
	if !ok {
		v.replyLookupPlain(c, bot, msg.Chat.ID, msg.MessageID, kernel.Unavailable.For(l))
		return nil
	}
	v.replyLookupHTML(c, bot, msg.Chat.ID, msg.MessageID, tgfmt.RenderKernel(l, releases))
	return nil
}
