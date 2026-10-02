package telegram

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

type LookupHandlers struct {
	*lookup.Service
	Telegram *Connector
}

func NewLookupHandlers(service *lookup.Service, connector *Connector) *LookupHandlers {
	return &LookupHandlers{Service: service, Telegram: connector}
}

// Delete group lookup commands and answers together using a fresh timer context.
func (s *LookupHandlers) scheduleLookupCleanup(_ *telego.Bot, chatID int64, cmdMsgID, respMsgID int) {
	s.Telegram.ScheduleCleanup(chatID, cmdMsgID, respMsgID, s.CleanupAfter(chatID))
}

// Plain text preserves angle-bracket placeholders and still follows reply/cleanup semantics.
func (s *LookupHandlers) replyLookupPlain(c context.Context, _ *telego.Bot, chatID int64, replyTo int, text string) {
	s.Telegram.ReplyPlain(c, chatID, replyTo, text, s.CleanupAfter(chatID))
}

// HTML lookup replies require callers to escape dynamic content.
func (s *LookupHandlers) replyLookupHTML(c context.Context, _ *telego.Bot, chatID int64, replyTo int, htmlText string) *telego.Message {
	return s.Telegram.ReplyHTML(c, chatID, replyTo, htmlText, s.CleanupAfter(chatID))
}

// Bot API rich messages fall back to HTML on server rejection.
func (s *LookupHandlers) sendRichOrHTML(c context.Context, _ *telego.Bot, chatID int64, replyTo int, richHTML, plainHTML string) {
	s.Telegram.SendRichOrHTML(c, chatID, replyTo, richHTML, plainHTML, s.RichEnabled(chatID), s.CleanupAfter(chatID))
}

func (s *LookupHandlers) RequesterLanguage(msg *telego.Message) i18n.Lang {
	fallback := i18n.LangEN
	if s.IsGroup(msg.Chat.ID) {
		fallback = s.GroupLanguage(msg.Chat.ID)
	}
	return i18n.FromRequester(msg.From.LanguageCode, fallback)
}

// RenamedHandler returns a route handler that directs users to a canonical command.
func (s *LookupHandlers) RenamedHandler(canonical string) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		message := update.Message
		if message == nil || message.From == nil {
			return nil
		}
		language := s.RequesterLanguage(message)
		s.replyLookupPlain(ctx.Context(), ctx.Bot(), message.Chat.ID, message.MessageID,
			i18n.Messages.Bot.RenamedCommand.Render(language, canonical))
		return nil
	}
}

// External lookups are unlimited in guarded groups and rate-limited per user in private chats.
func (s *LookupHandlers) QueryAllowed(ctx *th.Context, msg *telego.Message, l i18n.Lang) bool {
	if s.IsGroup(msg.Chat.ID) {
		return true
	}
	if msg.Chat.Type == "private" && msg.From != nil {
		if s.QueryRateOK(msg.From.ID) {
			return true
		}
		_, _ = ctx.Bot().SendMessage(ctx.Context(), tu.Message(tu.ID(msg.Chat.ID),
			i18n.Messages.LookupContent.Transport.PrivateRateLimited.Render(l, s.PrivateQueryPerMin())))
		return false
	}
	return false
}
