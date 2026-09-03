package status

import (
	"testing"
	"time"
)

func TestChallengeDeliveryObservationRequiresSustainedProblemEvents(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })

	observations.RecordChallengeDeliveryFailure()
	oneFailure := observations.Snapshot().ChallengeDelivery
	assertChallengeDeliveryStreak(t, "one delivery failure", oneFailure, 1, 0, false)

	now = now.Add(RollbackObservationWindow)
	observations.RecordChallengeDeliveryFailure()
	atThreshold := observations.Snapshot().ChallengeDelivery
	assertChallengeDeliveryStreak(t, "delivery failures at ten minutes", atThreshold, 2, RollbackObservationWindow, false)

	now = now.Add(time.Second)
	observations.RecordChallengeDeliveryFailure()
	sustained := observations.Snapshot().ChallengeDelivery
	assertChallengeDeliveryStreak(t, "sustained delivery failures", sustained, 3, RollbackObservationWindow+time.Second, true)

	observations.RecordChallengeDeliverySuccess()
	recovered := observations.Snapshot().ChallengeDelivery
	assertRecoveredChallengeDelivery(t, recovered, now)
}

func TestChallengeDeliveryTimestampsBoundTheProblemEvents(t *testing.T) {
	first := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	now := first
	observations := NewRollbackObservations(func() time.Time { return now })

	observations.RecordChallengeDeliveryFailure()
	now = now.Add(2 * time.Minute)
	observations.RecordChallengeDeliveryDuplicate()

	got := observations.Snapshot().ChallengeDelivery
	if got.FirstProblemAt == nil || !got.FirstProblemAt.Equal(first) ||
		got.LastProblemAt == nil || !got.LastProblemAt.Equal(now) {
		t.Fatalf("delivery problem bounds no longer identify the first and latest events: %+v", got)
	}
}

func TestAnOldChallengeFailureDoesNotBecomeSustainedWithoutANewProblem(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordChallengeDeliveryFailure()

	now = now.Add(RollbackObservationWindow + time.Second)
	got := observations.Snapshot().ChallengeDelivery
	if got.ObservedFor != 0 || got.ExceedsWindow || got.ProblemEvents != 1 {
		t.Fatalf("one old delivery failure became a sustained outage without another problem event: %+v", got)
	}
}

func TestChallengeDeliveryProblemSpanCannotGoNegative(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordChallengeDeliveryFailure()

	now = now.Add(-time.Second)
	observations.RecordChallengeDeliveryDuplicate()
	got := observations.Snapshot().ChallengeDelivery
	if got.ObservedFor != 0 || got.ExceedsWindow {
		t.Fatalf("a backward clock step produced a negative delivery problem span: %+v", got)
	}
}

func TestSuccessfulChallengeDeliveryDoesNotInventARecovery(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })

	observations.RecordChallengeDeliverySuccess()
	if got := observations.Snapshot().ChallengeDelivery; got.LastRecoveredAt != nil {
		t.Fatalf("a successful delivery without an earlier problem reported a recovery: %+v", got)
	}
}

func TestRecoveredChallengeDeliveryStartsWithEmptyCounts(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordChallengeDeliveryFailure()
	observations.RecordChallengeDeliveryDuplicate()
	observations.RecordChallengeDeliverySuccess()

	recovered := observations.Snapshot().ChallengeDelivery
	if recovered.ProblemEvents != 0 || recovered.FailedDeliveries != 0 || recovered.DuplicateDeliveries != 0 {
		t.Fatalf("recovered delivery retained counts from the ended problem streak: %+v", recovered)
	}

	now = now.Add(time.Second)
	observations.RecordChallengeDeliveryFailure()
	next := observations.Snapshot().ChallengeDelivery
	if next.ProblemEvents != 1 || next.FailedDeliveries != 1 || next.DuplicateDeliveries != 0 {
		t.Fatalf("new delivery problem inherited counts from the recovered streak: %+v", next)
	}
}

func assertChallengeDeliveryStreak(
	t *testing.T,
	label string,
	got ChallengeDeliverySnapshot,
	failures uint64,
	observedFor time.Duration,
	exceeds bool,
) {
	t.Helper()
	if got.ExceedsWindow != exceeds || got.FailedDeliveries != failures ||
		got.ProblemEvents != failures || got.ObservedFor != observedFor {
		t.Fatalf("%s = %+v", label, got)
	}
}

func assertRecoveredChallengeDelivery(t *testing.T, got ChallengeDeliverySnapshot, recoveredAt time.Time) {
	t.Helper()
	if got.FirstProblemAt != nil || got.LastProblemAt != nil || got.ProblemEvents != 0 ||
		got.LastRecoveredAt == nil || !got.LastRecoveredAt.Equal(recoveredAt) {
		t.Fatalf("recovered delivery observation = %+v", got)
	}
}

