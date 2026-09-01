package database

import (
	"context"
	"reflect"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestChallengeAuditLoadsTerminalHistoryAndPersistsOneUndo(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	banned := verification.PendingRecord{
		GroupID: -100, UserID: 42, Name: "Applicant", Nonce: "banned", Deadline: 90, Epoch: 2,
	}
	requireAuditTransition(t, state, banned, verification.ChallengeBanned, "", 100, 9,
		verification.ActionIntent{
			ID: "settle-ban", Kind: "settle_ban", Payload: `{}`, NextTryAt: 100,
			ClaimOwner: "settler", ClaimUntil: 130,
		})
	requireAuditActionCompletion(t, state, "settle-ban", "settler", 101)
	declined := verification.PendingRecord{
		GroupID: -100, UserID: 43, Name: "Rejected", Nonce: "declined", Deadline: 95, Epoch: 1,
	}
	requireAuditTransition(t, state, declined, verification.ChallengeDeclined, "wrong_answer", 110, 9)
	pending := verification.PendingRecord{
		GroupID: -100, UserID: 44, Name: "Still pending", Nonce: "pending", Deadline: 200, Epoch: 1,
	}
	requirePendingInsert(t, state, pending)

	records, err := state.LoadChallengeAudit(context.Background(), -100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("challenge audit records = %#v, want two", records)
	}
	gotStates := [][6]any{
		{records[0].State, records[0].Reason, records[0].SettledBy,
			records[0].SettlementAction, records[0].UndoAction, records[0].Record.Name},
		{records[1].State, records[1].Reason, records[1].SettledBy,
			records[1].SettlementAction, records[1].UndoAction, records[1].Record.Name},
	}
	wantStates := [][6]any{
		{verification.ChallengeDeclined, "wrong_answer", int64(9),
			verification.ChallengeActionNone, verification.ChallengeActionNone, "Rejected"},
		{verification.ChallengeBanned, "", int64(9),
			verification.ChallengeActionDone, verification.ChallengeActionNone, "Applicant"},
	}
	if !reflect.DeepEqual(gotStates, wantStates) {
		t.Fatalf("challenge audit states = %#v, want %#v", gotStates, wantStates)
	}

	expected := records[1]
	wrongActor := expected
	wrongActor.SettledBy = 10
	undo := verification.ActionIntent{
		ID: "undo:-100:42:banned:ban", Kind: "undo_ban", Payload: `{"chat_id":-100,"user_id":42}`,
		NextTryAt: 120, ClaimOwner: "operator", ClaimUntil: 150,
	}
	wrongUndo := undo
	wrongUndo.ID = "undo-wrong"
	requireChallengeUndo(t, state, wrongActor, wrongUndo, false)
	requireChallengeUndo(t, state, expected, undo, true)
	requireChallengeUndo(t, state, expected, undo, false)
	records, err = state.LoadChallengeAudit(context.Background(), -100)
	if err != nil {
		t.Fatal(err)
	}
	if records[1].UndoAction != verification.ChallengeActionPending {
		t.Fatalf("pending undo audit = %#v", records)
	}
}

func TestChallengeAuditRejectsUndoAfterNewerDecision(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)
	old := verification.PendingRecord{GroupID: -100, UserID: 42, Nonce: "old", Deadline: 90, Epoch: 1}
	requireAuditTransition(t, state, old, verification.ChallengeBanned, "", 100, 9)
	current := verification.PendingRecord{GroupID: -100, UserID: 42, Nonce: "current", Deadline: 190, Epoch: 2}
	requireAuditTransition(t, state, current, verification.ChallengeApproved, "", 200, 9)
	records, err := state.LoadChallengeAudit(context.Background(), -100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("challenge audit records = %#v, want two", records)
	}
	requireChallengeUndo(t, state, records[1], verification.ActionIntent{
		ID: "undo-old", Kind: "undo_ban", Payload: `{"chat_id":-100,"user_id":42}`,
		NextTryAt: 210, ClaimOwner: "operator", ClaimUntil: 240,
	}, false)
}

func requireAuditActionCompletion(
	t *testing.T,
	state *VerificationStore,
	id, owner string,
	completedAt int64,
) {
	t.Helper()
	completed, err := state.CompleteAction("ignored", id, owner, completedAt, nil)
	if err != nil || !completed {
		t.Fatalf("complete audit action = %t, %v", completed, err)
	}
}

func requireChallengeUndo(
	t *testing.T,
	state *VerificationStore,
	expected verification.ChallengeAuditRecord,
	action verification.ActionIntent,
	want bool,
) {
	t.Helper()
	inserted, err := state.EnqueueChallengeUndo(context.Background(), expected, action)
	if err != nil {
		t.Fatal(err)
	}
	if inserted != want {
		t.Fatalf("enqueue challenge undo = %t, want %t", inserted, want)
	}
}

func requireAuditTransition(
	t *testing.T,
	state *VerificationStore,
	record verification.PendingRecord,
	to verification.ChallengeState,
	reason string,
	settledAt, settledBy int64,
	actions ...verification.ActionIntent,
) {
	t.Helper()
	requirePendingInsert(t, state, record)
	changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: record.Ref(), Record: record, From: verification.ChallengePending, To: to,
		Reason: reason, SettledAt: settledAt, SettledBy: settledBy, Actions: actions,
	})
	if err != nil || !changed {
		t.Fatalf("settle challenge %s = %t, %v", record.Nonce, changed, err)
	}
}
