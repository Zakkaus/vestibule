package verification

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// The trusted-group bypass admits an applicant without a challenge when they already belong
// to a group this one trusts. Which Telegram member states count as belonging is decided by
// MemberIsMember, and the states that mean "thrown out" -- banned, and restricted after
// leaving -- were held by nothing: answering true for either admits somebody the trusted
// group has removed, without a challenge and without a trace.
func TestTrustedBypassRefusesStatesThatMeanRemoved(t *testing.T) {
	const gid, src, uid = int64(-1009000001501), int64(-1009000001502), int64(1501)
	for _, tc := range []struct {
		name   string
		member ChatMember
		admit  bool
	}{
		{"an ordinary member", &ChatMemberMember{Status: MemberStatusMember}, true},
		{"a restricted member still in the group",
			&ChatMemberRestricted{Status: MemberStatusRestricted, IsMember: true}, true},
		{"a restricted member who has left",
			&ChatMemberRestricted{Status: MemberStatusRestricted, IsMember: false}, false},
		{"somebody the trusted group banned",
			&ChatMemberBanned{Status: MemberStatusBanned}, false},
		{"somebody who left", &ChatMemberLeft{Status: MemberStatusLeft}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestService(&settings.Config{
				Groups: []settings.GroupConfig{{ID: gid, TrustedMemberGroupIDs: []int64{src}}},
			})
			bot := newFakeVerifyBot()
			bot.memberByID = map[int64]ChatMember{uid: tc.member}

			handled, trusted := service.tryTrustedBypass(context.Background(), bot, gid, uid)
			if handled != tc.admit || trusted != tc.admit {
				t.Fatalf("bypass for %s = handled %v trusted %v, want %v; the bypass skips the "+
					"challenge entirely, so it must only fire for somebody the trusted group "+
					"still holds", tc.name, handled, trusted, tc.admit)
			}
			if approved := bot.approves > 0; approved != tc.admit {
				t.Errorf("approvals for %s = %d, want admit=%v", tc.name, bot.approves, tc.admit)
			}
		})
	}
}

// The classification itself, including the two states no present call site consults: both
// gates switch on the status first and answer true for creator, administrator and member
// without asking. The rows are here so that a switch losing a case does not silently start
// asking a question nobody has kept an answer for.
func TestMemberStatesClassifyMembership(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member ChatMember
		want   bool
	}{
		{"member", &ChatMemberMember{}, true},
		{"administrator", &ChatMemberAdministrator{}, true},
		{"owner", &ChatMemberOwner{}, true},
		{"restricted and still in the chat", &ChatMemberRestricted{IsMember: true}, true},
		{"restricted after leaving", &ChatMemberRestricted{IsMember: false}, false},
		{"left", &ChatMemberLeft{}, false},
		{"banned", &ChatMemberBanned{}, false},
	} {
		if got := tc.member.MemberIsMember(); got != tc.want {
			t.Errorf("%s counts as a member = %v, want %v", tc.name, got, tc.want)
		}
	}
}
