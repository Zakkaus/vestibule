package telegram

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/telegram/ids"
	"github.com/Zakkaus/vestibule/internal/telegram/queue"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// VerificationGateway translates the core port to Bot API calls. It performs network I/O but
// never decides who passes or fails, and callers must not invoke it from a database transaction.
type VerificationGateway struct {
	connector *Connector
}

var _ verification.Gateway = (*VerificationGateway)(nil)

func NewVerificationGateway(connector *Connector) *VerificationGateway {
	if connector == nil {
		panic("telegram: verification gateway requires a connector")
	}
	return &VerificationGateway{connector: connector}
}

func (g *VerificationGateway) Send(ctx context.Context, message verification.OutgoingMessage) (int, error) {
	var params *telego.SendMessageParams
	if message.HTML {
		params = tgfmt.HTMLMessage(message.ChatID, message.Text)
	} else {
		params = tu.Message(tu.ID(message.ChatID), message.Text)
	}
	if len(message.Buttons) != 0 {
		rows := make([][]telego.InlineKeyboardButton, len(message.Buttons))
		for i, sourceRow := range message.Buttons {
			row := make([]telego.InlineKeyboardButton, len(sourceRow))
			for j, button := range sourceRow {
				row[j] = telego.InlineKeyboardButton{
					Text: button.Text, URL: button.URL, CallbackData: button.CallbackData,
				}
			}
			rows[i] = row
		}
		params = params.WithReplyMarkup(tu.InlineKeyboard(rows...))
	}
	sent, err := g.connector.bot.SendMessage(ctx, params)
	return ids.MessageID(sent), verificationError(err)
}

func (g *VerificationGateway) SendHTMLFallback(ctx context.Context, chatID int64, rich, simpler string) (int, error) {
	sent, err := g.connector.SendHTMLFallback(ctx, chatID, rich, simpler)
	return ids.MessageID(sent), verificationError(err)
}

func (g *VerificationGateway) Delete(ctx context.Context, chatID int64, messageID int) {
	g.connector.Delete(ctx, chatID, messageID)
}

func (g *VerificationGateway) Notify(ctx context.Context, chatID int64, text string, ttlSeconds int) {
	g.connector.Notify(ctx, chatID, text, ttlSeconds)
}

func (g *VerificationGateway) Alert(ctx context.Context, chatID int64, text string) {
	g.connector.Alert(ctx, chatID, text)
}

func (g *VerificationGateway) AuditLog(ctx context.Context, chatID int64, text string) {
	g.connector.AuditLog(ctx, chatID, text)
}

func (g *VerificationGateway) FailAlert(ctx context.Context, logChatID, groupID int64, text string) {
	g.connector.FailAlert(ctx, logChatID, groupID, text)
}

func (g *VerificationGateway) ApproveJoin(ctx context.Context, chatID, userID int64) error {
	return verificationError(g.connector.bot.ApproveChatJoinRequest(ctx, &telego.ApproveChatJoinRequestParams{
		ChatID: tu.ID(chatID), UserID: userID,
	}))
}

func (g *VerificationGateway) DeclineJoin(ctx context.Context, chatID, userID int64) error {
	return verificationError(g.connector.bot.DeclineChatJoinRequest(ctx, &telego.DeclineChatJoinRequestParams{
		ChatID: tu.ID(chatID), UserID: userID,
	}))
}

func (g *VerificationGateway) Ban(ctx context.Context, chatID, userID int64, seconds int, revoke bool) error {
	return verificationError(g.connector.Ban(ctx, chatID, userID, seconds, revoke))
}

func (g *VerificationGateway) Unban(ctx context.Context, chatID, userID int64, onlyIfBanned bool) error {
	return verificationError(g.connector.Unban(ctx, chatID, userID, onlyIfBanned))
}

func (g *VerificationGateway) Mute(ctx context.Context, chatID, userID int64, seconds int) error {
	return verificationError(g.connector.Mute(ctx, chatID, userID, seconds))
}

func (g *VerificationGateway) Unmute(ctx context.Context, chatID, userID int64) error {
	return verificationError(g.connector.Unmute(ctx, chatID, userID))
}

func (g *VerificationGateway) Member(ctx context.Context, chatID, userID int64) (verification.ChatMember, error) {
	member, err := g.connector.bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: tu.ID(chatID), UserID: userID})
	if err != nil || member == nil {
		return nil, verificationError(err)
	}
	return verificationMember(member), nil
}

func (g *VerificationGateway) CachedAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	ok, err := g.connector.CachedAdmin(ctx, chatID, userID)
	return ok, verificationError(err)
}

func (g *VerificationGateway) FreshAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	ok, err := g.connector.FreshAdmin(ctx, chatID, userID)
	return ok, verificationError(err)
}

func (g *VerificationGateway) AckFast(ctx context.Context, interactionID string) error {
	return verificationError(g.connector.bot.AnswerCallbackQuery(ctx, tu.CallbackQuery(interactionID)))
}

