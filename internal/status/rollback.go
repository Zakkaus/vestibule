package status

import (
	"sync"
	"time"
)

// RollbackObservationWindow is the fixed duration used for sustained-failure and write-rate readings.
const RollbackObservationWindow = 10 * time.Minute

const rollbackWriteBucketCount = int(RollbackObservationWindow/time.Second) + 1

// RollbackObservations keeps the process-local measurements used to assess cutover rollback conditions.
type RollbackObservations struct {
	now func() time.Time

	mu                sync.Mutex
	challengeDelivery deliveryProblemStreak
	consoleAccess     problemStreak
	databaseWrites    writeOutcomes
}

// RollbackSnapshot is one consistent reading of cutover-related observations.
type RollbackSnapshot struct {
	ObservedAt        time.Time
	ChallengeDelivery ChallengeDeliverySnapshot
	ConsoleAccess     ProblemStreakSnapshot
	DatabaseWrites    DatabaseWriteSnapshot
}

// ProblemStreakSnapshot describes an unresolved sequence of failures. A successful observation
// ends the sequence, so a single old failure cannot look like a sustained outage.
type ProblemStreakSnapshot struct {
	FirstProblemAt  *time.Time
	LastProblemAt   *time.Time
	LastRecoveredAt *time.Time
	ProblemEvents   uint64
	ObservedFor     time.Duration
	ExceedsWindow   bool
}

// ChallengeDeliverySnapshot distinguishes failed deliveries from repeated challenge deliveries.
type ChallengeDeliverySnapshot struct {
	ProblemStreakSnapshot
	FailedDeliveries    uint64
	DuplicateDeliveries uint64
}

// DatabaseWriteSnapshot reports logical writes after retryStoreWrite has exhausted or completed its retry policy.
type DatabaseWriteSnapshot struct {
	WindowStart        time.Time
	WindowEnd          time.Time
	TotalWrites        uint64
	FailedWrites       uint64
	FailureRatePercent float64
	ExceedsOnePercent  bool
}

// NewRollbackObservations constructs a process-local recorder. The clock is injectable for deterministic tests.
func NewRollbackObservations(now func() time.Time) *RollbackObservations {
	if now == nil {
		now = time.Now
	}
	return &RollbackObservations{now: now}
}

// RecordChallengeDeliverySuccess ends a current delivery problem sequence.
func (o *RollbackObservations) RecordChallengeDeliverySuccess() {
	o.recordChallengeDelivery(o.now().UTC(), false, false)
}

// RecordChallengeDeliveryFailure records one verification challenge whose delivery did not complete.
func (o *RollbackObservations) RecordChallengeDeliveryFailure() {
	o.recordChallengeDelivery(o.now().UTC(), true, false)
}

// RecordChallengeDeliveryDuplicate records a repeated verification challenge delivery.
func (o *RollbackObservations) RecordChallengeDeliveryDuplicate() {
	o.recordChallengeDelivery(o.now().UTC(), false, true)
}

func (o *RollbackObservations) recordChallengeDelivery(at time.Time, failed, duplicate bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !failed && !duplicate {
		o.challengeDelivery.recover(at)
		return
	}
	o.challengeDelivery.record(at, failed, duplicate)
}

// RecordConsoleAccessUnavailable records a failed group-access verification for the console.
func (o *RollbackObservations) RecordConsoleAccessUnavailable() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.consoleAccess.record(o.now().UTC())
}

// RecordConsoleAccessVerified ends a current console-access problem sequence.
func (o *RollbackObservations) RecordConsoleAccessVerified() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.consoleAccess.recover(o.now().UTC())
}

// RecordDatabaseWrite records one logical retryStoreWrite outcome.
func (o *RollbackObservations) RecordDatabaseWrite(success bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.databaseWrites.record(o.now().UTC(), !success)
}

// Snapshot returns all current observations against the same clock reading.
func (o *RollbackObservations) Snapshot() RollbackSnapshot {
	at := o.now().UTC()
	o.mu.Lock()
	defer o.mu.Unlock()
	return RollbackSnapshot{
		ObservedAt:        at,
		ChallengeDelivery: o.challengeDelivery.snapshot(at),
		ConsoleAccess:     o.consoleAccess.snapshot(at),
		DatabaseWrites:    o.databaseWrites.snapshot(at),
	}
}

