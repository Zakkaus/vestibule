package telegram

import (
	"context"
	"testing"

	"github.com/mymmrac/telego"
)

// Registration asks twice whether somebody is an administrator: of the person acting on a
// membership change or an enrolment link, and of the bot itself in a chat. Telegram calls the
// person who made the group "creator", so both have to name that status as well as
// "administrator". Dropping it from either left every test in the repository passing.
//
// The actor check gates enrolment and the membership-change path, and refusing the owner
// means the one person who can always add a bot to their own group is turned away from
// registering it.
func TestRegistrationCountsTheOwnerAsAdministrator(t *testing.T) {
	const groupID, actorID = int64(-1009000001701), int64(1701)
	for _, tc := range []struct {
		name   string
		member telego.ChatMember
		want   bool
	}{
		{"the person who made the group", &telego.ChatMemberOwner{Status: telego.MemberStatusCreator}, true},
		{"an administrator", &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}, true},
		{"an ordinary member", &telego.ChatMemberMember{Status: telego.MemberStatusMember}, false},
		{"somebody who left", &telego.ChatMemberLeft{Status: telego.MemberStatusLeft}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, store := registrationFixture(t)
			caller := &registrationCaller{
				members: map[[2]int64]telego.ChatMember{{groupID, actorID}: tc.member},
				events:  make(chan string, 16),
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			service := newRegistrationService(ctx, newRegistrationBot(t, caller), store, cfg,
				"verify_test_bot", testBotID, nil, nil, nil)

			if got := service.actorIsAdmin(ctx, groupID, actorID); got != tc.want {
				t.Fatalf("actorIsAdmin for %s = %v, want %v; registration and enrolment are "+
					"gated on this answer", tc.name, got, tc.want)
			}
		})
	}
}

// The same question about the bot, which decides whether it can work in a chat at all.
func TestBotMembershipCountsCreatorAsAdministrator(t *testing.T) {
	const groupID = int64(-1009000001702)
	for _, tc := range []struct {
		name   string
		member telego.ChatMember
		want   botMembershipState
	}{
		{"the bot made the chat", &telego.ChatMemberOwner{Status: telego.MemberStatusCreator}, botMembershipAdmin},
		{"the bot is an administrator", &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}, botMembershipAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, store := registrationFixture(t)
			caller := &registrationCaller{
				members: map[[2]int64]telego.ChatMember{{groupID, testBotID}: tc.member},
				events:  make(chan string, 16),
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			service := newRegistrationService(ctx, newRegistrationBot(t, caller), store, cfg,
				"verify_test_bot", testBotID, nil, nil, nil)

			got, err := service.currentBotMembership(ctx, groupID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("bot membership for %s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
