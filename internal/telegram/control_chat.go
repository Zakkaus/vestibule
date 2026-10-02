package telegram

import (
	"context"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

func (s *registrationService) checkControlChatAssignment(chatID, actorID int64) error {
	ctx, cancel := context.WithTimeout(s.root, 5*time.Second)
	defer cancel()
	chat, err := s.bot.GetChat(ctx, &telego.GetChatParams{ChatID: tu.ID(chatID)})
	if err != nil || chat == nil || (chat.Type != telego.ChatTypeGroup && chat.Type != telego.ChatTypeSupergroup) {
		return errBotMembershipUnreadable
	}
	member, err := s.bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: tu.ID(chatID), UserID: s.selfID})
	if err != nil || member == nil || !member.MemberIsMember() || !s.actorIsAdmin(ctx, chatID, actorID) {
		return errBotMembershipUnreadable
	}
	return nil
}