func (g *VerificationGateway) AckResult(ctx context.Context, interactionID string, result verification.AckResult) error {
	params := tu.CallbackQuery(interactionID).WithText(result.Text)
	if result.Alert {
		params = params.WithShowAlert()
	}
	return verificationError(g.connector.bot.AnswerCallbackQuery(ctx, params))
}

func verificationError(err error) error {
	if err == nil {
		return nil
	}
	var kinds verification.FailureKind
	if JoinRequestGone(err) {
		kinds |= verification.FailureJoinRequestGone
	}
	if ApplicantGone(err) {
		kinds |= verification.FailureApplicantGone
	}
	if CannotInitiateConversation(err) {
		kinds |= verification.FailureCannotInitiateConversation
	}
	if BotWasBlockedByUser(err) {
		kinds |= verification.FailureBlockedByUser
	}
	if queue.GroupUnreachable(err) {
		kinds |= verification.FailureGroupUnreachable
	}
	if queue.IsRateLimited(err) {
		kinds |= verification.FailureRateLimited
	}
	return &verification.GatewayError{
		Cause: err, Kinds: kinds, Code: queue.ErrorCode(err), RetryAfter: queue.RetryAfter(err),
	}
}

func verificationUser(user telego.User) verification.User {
	return verification.User{
		ID: user.ID, IsBot: user.IsBot, FirstName: user.FirstName,
		Username: user.Username, LanguageCode: user.LanguageCode,
	}
}

func verificationMember(member telego.ChatMember) verification.ChatMember {
	if member == nil {
		return nil
	}
	user := verificationUser(member.MemberUser())
	status := member.MemberStatus()
	switch status {
	case telego.MemberStatusCreator:
		return &verification.ChatMemberOwner{Status: status, User: user}
	case telego.MemberStatusAdministrator:
		return &verification.ChatMemberAdministrator{Status: status, User: user}
	case telego.MemberStatusRestricted:
		until := int64(0)
		if restricted, ok := member.(*telego.ChatMemberRestricted); ok {
			until = restricted.UntilDate
		}
		return &verification.ChatMemberRestricted{
			Status: status, User: user, IsMember: member.MemberIsMember(), UntilDate: until,
		}
	case telego.MemberStatusLeft:
		return &verification.ChatMemberLeft{Status: status, User: user}
	case telego.MemberStatusBanned:
		return &verification.ChatMemberBanned{Status: status, User: user}
	default:
		return &verification.ChatMemberMember{Status: status, User: user}
	}
}

// NewVerificationHandlers converts protocol updates before invoking the core service.
func NewVerificationHandlers(service *verification.Service, gateway *VerificationGateway) VerificationHandlers {
	return VerificationHandlers{
		Answer:         verificationHandler(service.OnAnswer, gateway),
		AdminAction:    verificationHandler(service.OnAdminAction, gateway),
		ChannelRecheck: verificationHandler(service.OnChannelRecheck, gateway),
		JoinRequest:    verificationHandler(service.OnJoinRequest, gateway),
		MemberJoined:   verificationHandler(service.OnMemberJoined, gateway),
		KernelAnswer:   verificationHandler(service.OnKernelAnswer, gateway),
		KernelAnswerDM: func(_ context.Context, update telego.Update) bool {
			message := update.Message
			return message != nil && message.From != nil && service.KernelAnswerDM(
				message.From.ID, message.Text, message.Chat.Type == telego.ChatTypePrivate,
			)
		},
		AnswerPrefix:         verification.AnswerCallbackPrefix,
		AdminPrefix:          verification.AdminCallbackPrefix,
		ChannelRecheckPrefix: verification.ChannelRecheckCallbackPrefix,
	}
}

func verificationHandler(handler verification.Handler, gateway *VerificationGateway) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		return handler(verification.NewHandlerContext(ctx.Context(), gateway), verificationUpdate(update))
	}
}

func verificationUpdate(update telego.Update) verification.Update {
	converted := verification.Update{}
	if request := update.ChatJoinRequest; request != nil {
		converted.ChatJoinRequest = &verification.ChatJoinRequest{
			Chat: verification.Chat{ID: request.Chat.ID, Type: request.Chat.Type},
			From: verificationUser(request.From),
		}
	}
	if membership := update.ChatMember; membership != nil {
		converted.ChatMember = &verification.ChatMemberUpdated{
			Chat:          verification.Chat{ID: membership.Chat.ID, Type: membership.Chat.Type},
			From:          verificationUser(membership.From),
			OldChatMember: verificationMember(membership.OldChatMember),
			NewChatMember: verificationMember(membership.NewChatMember),
		}
	}
	if callback := update.CallbackQuery; callback != nil {
		converted.CallbackQuery = &verification.CallbackQuery{
			ID: callback.ID, From: verificationUser(callback.From), Data: callback.Data,
		}
	}
	if message := update.Message; message != nil {
		converted.Message = &verification.Message{
			MessageID: message.MessageID,
			Chat:      verification.Chat{ID: message.Chat.ID, Type: message.Chat.Type},
			Text:      message.Text,
		}
		if message.From != nil {
			from := verificationUser(*message.From)
			converted.Message.From = &from
		}
	}
	return converted
}
