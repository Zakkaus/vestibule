package verification

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func (b *fakeVerifyBot) FreshRights(ctx context.Context, chatID, userID int64) (GroupRights, error) {
	if b.rightsErr != nil {
		return GroupRights{}, b.rightsErr
	}
	if b.rightsSet {
		return b.rights, nil
	}
	member, err := b.Member(ctx, chatID, userID)
	if err != nil {
		return GroupRights{}, err
	}
	if member == nil {
		return GroupRights{CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true}, nil
	}
	switch member.MemberStatus() {
	case MemberStatusCreator, MemberStatusAdministrator:
		return GroupRights{CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true}, nil
	default:
		return GroupRights{}, nil
	}
}

func TestAdministratorCallbacksRequireFullFreshRightsBeforeConsumingPending(t *testing.T) {
	const (
		groupID  int64 = -100900000300
		targetID int64 = 801
		adminID  int64 = 802
	)
	all := GroupRights{CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true}
	cases := []struct {
		name   string
		rights GroupRights
		role   string
		settle bool
	}{
		{name: "no capabilities", rights: GroupRights{}},
		{name: "invite only", rights: GroupRights{CanInviteUsers: true}},
		{name: "restrict only", rights: GroupRights{CanRestrictMembers: true}},
		{name: "delete only", rights: GroupRights{CanDeleteMessages: true}},
		{name: "invite and restrict", rights: GroupRights{CanInviteUsers: true, CanRestrictMembers: true}},
		{name: "invite and delete", rights: GroupRights{CanInviteUsers: true, CanDeleteMessages: true}},
		{name: "restrict and delete", rights: GroupRights{CanRestrictMembers: true, CanDeleteMessages: true}},
		{name: "administrator with all capabilities", rights: all, role: MemberStatusAdministrator, settle: true},
		{name: "creator with all capabilities", rights: all, role: MemberStatusCreator, settle: true},
	}
	for _, action := range []string{"pass", "ban"} {
		for _, tc := range cases {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				v := newTestService(&settings.Config{GroupIDs: []int64{groupID}, BanSeconds: 3600})
				key := pkey{gid: groupID, uid: targetID}
				v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
				bot := newFakeVerifyBot()
				bot.memberByID = map[int64]ChatMember{
					adminID: &ChatMemberAdministrator{Status: MemberStatusAdministrator},
				}
				if tc.role != "" {
					bot.memberByID[adminID] = &ChatMemberOwner{Status: tc.role}
				}
				bot.rights, bot.rightsSet = tc.rights, true

				err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
					CallbackQuery: &CallbackQuery{
						ID: "rights-matrix", From: User{ID: adminID},
						Data: AdminCallbackPrefix + action + ":-100900000300:801:current",
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				_, pending := v.pend[key]
				if pending == tc.settle {
					t.Fatalf("pending presence = %t, want %t", pending, !tc.settle)
				}
				if !tc.settle {
					if bot.approves != 0 || bot.bans != 0 || bot.declines != 0 {
						t.Fatalf("insufficient-rights callback performed actions: approves=%d bans=%d declines=%d", bot.approves, bot.bans, bot.declines)
					}
				}
			})
		}
	}
}

func TestAdministratorCallbackRejectsBotOrAnonymousSender(t *testing.T) {
	const (
		groupID  int64 = -100900000301
		targetID int64 = 811
	)
	for _, tc := range []struct {
		name string
		from User
	}{
		{name: "bot sender", from: User{ID: 812, IsBot: true}},
		{name: "anonymous sender", from: User{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{groupID}, BanSeconds: 3600})
			key := pkey{gid: groupID, uid: targetID}
			v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
			bot := newFakeVerifyBot()
			bot.rights, bot.rightsSet = GroupRights{
				CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true,
			}, true
			err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "sender-boundary", From: tc.from,
					Data: AdminCallbackPrefix + "pass:-100900000301:811:current",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, pending := v.pend[key]; !pending {
				t.Fatal("bot or anonymous callback consumed pending challenge")
			}
			if bot.approves != 0 || bot.bans != 0 || bot.declines != 0 {
				t.Fatalf("sender boundary performed actions: approves=%d bans=%d declines=%d",
					bot.approves, bot.bans, bot.declines)
			}
		})
	}
}
