package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type consoleAuditTestStore struct {
	testVerificationStore
	records  []ChallengeAuditRecord
	action   ActionIntent
	enqueues int
}

func (s *consoleAuditTestStore) LoadChallengeAudit(context.Context, int64) ([]ChallengeAuditRecord, error) {
	return append([]ChallengeAuditRecord(nil), s.records...), nil
}

func (s *consoleAuditTestStore) EnqueueChallengeUndo(
	_ context.Context,
	expected ChallengeAuditRecord,
	action ActionIntent,
) (bool, error) {
	for i := range s.records {
		if s.records[i].ID != expected.ID || s.records[i].UndoAction != ChallengeActionNone {
			continue
		}
		s.records[i].UndoAction = ChallengeActionPending
		s.action = action
		s.enqueues++
		return true, nil
	}
	return false, nil
}

func (s *consoleAuditTestStore) CompleteAction(
	_ string,
	id, owner string,
	_ int64,
	_ []ActionIntent,
) (bool, error) {
	if id != s.action.ID || owner != s.action.ClaimOwner {
		return false, nil
	}
	for i := range s.records {
		if s.records[i].ID == "-100:42:current" {
			s.records[i].UndoAction = ChallengeActionDone
			return true, nil
		}
	}
	return false, nil
}

func TestConsoleAuditExposesHistoryAndNarrowUndoState(t *testing.T) {
	service, store, _ := newConsoleAuditTestService()
	entries, err := service.ConsoleAudit(context.Background(), -100, 9)
	if err != nil {
		t.Fatal(err)
	}
	states := make(map[string]ConsoleUndoState, len(entries))
	for _, entry := range entries {
		states[entry.ID] = entry.UndoState
	}
	if len(entries) != 5 || entries[0].ID != "-100:45:pending-settlement" ||
		states["-100:42:current"] != ConsoleUndoAvailable ||
		states["-100:42:old"] != ConsoleUndoUnavailable ||
		states["-100:43:other-actor"] != ConsoleUndoUnavailable ||
		states["-100:45:pending-settlement"] != ConsoleUndoUnavailable {
		t.Fatalf("console audit entries = %#v", entries)
	}
	if store.enqueues != 0 {
		t.Fatalf("read enqueued %d actions", store.enqueues)
	}
}

func TestUndoConsoleAuditDurablyUnbansLatestDecisionFromSameActor(t *testing.T) {
	service, store, gateway := newConsoleAuditTestService()
	entry, err := service.UndoConsoleAudit(context.Background(), ConsoleAuditUndo{
		ID: "-100:42:current", GroupID: -100, ActorID: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.UndoState != ConsoleUndoCompleted || store.enqueues != 1 ||
		store.action.Kind != actionUndoBan || gateway.unbans != 1 ||
		len(gateway.unbanOnlyIfBanned) != 1 || !gateway.unbanOnlyIfBanned[0] ||
		gateway.unbanned[0] != [2]int64{-100, 42} {
		t.Fatalf("undo entry=%#v action=%#v unbans=%#v only_if_banned=%#v",
			entry, store.action, gateway.unbanned, gateway.unbanOnlyIfBanned)
	}
	_, err = service.UndoConsoleAudit(context.Background(), ConsoleAuditUndo{
		ID: "-100:42:current", GroupID: -100, ActorID: 9,
	})
	if !errors.Is(err, ErrConsoleAuditNotUndoable) || gateway.unbans != 1 {
		t.Fatalf("repeated undo error=%v unbans=%d", err, gateway.unbans)
	}
}

// A ban that has been overtaken must not be liftable. Undoing -100:42:old would
// unban a user who was banned again afterwards, and the two guards that prevent
// it — the actor-side isLatest and the SQL's NOT EXISTS newer — could both be
// removed with the suite still green, which is what a property nobody tests
// looks like.
func TestUndoConsoleAuditRejectsASupersededDecision(t *testing.T) {
	service, store, gateway := newConsoleAuditTestService()
	_, err := service.UndoConsoleAudit(context.Background(), ConsoleAuditUndo{
		ID: "-100:42:old", GroupID: -100, ActorID: 9,
	})
	if !errors.Is(err, ErrConsoleAuditNotUndoable) || store.enqueues != 0 || gateway.unbans != 0 {
		t.Fatalf("superseded undo error=%v enqueues=%d unbans=%d", err, store.enqueues, gateway.unbans)
	}
}

func TestUndoConsoleAuditRejectsDifferentActorBeforeAction(t *testing.T) {
	service, store, gateway := newConsoleAuditTestService()
	_, err := service.UndoConsoleAudit(context.Background(), ConsoleAuditUndo{
		ID: "-100:43:other-actor", GroupID: -100, ActorID: 9,
	})
	if !errors.Is(err, ErrConsoleAuditNotUndoable) || store.enqueues != 0 || gateway.unbans != 0 {
		t.Fatalf("different-actor undo error=%v enqueues=%d unbans=%d", err, store.enqueues, gateway.unbans)
	}
}

func newConsoleAuditTestService() (*Service, *consoleAuditTestStore, *fakeVerifyBot) {
	gateway := newFakeVerifyBot()
	service := newTestService(&settings.Config{})
	service.gateway = gateway
	service.timeNow = func() time.Time { return time.Unix(500, 0) }
	store := &consoleAuditTestStore{records: []ChallengeAuditRecord{
		{
			ID: "-100:42:old", Record: PendingRecord{GroupID: -100, UserID: 42, Nonce: "old", Epoch: 1},
			State: ChallengeBanned, SettledAt: 90, SettledBy: 9,
		},
		{
			ID: "-100:42:current", Record: PendingRecord{GroupID: -100, UserID: 42, Nonce: "current", Epoch: 2},
			State: ChallengeBanned, SettledAt: 100, SettledBy: 9, SettlementAction: ChallengeActionDone,
		},
		{
			ID: "-100:43:other-actor", Record: PendingRecord{GroupID: -100, UserID: 43, Nonce: "other-actor", Epoch: 1},
			State: ChallengeBanned, SettledAt: 110, SettledBy: 10,
		},
		{
			ID: "-100:44:declined", Record: PendingRecord{GroupID: -100, UserID: 44, Nonce: "declined", Epoch: 1},
			State: ChallengeDeclined, Reason: "wrong_answer", SettledAt: 120, SettledBy: 9,
		},
		{
			ID: "-100:45:pending-settlement", Record: PendingRecord{GroupID: -100, UserID: 45, Nonce: "pending-settlement", Epoch: 1},
			State: ChallengeBanned, SettledAt: 130, SettledBy: 9, SettlementAction: ChallengeActionPending,
		},
	}}
	service.stateStore = store
	return service, store, gateway
}
