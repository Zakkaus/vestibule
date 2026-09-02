package verification

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// Caller cancellation and the service shutdown flag are independent guards. The canceled-context
// case lives in TestScanExpiredDeclinesNobodyOnceShutdownStarted; this case keeps the context live.
// ScanExpired, installExpiryClaim, and claimPendingExpiry all enforce the flag, so removing only one
// guard does not make this property fail: the other two deliberately provide defense in depth.
func TestServiceShutdownPreventsExpirySettlementWithLiveContext(t *testing.T) {
	const groupID, userID = int64(-1009000000941), int64(941)
	now := time.Unix(1_700_000_000, 0)
	service := newTestService(&settings.Config{
		Groups:         []settings.GroupConfig{{ID: groupID}},
		GroupIDs:       []int64{groupID},
		TimeoutSeconds: 240,
	})
	service.timeNow = func() time.Time { return now }
	item := &pending{nonce: "due", deadline: now, challengeDelivered: true}
	state := &actionTestStore{pending: []PendingRecord{pendingRecord(pkey{groupID, userID}, item)}}
	service.stateStore = state
	service.statePath = "service-shutdown-expiry-test"
	gateway := newFakeVerifyBot()
	service.gateway = gateway
	service.pend[pkey{groupID, userID}] = item

	service.Shutdown()
	service.ScanExpired(context.Background())

	if decisions := gateway.approves + gateway.declines + gateway.bans; decisions != 0 {
		t.Fatalf("verification decisions after service shutdown = %d, want 0", decisions)
	}
	if _, exists := service.pend[pkey{groupID, userID}]; !exists {
		t.Fatal("service shutdown lost the pending verification instead of leaving it for restart")
	}
}
