package verification_test

import (
	"cmp"
	"slices"
	"strings"

	v "github.com/Zakkaus/vestibule/internal/verification"
)

// Cancellation runs after the DB lease commits and before the worker installs its
// payload. No goroutines, sleeps, or mutations of production scheduler timestamps.
type chaosClaimStore struct {
	v.Store
	fixture *chaosFixture
}

func (s *chaosClaimStore) InsertPending(namespace string, record v.PendingRecord) (bool, error) {
	inserted, err := s.Store.InsertPending(namespace, record)
	if inserted {
		s.fixture.order[chaosID(record)] = len(s.fixture.order)
	}
	return inserted, err
}

func (s *chaosClaimStore) ClaimActions(namespace, owner string, now, until int64, limit int) ([]v.PendingAction, error) {
	actions, err := s.Store.ClaimActions(namespace, owner, now, until, limit)
	// Cryptographic nonce text must not choose the order of equal-time actions.
	slices.SortStableFunc(actions, func(a, b v.PendingAction) int {
		if order := cmp.Compare(a.NextTryAt, b.NextTryAt); order != 0 {
			return order
		}
		if order := cmp.Compare(s.fixture.order[a.ChallengeID], s.fixture.order[b.ChallengeID]); order != 0 {
			return order
		}
		return cmp.Compare(a.Kind, b.Kind)
	})
	if err != nil || s.fixture.interrupt == "" {
		return actions, err
	}
	for _, action := range actions {
		if !strings.HasPrefix(action.Kind, "settle_") {
			continue
		}
		var payload struct {
			Record v.PendingRecord `json:"record"`
		}
		chaosDecode(s.fixture.t, action.Payload, &payload)
		op := s.fixture.interrupt
		s.fixture.interrupt = ""
		s.fixture.cancel(op, payload.Record.GroupID)
		break
	}
	return actions, nil
}
