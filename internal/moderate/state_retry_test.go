package moderate

import (
	"errors"
	"testing"
	"time"
)

type flakyWarningStore struct {
	failures  int
	calls     int
	saved     []WarningRecord
	callTimes []time.Time
}

func (s *flakyWarningStore) LoadWarnings() ([]WarningRecord, error) {
	return nil, nil
}

func (s *flakyWarningStore) SaveWarnings(snapshot func() []WarningRecord) error {
	s.calls++
	s.callTimes = append(s.callTimes, time.Now())
	records := snapshot()
	if s.calls <= s.failures {
		return errors.New("temporary warning write failure")
	}
	s.saved = records
	return nil
}

func TestWarningSnapshotMakesThreeAttemptsBeforeGivingUp(t *testing.T) {
	t.Run("two transient failures recover", func(t *testing.T) {
		store := &flakyWarningStore{failures: 2}
		state := newWarningState(store)
		state.increment(stateCompatGroupA, 7)

		if err := state.save(); err != nil {
			t.Fatalf("warning snapshot was abandoned after two transient failures, so a member's warning count would be lost on restart: %v", err)
		}
		if store.calls != 3 {
			t.Fatalf("warning snapshot writes = %d, want 3", store.calls)
		}
		if state.counters[warningKey{stateCompatGroupA, 7}] != 1 || len(store.saved) != 1 || store.saved[0].Count != 1 {
			t.Fatalf("warning count after retry = local %d stored %#v", state.counters[warningKey{stateCompatGroupA, 7}], store.saved)
		}
	})

	t.Run("three failures give up", func(t *testing.T) {
		store := &flakyWarningStore{failures: 3}
		state := newWarningState(store)
		state.increment(stateCompatGroupA, 8)

		if err := state.save(); err == nil {
			t.Fatal("warning snapshot did not give up after three failed writes; a failing store would be hammered")
		}
		if store.calls != 3 {
			t.Fatalf("warning snapshot made %d write attempts before giving up, want 3; fewer attempts can discard a member's warning count", store.calls)
		}
	})
}

func TestWarningSnapshotBacksOffBetweenFailedAttempts(t *testing.T) {
	const minimumBackoff = 8 * time.Millisecond

	store := &flakyWarningStore{failures: 2}
	state := newWarningState(store)
	state.increment(stateCompatGroupB, 7)
	if err := state.save(); err != nil {
		t.Fatalf("warning snapshot did not recover after transient failures: %v", err)
	}
	if len(store.callTimes) != 3 {
		t.Fatalf("warning snapshot writes = %d, want 3", len(store.callTimes))
	}
	for attempt := 1; attempt < len(store.callTimes); attempt++ {
		backoff := store.callTimes[attempt].Sub(store.callTimes[attempt-1])
		if backoff < minimumBackoff {
			t.Fatalf("warning snapshot retried after %s without backoff after failed attempt %d; a failing store would be hammered", backoff, attempt)
		}
	}
}
