package moderate

import (
	"errors"
	"testing"
)

type flakyWarningStore struct {
	failures int
	calls    int
	saved    []WarningRecord
}

func (s *flakyWarningStore) LoadWarnings() ([]WarningRecord, error) {
	return nil, nil
}

func (s *flakyWarningStore) SaveWarnings(snapshot func() []WarningRecord) error {
	s.calls++
	records := snapshot()
	if s.calls <= s.failures {
		return errors.New("temporary warning write failure")
	}
	s.saved = records
	return nil
}

func TestWarningSnapshotRetriesWithoutDroppingLocalCount(t *testing.T) {
	store := &flakyWarningStore{failures: warningWriteMaxAttempts - 1}
	state := newWarningState(store)
	if err := state.load(); err != nil {
		t.Fatal(err)
	}
	state.increment(-100, 7)
	if err := state.save(); err != nil {
		t.Fatal(err)
	}

	t.Logf("warning writes=%d local_count=%d stored=%#v", store.calls, state.counters[warningKey{-100, 7}], store.saved)
	if store.calls != warningWriteMaxAttempts {
		t.Fatalf("warning snapshot writes = %d, want %d", store.calls, warningWriteMaxAttempts)
	}
	if state.counters[warningKey{-100, 7}] != 1 || len(store.saved) != 1 || store.saved[0].Count != 1 {
		t.Fatalf("warning count after retry = local %d stored %#v", state.counters[warningKey{-100, 7}], store.saved)
	}
}
