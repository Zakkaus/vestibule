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
	loads    int
}

func (s *consoleAuditTestStore) LoadChallengeAudit(context.Context, int64) ([]ChallengeAuditRecord, error) {
	s.loads++
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

const consoleAuditCoverageChatID int64 = -1009000000704

func TestConsoleAuditRefusesInvalidReadParameters(t *testing.T) {
	service, store := newConsoleAuditCoverageService([]ChallengeAuditRecord{
		consoleAuditCoverageRecord("-1009000000704:101:valid", 101, 100, 9),
	})
	if _, err := service.ConsoleAudit(context.Background(), consoleAuditCoverageChatID, 9); err != nil {
		t.Fatalf("a valid audit read was refused: %v", err)
	}
	cases := []struct {
		name    string
		groupID int64
		actorID int64
		harm    string
	}{
		{
			name: "missing group", groupID: 0, actorID: 9,
			harm: "an audit read without a target group could expose history",
		},
		{
			name: "missing actor", groupID: consoleAuditCoverageChatID, actorID: 0,
			harm: "an audit read without an actor could advertise undo permissions",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := service.ConsoleAudit(context.Background(), testCase.groupID, testCase.actorID)
			if !errors.Is(err, ErrConsoleAuditInvalid) {
				t.Fatalf("%s: audit error=%v, want %v", testCase.harm, err, ErrConsoleAuditInvalid)
			}
		})
	}
	if store.loads != 1 {
		t.Fatalf("invalid audit reads reached the store %d additional times", store.loads-1)
	}
}

func TestConsoleAuditSortsEqualSettlementsByDescendingID(t *testing.T) {
	lowerID := "-1009000000704:104:alpha"
	higherID := "-1009000000704:105:beta"
	service, _ := newConsoleAuditCoverageService([]ChallengeAuditRecord{
		consoleAuditCoverageRecord(lowerID, 104, 100, 9),
		consoleAuditCoverageRecord(higherID, 105, 100, 9),
	})

	entries, err := service.ConsoleAudit(context.Background(), consoleAuditCoverageChatID, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != higherID || entries[1].ID != lowerID {
		t.Fatalf("equal-time audit order=%#v; unstable ties can repeat or skip history decisions", entries)
	}
}

func TestConsoleAuditWithholdsUndoForAmbiguousLatestSettlement(t *testing.T) {
	firstID := "-1009000000704:106:tie-a"
	secondID := "-1009000000704:106:tie-b"
	controlID := "-1009000000704:107:control"
	service, _ := newConsoleAuditCoverageService([]ChallengeAuditRecord{
		consoleAuditCoverageRecord(firstID, 106, 100, 9),
		consoleAuditCoverageRecord(secondID, 106, 100, 9),
		consoleAuditCoverageRecord(controlID, 107, 100, 9),
	})

	entries, err := service.ConsoleAudit(context.Background(), consoleAuditCoverageChatID, 9)
	if err != nil {
		t.Fatal(err)
	}
	states := consoleAuditUndoStates(entries)
	if states[firstID] != ConsoleUndoUnavailable || states[secondID] != ConsoleUndoUnavailable {
		t.Fatalf("ambiguous latest settlements offered an undo; either ban could lift the other: %#v", states)
	}
	if states[controlID] != ConsoleUndoAvailable {
		t.Fatalf("an unambiguous latest ban was not available as the positive control: %#v", states)
	}
}

func TestConsoleAuditPreservesRecordFieldsAndUndoLifecycle(t *testing.T) {
	declinedID := "-1009000000704:108:declined"
	pendingID := "-1009000000704:109:pending"
	failedID := "-1009000000704:110:failed"
	completedID := "-1009000000704:111:completed"
	declined := consoleAuditCoverageRecord(declinedID, 108, 1_700_000_000, 17)
	declined.Record.Name = "Applicant"
	declined.State = ChallengeDeclined
	declined.Reason = "wrong_answer"
	pending := consoleAuditCoverageRecord(pendingID, 109, 101, 9)
	pending.UndoAction = ChallengeActionPending
	failed := consoleAuditCoverageRecord(failedID, 110, 102, 9)
	failed.UndoAction = ChallengeActionFailed
	completed := consoleAuditCoverageRecord(completedID, 111, 103, 9)
	completed.UndoAction = ChallengeActionDone
	service, _ := newConsoleAuditCoverageService([]ChallengeAuditRecord{declined, pending, failed, completed})

	entries, err := service.ConsoleAudit(context.Background(), consoleAuditCoverageChatID, 9)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]ConsoleAuditEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	wantDeclined := ConsoleAuditEntry{
		ID: declinedID, GroupID: consoleAuditCoverageChatID, UserID: 108, Name: "Applicant",
		State: ChallengeDeclined, Reason: "wrong_answer",
		SettledAt: time.Unix(1_700_000_000, 0).UTC(), SettledBy: 17,
		UndoState: ConsoleUndoUnavailable,
	}
	if got, exists := byID[declinedID]; !exists || got != wantDeclined {
		t.Fatalf("audit projection lost who, why, or when the decision happened: got=%#v want=%#v", got, wantDeclined)
	}
	for _, testCase := range []struct {
		id   string
		want ConsoleUndoState
	}{
		{id: pendingID, want: ConsoleUndoPending},
		{id: failedID, want: ConsoleUndoFailed},
		{id: completedID, want: ConsoleUndoCompleted},
	} {
		if got := byID[testCase.id].UndoState; got != testCase.want {
			t.Fatalf("audit undo lifecycle for %s=%q, want %q", testCase.id, got, testCase.want)
		}
	}
}

