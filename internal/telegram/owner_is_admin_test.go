package telegram

import (
	"context"
	"testing"

	"github.com/mymmrac/telego"
)

// Telegram calls the person who made the group "creator", not "administrator", so every place
// that asks "is this an administrator" has to name both. Four places do, spelled out by hand.
// Dropping creator from the connector's answer leaves every test passing, and the answer is
// the one the whole product asks: moderation commands, console access, and the approve and
// reject buttons all reach it. The group's owner would stop being an administrator everywhere
// at once.
func TestAdminLookupCountsTheOwner(t *testing.T) {
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
			caller := &scriptedCaller{responses: map[string][]scriptedResult{
				"getChatMember": {{value: tc.member}, {value: tc.member}},
			}}
			client := newTestClient(t, caller)
			if got, err := client.FreshAdmin(context.Background(), -1009000001601, 7); err != nil || got != tc.want {
				t.Fatalf("FreshAdmin for %s = (%v, %v), want (%v, nil)", tc.name, got, err, tc.want)
			}
			if got, err := client.CachedAdmin(context.Background(), -1009000001601, 8); err != nil || got != tc.want {
				t.Fatalf("CachedAdmin for %s = (%v, %v), want (%v, nil)", tc.name, got, err, tc.want)
			}
		})
	}
}
