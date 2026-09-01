package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestSettleConsoleRejectsAdministratorTarget(t *testing.T) {
	const groupID, userID = int64(-100), int64(42)
	service := newTestService(&settings.Config{Groups: []settings.GroupConfig{{ID: groupID}}, GroupIDs: []int64{groupID}})
	service.gateway = &fakeVerifyBot{member: &ChatMemberAdministrator{Status: MemberStatusAdministrator}}
	item := &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
	service.pend[pkey{gid: groupID, uid: userID}] = item
	_, err := service.SettleConsole(context.Background(), ConsoleSettlement{
		ID: challengeConsoleID(groupID, userID, item.nonce), GroupID: groupID, ActorID: 9,
		Expected: ChallengePending, Target: ChallengeApproved,
	})
	if !errors.Is(err, ErrConsoleTargetProtected) {
		t.Fatalf("settlement error = %v, want %v", err, ErrConsoleTargetProtected)
	}
	if item.done {
		t.Fatal("protected target must leave the pending challenge unchanged")
	}
}
