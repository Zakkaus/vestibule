// VerificationJSONStore is the retained legacy adapter; runtime state uses VerificationStore.
package database

import (
	"fmt"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/verification"
	"sync"
)

// VerificationJSONStore preserves the legacy atomic JSON write and corruption-recovery behavior.
// Its mutex makes each retained read-modify-write operation atomic within one process.
type VerificationJSONStore struct {
	pendingMu sync.Mutex
}

var _ verification.Store = (*VerificationJSONStore)(nil)

func NewVerificationJSONStore() *VerificationJSONStore { return &VerificationJSONStore{} }

func (s *VerificationJSONStore) LoadPending(path string) ([]verification.PendingRecord, error) {
	var records []verification.PendingRecord
	if err := store.Load(path, &records); err != nil {
		return nil, stateLoadError(err)
	}
	return records, nil
}

func (s *VerificationJSONStore) InsertPending(path string, record verification.PendingRecord) (bool, error) {
	return s.mutatePending(path, func(records *[]verification.PendingRecord) bool {
		for _, current := range *records {
			if current.GroupID == record.GroupID && current.UserID == record.UserID {
				return false
			}
		}
		*records = append(*records, record)
		return true
	})
}

func (s *VerificationJSONStore) UpdatePending(
	path string,
	expected verification.PendingRef,
	record verification.PendingRecord,
) (bool, error) {
	return s.mutatePending(path, func(records *[]verification.PendingRecord) bool {
		for i := range *records {
			if pendingRecordMatches((*records)[i], expected) {
				(*records)[i] = record
				return true
			}
		}
		return false
	})
}

func (s *VerificationJSONStore) TransitionChallenge(path string, transition verification.ChallengeTransition) (bool, error) {
	switch {
	case transition.From == verification.ChallengePending:
		return s.mutatePending(path, func(records *[]verification.PendingRecord) bool {
			for i := range *records {
				if pendingRecordMatches((*records)[i], transition.Expected) {
					*records = append((*records)[:i], (*records)[i+1:]...)
					return true
				}
			}
			return false
		})
	case transition.To == verification.ChallengePending:
		return s.InsertPending(path, transition.Record)
	default:
		// Legacy JSON represents only pending rows; a terminal-to-terminal transition has no file change.
		return true, nil
	}
}

func (s *VerificationJSONStore) DeletePending(path string, expected verification.PendingRef) (bool, error) {
	return s.mutatePending(path, func(records *[]verification.PendingRecord) bool {
		for i := range *records {
			if pendingRecordMatches((*records)[i], expected) {
				*records = append((*records)[:i], (*records)[i+1:]...)
				return true
			}
		}
		return false
	})
}

func (s *VerificationJSONStore) mutatePending(
	path string,
	mutate func(*[]verification.PendingRecord) bool,
) (bool, error) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	records, err := s.LoadPending(path)
	if err != nil {
		return false, err
	}
	if !mutate(&records) {
		return false, nil
	}
	if err = store.Write(path, records); err != nil {
		return false, err
	}
	return true, nil
}

func pendingRecordMatches(record verification.PendingRecord, expected verification.PendingRef) bool {
	return record.GroupID == expected.GroupID && record.UserID == expected.UserID &&
		record.Nonce == expected.Nonce && record.Epoch == expected.Epoch
}

func (s *VerificationJSONStore) LoadFailures(path string) ([]verification.FailureRecord, error) {
	var records []verification.FailureRecord
	if err := store.Load(path, &records); err != nil {
		return nil, stateLoadError(err)
	}
	return records, nil
}

func (s *VerificationJSONStore) SaveFailures(path string, snapshot func() []verification.FailureRecord) error {
	return store.Save(path, func() any { return snapshot() })
}

func (s *VerificationJSONStore) LoadAgents(path string) (verification.AgentTally, error) {
	var tally verification.AgentTally
	if err := store.Load(path, &tally); err != nil {
		return verification.AgentTally{}, stateLoadError(err)
	}
	return tally, nil
}

func (s *VerificationJSONStore) SaveAgents(path string, snapshot func() verification.AgentTally) error {
	return store.Save(path, func() any { return snapshot() })
}

func (s *VerificationJSONStore) LoadHeartbeat(path string) (verification.HeartbeatRecord, error) {
	var heartbeat verification.HeartbeatRecord
	if err := store.Load(path, &heartbeat); err != nil {
		return verification.HeartbeatRecord{}, stateLoadError(err)
	}
	return heartbeat, nil
}

func (s *VerificationJSONStore) SaveHeartbeat(path string, heartbeat verification.HeartbeatRecord) error {
	return store.Write(path, heartbeat)
}

func stateLoadError(err error) error {
	if store.ReadFailed(err) {
		return fmt.Errorf("%w: %v", verification.ErrStoreReadOnly, err)
	}
	return err
}