func TestConsoleAuditDoesNotOfferUndoAfterFailedSettlement(t *testing.T) {
	failedID := "-1009000000704:112:failed-settlement"
	controlID := "-1009000000704:113:completed-settlement"
	failed := consoleAuditCoverageRecord(failedID, 112, 100, 9)
	failed.SettlementAction = ChallengeActionFailed
	control := consoleAuditCoverageRecord(controlID, 113, 101, 9)
	control.SettlementAction = ChallengeActionDone
	service, _ := newConsoleAuditCoverageService([]ChallengeAuditRecord{failed, control})

	entries, err := service.ConsoleAudit(context.Background(), consoleAuditCoverageChatID, 9)
	if err != nil {
		t.Fatal(err)
	}
	states := consoleAuditUndoStates(entries)
	if states[failedID] != ConsoleUndoUnavailable {
		t.Fatalf("a failed settlement offered an undo; it could lift a ban that never completed: %#v", states)
	}
	if states[controlID] != ConsoleUndoAvailable {
		t.Fatalf("a completed settlement was not undoable as the positive control: %#v", states)
	}
}

func newConsoleAuditCoverageService(records []ChallengeAuditRecord) (*Service, *consoleAuditTestStore) {
	service := newTestService(&settings.Config{GroupIDs: []int64{consoleAuditCoverageChatID}})
	store := &consoleAuditTestStore{records: records}
	service.stateStore = store
	return service, store
}

func consoleAuditCoverageRecord(
	id string,
	userID, settledAt, settledBy int64,
) ChallengeAuditRecord {
	return ChallengeAuditRecord{
		ID: id,
		Record: PendingRecord{
			GroupID: consoleAuditCoverageChatID,
			UserID:  userID,
			Name:    "Applicant",
		},
		State: ChallengeBanned, SettledAt: settledAt, SettledBy: settledBy,
	}
}

func consoleAuditUndoStates(entries []ConsoleAuditEntry) map[string]ConsoleUndoState {
	states := make(map[string]ConsoleUndoState, len(entries))
	for _, entry := range entries {
		states[entry.ID] = entry.UndoState
	}
	return states
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
