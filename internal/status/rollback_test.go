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
