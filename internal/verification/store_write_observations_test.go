package verification

import (
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
)

func TestRetryStoreWriteRecordsOneFinalDatabaseWriteOutcome(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	store := &flakyAgentStore{failures: storeWriteMaxAttempts}
	service := newTestService(&settings.Config{})
	service.rollbackObserver = observations
	service.agentPath = "database"
	service.stateStore = store

	service.recordAgent("model=gpt-5")

	snapshot := observations.Snapshot().DatabaseWrites
	if store.calls != storeWriteMaxAttempts || snapshot.TotalWrites != 1 || snapshot.FailedWrites != 1 ||
		snapshot.FailureRatePercent != 100 || !snapshot.ExceedsOnePercent {
		t.Fatalf("retry calls=%d database write observation=%+v", store.calls, snapshot)
	}
}
