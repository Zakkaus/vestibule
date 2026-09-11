package telegram

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func TestFreshRightsRejectsBotAdministrator(t *testing.T) {
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChatMember": {{value: &telego.ChatMemberAdministrator{
			Status:         telego.MemberStatusAdministrator,
			User:           telego.User{ID: 7, IsBot: true},
			CanInviteUsers: true, CanRestrictMembers: true, CanDeleteMessages: true,
		}}},
	}}
	client := newTestClient(t, caller)
	got, err := client.FreshRights(context.Background(), -100, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got != (verification.GroupRights{}) {
		t.Fatalf("bot administrator rights = %+v, want no human capabilities", got)
	}
}
