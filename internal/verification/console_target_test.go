package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type targetAdminTrackingGateway struct {
	*fakeVerifyBot
	cachedAdminCalls int
}

func (g *targetAdminTrackingGateway) CachedAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	g.cachedAdminCalls++
	return g.fakeVerifyBot.CachedAdmin(ctx, chatID, userID)
}

func TestSettleConsoleRejectsAdministratorTarget(t *testing.T) {
	const groupID, userID = int64(-100), int64(42)
	service := newTestService(&settings.Config{Groups: []settings.GroupConfig{{ID: groupID}}, GroupIDs: []int64{groupID}})
	gateway := &targetAdminTrackingGateway{
		fakeVerifyBot: &fakeVerifyBot{member: &ChatMemberAdministrator{Status: MemberStatusAdministrator}},
	}
	service.gateway = gateway
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
	if gateway.cachedAdminCalls != 0 || len(gateway.memberRequests) != 1 {
		t.Fatalf("target cached_admin_calls=%d membership_queries=%d, want 0 and 1",
			gateway.cachedAdminCalls, len(gateway.memberRequests))
	}
	t.Logf("target cached_admin_calls=%d membership_queries=%d",
		gateway.cachedAdminCalls, len(gateway.memberRequests))
}
