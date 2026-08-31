// VerificationJSONStore is the retained legacy adapter; runtime state uses VerificationStore.
package database

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/verification"
)

// VerificationJSONStore preserves the legacy atomic JSON write and corruption-recovery behavior.
// Runtime state uses VerificationStore; the in-memory action map only keeps legacy embeddings
// compiling while their caller remains alive.
type VerificationJSONStore struct {
	pendingMu sync.Mutex
	actionMu  sync.Mutex
	actions   map[string]jsonPendingAction
}

type jsonPendingAction struct {
	verification.PendingAction
	state      string
	claimOwner string
	claimUntil int64
}

var _ verification.Store = (*VerificationJSONStore)(nil)

func NewVerificationJSONStore() *VerificationJSONStore {
	return &VerificationJSONStore{actions: make(map[string]jsonPendingAction)}
}

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
	var changed bool
	var err error
	switch {
	case transition.From == verification.ChallengePending:
		changed, err = s.mutatePending(path, func(records *[]verification.PendingRecord) bool {
			for i := range *records {
				if pendingRecordMatches((*records)[i], transition.Expected) {
					*records = append((*records)[:i], (*records)[i+1:]...)
					return true
				}
			}
			return false
		})
	case transition.To == verification.ChallengePending:
		changed, err = s.InsertPending(path, transition.Record)
	default:
		changed = true // Legacy JSON represents only pending rows; terminal rows have no file change.
	}
	if err != nil || !changed {
		return changed, err
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.ensureActionsLocked()
	for _, intent := range transition.Actions {
		if _, exists := s.actions[intent.ID]; exists {
			return false, fmt.Errorf("duplicate legacy action id %q", intent.ID)
		}
		s.actions[intent.ID] = jsonPendingAction{PendingAction: verification.PendingAction{
			ActionIntent: intent, ChallengeID: strconv.FormatInt(transition.Expected.GroupID, 10) + ":" +
				strconv.FormatInt(transition.Expected.UserID, 10) + ":" + transition.Expected.Nonce,
		}, state: "pending", claimOwner: intent.ClaimOwner, claimUntil: intent.ClaimUntil}
	}
	return true, nil
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

func (s *VerificationJSONStore) ClaimExpired(path string, now, claimUntil int64, limit int) ([]verification.PendingRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	if claimUntil <= now {
		return nil, fmt.Errorf("expiry claim ends at %d, not after %d", claimUntil, now)
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	records, err := s.LoadPending(path)
	if err != nil {
		return nil, err
	}
	claimed := make([]verification.PendingRecord, 0, limit)
	for i := range records {
		record := &records[i]
		if len(claimed) == limit || record.Deadline > now {
			continue
		}
		if record.Epoch >= 1<<63-1 {
			return nil, fmt.Errorf("due challenge for chat %d user %d exhausted epoch", record.GroupID, record.UserID)
		}
		record.Epoch++
		record.Deadline = claimUntil
		claimed = append(claimed, *record)
	}
	if len(claimed) == 0 {
		return nil, nil
	}
	if err := store.Save(path, func() any { return records }); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *VerificationJSONStore) ClaimActions(
	_ string,
	owner string,
	now, claimUntil int64,
	limit int,
) ([]verification.PendingAction, error) {
	if limit <= 0 {
		return nil, nil
	}
	if strings.TrimSpace(owner) == "" || claimUntil <= now {
		return nil, fmt.Errorf("invalid legacy action claim")
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.ensureActionsLocked()
	claimed := make([]verification.PendingAction, 0, limit)
	for id, action := range s.actions {
		if len(claimed) == limit {
			break
		}
		if action.state != "pending" || action.NextTryAt > now || action.claimUntil > now {
			continue
		}
		action.claimOwner = owner
		action.claimUntil = claimUntil
		s.actions[id] = action
		claimed = append(claimed, action.PendingAction)
	}
	return claimed, nil
}

func (s *VerificationJSONStore) CompleteAction(
	_ string,
	id, owner string,
	_ int64,
	followups []verification.ActionIntent,
) (bool, error) {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.ensureActionsLocked()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	for _, intent := range followups {
		if _, exists := s.actions[intent.ID]; exists {
			return false, fmt.Errorf("duplicate legacy action id %q", intent.ID)
		}
		s.actions[intent.ID] = jsonPendingAction{PendingAction: verification.PendingAction{
			ActionIntent: intent, ChallengeID: action.ChallengeID,
		}, state: "pending"}
	}
	action.state, action.claimOwner, action.claimUntil = "done", "", 0
	s.actions[id] = action
	return true, nil
}

func (s *VerificationJSONStore) RetryAction(
	_ string,
	id, owner string,
	attempts int,
	nextTryAt int64,
	_ string,
) (bool, error) {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	action.Attempts, action.NextTryAt = attempts, nextTryAt
	action.claimOwner, action.claimUntil = "", 0
	s.actions[id] = action
	return true, nil
}

func (s *VerificationJSONStore) FailAction(
	_ string,
	id, owner string,
	_ int64,
	_ string,
) (bool, error) {
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	action.state, action.claimOwner, action.claimUntil = "failed", "", 0
	s.actions[id] = action
	return true, nil
}

func (s *VerificationJSONStore) ensureActionsLocked() {
	if s.actions == nil {
		s.actions = make(map[string]jsonPendingAction)
	}
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