func TestSuccessfulDeliveryClearsResolvedIncidentCounters(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })

	observations.RecordChallengeDeliveryFailure()
	observations.RecordChallengeDeliveryDuplicate()
	beforeRecovery := observations.Snapshot().ChallengeDelivery
	if beforeRecovery.FailedDeliveries != 1 || beforeRecovery.DuplicateDeliveries != 1 {
		t.Fatalf("delivery incident counters before recovery = %+v, want one failed and one duplicate delivery", beforeRecovery)
	}

	now = now.Add(time.Second)
	observations.RecordChallengeDeliverySuccess()
	recovered := observations.Snapshot().ChallengeDelivery
	if recovered.FailedDeliveries != 0 || recovered.DuplicateDeliveries != 0 {
		t.Fatalf("resolved delivery incident remains in rollback diagnostics: %+v", recovered)
	}
}

func TestDatabaseWriteFailureRateUsesLogicalWriteWindow(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })

	for range 99 {
		observations.RecordDatabaseWrite(true)
	}
	observations.RecordDatabaseWrite(false)
	atOnePercent := observations.Snapshot().DatabaseWrites
	if atOnePercent.TotalWrites != 100 || atOnePercent.FailedWrites != 1 ||
		atOnePercent.FailureRatePercent != 1 || atOnePercent.ExceedsOnePercent {
		t.Fatalf("one-percent write rate = %+v, want exactly one percent without threshold breach", atOnePercent)
	}

	observations.RecordDatabaseWrite(false)
	overOnePercent := observations.Snapshot().DatabaseWrites
	if overOnePercent.TotalWrites != 101 || overOnePercent.FailedWrites != 2 ||
		!overOnePercent.ExceedsOnePercent {
		t.Fatalf("over-one-percent write rate = %+v", overOnePercent)
	}

	now = now.Add(RollbackObservationWindow + time.Second)
	expired := observations.Snapshot().DatabaseWrites
	if expired.TotalWrites != 0 || expired.FailedWrites != 0 || expired.FailureRatePercent != 0 || expired.ExceedsOnePercent {
		t.Fatalf("expired write window = %+v, want no retained samples", expired)
	}
}

func TestDatabaseWriteWindowIncludesBothBoundarySeconds(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordDatabaseWrite(true)

	now = now.Add(RollbackObservationWindow)
	observations.RecordDatabaseWrite(false)
	got := observations.Snapshot().DatabaseWrites
	if got.TotalWrites != 2 || got.FailedWrites != 1 {
		t.Fatalf("a logical write at the oldest included second disappeared from the window: %+v", got)
	}
}

func TestReusedDatabaseWriteBucketDiscardsExpiredCounts(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordDatabaseWrite(false)

	now = now.Add(RollbackObservationWindow + time.Second)
	observations.RecordDatabaseWrite(true)
	got := observations.Snapshot().DatabaseWrites
	if got.TotalWrites != 1 || got.FailedWrites != 0 || got.FailureRatePercent != 0 {
		t.Fatalf("an expired failure leaked into a reused database-write bucket: %+v", got)
	}
}

func TestDatabaseWriteWindowExcludesOutcomesAfterABackwardClockStep(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordDatabaseWrite(false)

	now = now.Add(-time.Second)
	observations.RecordDatabaseWrite(true)
	got := observations.Snapshot().DatabaseWrites
	if got.TotalWrites != 1 || got.FailedWrites != 0 {
		t.Fatalf("a future failed write contaminated the window after the clock moved backward: %+v", got)
	}
}

func TestDatabaseWriteObservationAcceptsAPreEpochClock(t *testing.T) {
	defer func() {
		if value := recover(); value != nil {
			t.Errorf("a pre-epoch clock crashed database-write observation: %v", value)
		}
	}()

	now := time.Unix(-1, 0)
	observations := NewRollbackObservations(func() time.Time { return now })
	observations.RecordDatabaseWrite(true)
	if got := observations.Snapshot().DatabaseWrites; got.TotalWrites != 1 || got.FailedWrites != 0 {
		t.Fatalf("pre-epoch database-write observation was lost: %+v", got)
	}
}

func TestDatabaseWriteThresholdTriggersAtAnyRateAboveOnePercent(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })
	for range 98 {
		observations.RecordDatabaseWrite(true)
	}
	observations.RecordDatabaseWrite(false)

	got := observations.Snapshot().DatabaseWrites
	if got.TotalWrites != 99 || got.FailedWrites != 1 || !got.ExceedsOnePercent {
		t.Fatalf("a database-write failure rate just above one percent did not trigger rollback review: %+v", got)
	}
}

func TestRollbackSnapshotReportsTheExactTenMinuteWindow(t *testing.T) {
	now := time.Date(2026, time.September, 3, 10, 0, 0, 0, time.UTC)
	observations := NewRollbackObservations(func() time.Time { return now })

	got := observations.Snapshot()
	wantStart := now.Add(-10 * time.Minute)
	if !got.ObservedAt.Equal(now) || !got.DatabaseWrites.WindowStart.Equal(wantStart) ||
		!got.DatabaseWrites.WindowEnd.Equal(now) {
		t.Fatalf("rollback reading no longer reports one exact ten-minute window ending at observation time: %+v", got)
	}
}
