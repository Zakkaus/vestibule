package moderate

import (
	"context"
	"log"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func requiredRightsSatisfied(got, required verification.GroupRights) bool {
	return (!required.CanInviteUsers || got.CanInviteUsers) &&
		(!required.CanRestrictMembers || got.CanRestrictMembers) &&
		(!required.CanDeleteMessages || got.CanDeleteMessages)
}

// requireRights performs a fresh capability check before an administrator command can write.
func (s *Service) requireRights(
	ctx context.Context,
	chatID, userID int64,
	command string,
	l i18n.Lang,
	required verification.GroupRights,
) bool {
	got, err := s.telegram.FreshRights(ctx, chatID, userID)
	if err != nil {
		log.Printf("requireRights getChatMember chat=%d user=%d: %v", chatID, userID, err)
		s.notify(ctx, chatID, i18n.Messages.Moderate.Common.CallerAdminCheckFailed.For(l))
		return false
	}
	if !requiredRightsSatisfied(got, required) {
		s.notify(ctx, chatID, i18n.Messages.Moderate.Common.CommandAdminOnly.Render(l, command))
		return false
	}
	return true
}
