package moderate

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func TestBanRequiresRestrictAndDeleteRightsBeforeTelegramActions(t *testing.T) {
	const groupID int64 = -1009000000501

	tests := []struct {
		name    string
		member  telego.ChatMember
		rights  verification.GroupRights
		allowed bool
	}{
		{
			name: "delete-only administrator is refused",
			member: &telego.ChatMemberAdministrator{
				Status:            telego.MemberStatusAdministrator,
				CanDeleteMessages: true,
			},
			rights: verification.GroupRights{CanDeleteMessages: true},
		},
		{
			name: "restrict-only administrator is refused",
			member: &telego.ChatMemberAdministrator{
				Status:             telego.MemberStatusAdministrator,
				CanRestrictMembers: true,
			},
			rights: verification.GroupRights{CanRestrictMembers: true},
		},
		{
			name: "administrator with both rights is allowed",
			member: &telego.ChatMemberAdministrator{
				Status:             telego.MemberStatusAdministrator,
				CanRestrictMembers: true,
				CanDeleteMessages:  true,
			},
			rights:  verification.GroupRights{CanRestrictMembers: true, CanDeleteMessages: true},
			allowed: true,
		},
		{
			name:    "creator is allowed",
			member:  &telego.ChatMemberOwner{Status: telego.MemberStatusCreator},
			rights:  verification.GroupRights{CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true},
			allowed: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			telegram := newFakeMod()
			telegram.rightsByID = map[int64]verification.GroupRights{7: test.rights}
			telegram.memberByID = map[int64]telego.ChatMember{
				7: test.member,
				8: &telego.ChatMemberMember{Status: telego.MemberStatusMember},
			}
			service := newAuthorizationTestService(t, telegram)
			message := moderationCommand(groupID, "/ban")
			runFakeHandler(t, newAPITestBot(t, telegram), service.OnBan, telego.Update{Message: message})

			if test.allowed {
				if telegram.bans != 1 {
					t.Fatalf("ban calls = %d, want 1", telegram.bans)
				}
				if !containsMessageID(telegram.deletedMessageIDs, message.ReplyToMessage.MessageID) {
					t.Fatalf("deleted message IDs = %v, want target message %d", telegram.deletedMessageIDs, message.ReplyToMessage.MessageID)
				}
				return
			}

			if telegram.bans != 0 {
				t.Errorf("ban calls = %d, want 0 when caller lacks a required right", telegram.bans)
			}
			if containsMessageID(telegram.deletedMessageIDs, message.ReplyToMessage.MessageID) {
				t.Errorf("deleted message IDs = %v, want command cleanup only; target evidence must remain", telegram.deletedMessageIDs)
			}
		})
	}
}

func containsMessageID(ids []int, want int) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
