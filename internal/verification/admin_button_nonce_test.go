package verification

import (
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// An administrator button carries the nonce of the challenge it was rendered for, so that a
// button left in the group cannot settle whatever verification is open for that person now.
// The check used to run only when the payload carried a nonce, which meant a payload without
// one skipped the protection its own comment described: a three-part button settled the
// current pending whatever its nonce stood for. The plan requires that an old nonce cannot
// settle a new challenge, and a payload with no nonce reaches the same end.
//
// This drives the handler rather than the lookup it calls, because the guard being held is
// in the handler; asserting the lookup passes with the guard removed.
func TestAdminButtonWithoutTheChallengeNonceSettlesNothing(t *testing.T) {
	const gid, uid, adminID = int64(-100), int64(5), int64(77)
	for _, tc := range []struct {
		name    string
		payload string
		settles bool
	}{
		{"the nonce it was rendered with", ":n", true},
		{"a nonce from a replaced challenge", ":old", false},
		{"no nonce at all", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{BanSeconds: 3600})
			v.pend[pkey{gid, uid}] = &pending{nonce: "n", deadline: time.Now().Add(time.Hour)}
			bot := newFakeVerifyBot()
			bot.member = &ChatMemberAdministrator{Status: MemberStatusAdministrator}
			update := Update{CallbackQuery: &CallbackQuery{
				ID: "admin", From: User{ID: adminID},
				Data: AdminCallbackPrefix + "pass:-100:5" + tc.payload,
			}}

			runFakeHandler(t, newAPITestBot(t, bot), v.OnAdminAction, update)

			if settled := bot.approves > 0; settled != tc.settles {
				t.Errorf("a button carrying %q approved = %v, want %v; the challenge holds nonce %q",
					tc.payload, settled, tc.settles, "n")
			}
		})
	}
}