type problemStreak struct {
	firstProblemAt  time.Time
	lastProblemAt   time.Time
	lastRecoveredAt time.Time
	problemEvents   uint64
}

func (s *problemStreak) record(at time.Time) {
	if s.firstProblemAt.IsZero() {
		s.firstProblemAt = at
	}
	s.lastProblemAt = at
	s.problemEvents++
}

func (s *problemStreak) recover(at time.Time) {
	if s.firstProblemAt.IsZero() {
		return
	}
	s.firstProblemAt = time.Time{}
	s.lastProblemAt = time.Time{}
	s.problemEvents = 0
	s.lastRecoveredAt = at
}

func (s problemStreak) snapshot(at time.Time) ProblemStreakSnapshot {
	snapshot := ProblemStreakSnapshot{
		LastRecoveredAt: timePointer(s.lastRecoveredAt),
	}
	if s.firstProblemAt.IsZero() {
		return snapshot
	}
	observedFor := s.lastProblemAt.Sub(s.firstProblemAt)
	if observedFor < 0 {
		observedFor = 0
	}
	snapshot.FirstProblemAt = timePointer(s.firstProblemAt)
	snapshot.LastProblemAt = timePointer(s.lastProblemAt)
	snapshot.ProblemEvents = s.problemEvents
	snapshot.ObservedFor = observedFor
	snapshot.ExceedsWindow = observedFor > RollbackObservationWindow
	return snapshot
}

type deliveryProblemStreak struct {
	problemStreak
	failedDeliveries    uint64
	duplicateDeliveries uint64
}

func (s *deliveryProblemStreak) record(at time.Time, failed, duplicate bool) {
	s.problemStreak.record(at)
	if failed {
		s.failedDeliveries++
	}
	if duplicate {
		s.duplicateDeliveries++
	}
}

func (s *deliveryProblemStreak) recover(at time.Time) {
	s.problemStreak.recover(at)
	s.failedDeliveries = 0
	s.duplicateDeliveries = 0
}

func (s deliveryProblemStreak) snapshot(at time.Time) ChallengeDeliverySnapshot {
	return ChallengeDeliverySnapshot{
		ProblemStreakSnapshot: s.problemStreak.snapshot(at),
		FailedDeliveries:      s.failedDeliveries,
		DuplicateDeliveries:   s.duplicateDeliveries,
	}
}

type writeOutcomeBucket struct {
	second   int64
	writes   uint64
	failures uint64
}

type writeOutcomes struct {
	buckets [rollbackWriteBucketCount]writeOutcomeBucket
}

func (w *writeOutcomes) record(at time.Time, failed bool) {
	second := at.Unix()
	slot := second % int64(len(w.buckets))
	if slot < 0 {
		slot += int64(len(w.buckets))
	}
	bucket := &w.buckets[slot]
	if bucket.second != second {
		*bucket = writeOutcomeBucket{second: second}
	}
	bucket.writes++
	if failed {
		bucket.failures++
	}
}

func (w writeOutcomes) snapshot(at time.Time) DatabaseWriteSnapshot {
	windowStart := at.Add(-RollbackObservationWindow)
	startSecond := windowStart.Unix()
	endSecond := at.Unix()
	snapshot := DatabaseWriteSnapshot{WindowStart: windowStart, WindowEnd: at}
	for _, bucket := range w.buckets {
		if bucket.second < startSecond || bucket.second > endSecond {
			continue
		}
		snapshot.TotalWrites += bucket.writes
		snapshot.FailedWrites += bucket.failures
	}
	if snapshot.TotalWrites == 0 {
		return snapshot
	}
	snapshot.FailureRatePercent = 100 * float64(snapshot.FailedWrites) / float64(snapshot.TotalWrites)
	snapshot.ExceedsOnePercent = snapshot.FailedWrites*100 > snapshot.TotalWrites
	return snapshot
}

func timePointer(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	copy := at.UTC()
	return &copy
}
