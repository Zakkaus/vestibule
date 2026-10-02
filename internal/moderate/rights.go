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
	if notice := s.rightsFailure(ctx, chatID, userID, command, l, required); notice != "" {
		s.notify(ctx, chatID, notice)
		return false
	}
	return true
}

func (s *Service) rightsFailure(ctx context.Context, chatID, userID int64, command string, l i18n.Lang, required verification.GroupRights) string {
	got, err := s.telegram.FreshRights(ctx, chatID, userID)
	if err != nil {
		log.Printf("requireRights getChatMember chat=%d user=%d: %v", chatID, userID, err)
		return i18n.Messages.Moderate.Common.CallerAdminCheckFailed.For(l)
	}
	if !requiredRightsSatisfied(got, required) {
		return i18n.Messages.Moderate.Common.CommandAdminOnly.Render(l, command)
	}
	return ""
}
