package verification

import (
	"fmt"
	"sync"

	"github.com/Zakkaus/vestibule/internal/store"
)

type testVerificationStore struct{}

var testPendingWriteMu sync.Mutex

func (testVerificationStore) LoadPending(path string) ([]PendingRecord, error) {
	var records []PendingRecord
	if err := store.Load(path, &records); err != nil {
		return nil, testStoreLoadError(err)
	}
	return records, nil
}

func (testVerificationStore) InsertPending(path string, record PendingRecord) (bool, error) {
	return mutateTestPending(path, func(records *[]PendingRecord) bool {
		for _, current := range *records {
			if current.GroupID == record.GroupID && current.UserID == record.UserID {
				return false
			}
		}
		*records = append(*records, record)
		return true
	})
}

func (testVerificationStore) UpdatePending(path string, expected PendingRef, record PendingRecord) (bool, error) {
	return mutateTestPending(path, func(records *[]PendingRecord) bool {
		for i := range *records {
			if testPendingMatches((*records)[i], expected) {
				(*records)[i] = record
				return true
			}
		}
		return false
	})
}

func (testVerificationStore) TransitionChallenge(path string, transition ChallengeTransition) (bool, error) {
	switch {
	case transition.From == ChallengePending:
		return mutateTestPending(path, func(records *[]PendingRecord) bool {
			for i := range *records {
				if testPendingMatches((*records)[i], transition.Expected) {
					*records = append((*records)[:i], (*records)[i+1:]...)
					return true
				}
			}
			return false
		})
	case transition.To == ChallengePending:
		return (testVerificationStore{}).InsertPending(path, transition.Record)
	default:
		return true, nil
	}
}

func (testVerificationStore) DeletePending(path string, expected PendingRef) (bool, error) {
	return mutateTestPending(path, func(records *[]PendingRecord) bool {
		for i := range *records {
			if testPendingMatches((*records)[i], expected) {
				*records = append((*records)[:i], (*records)[i+1:]...)
				return true
			}
		}
		return false
	})
}

func mutateTestPending(path string, mutate func(*[]PendingRecord) bool) (bool, error) {
	testPendingWriteMu.Lock()
	defer testPendingWriteMu.Unlock()
	var records []PendingRecord
	if err := store.Load(path, &records); err != nil {
		return false, testStoreLoadError(err)
	}
	if !mutate(&records) {
		return false, nil
	}
	if err := store.Write(path, records); err != nil {
		return false, err
	}
	return true, nil
}

func testPendingMatches(record PendingRecord, expected PendingRef) bool {
	return record.GroupID == expected.GroupID && record.UserID == expected.UserID &&
		record.Nonce == expected.Nonce && record.Epoch == expected.Epoch
}

type zeroTransitionStore struct {
	testVerificationStore
	calls int
}

func (s *zeroTransitionStore) TransitionChallenge(string, ChallengeTransition) (bool, error) {
	s.calls++
	return false, nil
}

func (s *zeroTransitionStore) UpdatePending(string, PendingRef, PendingRecord) (bool, error) {
	return true, nil
}

func (testVerificationStore) LoadFailures(path string) ([]FailureRecord, error) {
	var records []FailureRecord
	if err := store.Load(path, &records); err != nil {
		return nil, testStoreLoadError(err)
	}
	return records, nil
}

func (testVerificationStore) SaveFailures(path string, snapshot func() []FailureRecord) error {
	return store.Save(path, func() any { return snapshot() })
}

func (testVerificationStore) LoadAgents(path string) (AgentTally, error) {
	var tally AgentTally
	if err := store.Load(path, &tally); err != nil {
		return AgentTally{}, testStoreLoadError(err)
	}
	return tally, nil
}

func (testVerificationStore) SaveAgents(path string, snapshot func() AgentTally) error {
	return store.Save(path, func() any { return snapshot() })
}

func (testVerificationStore) LoadHeartbeat(path string) (HeartbeatRecord, error) {
	var heartbeat HeartbeatRecord
	if err := store.Load(path, &heartbeat); err != nil {
		return HeartbeatRecord{}, testStoreLoadError(err)
	}
	return heartbeat, nil
}

func (testVerificationStore) SaveHeartbeat(path string, heartbeat HeartbeatRecord) error {
	return store.Write(path, heartbeat)
}

func testStoreLoadError(err error) error {
	if store.ReadFailed(err) {
		return fmt.Errorf("%w: %v", ErrStoreReadOnly, err)
	}
	return err
}
