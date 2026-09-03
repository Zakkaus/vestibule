package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// The undo is inserted straight into the same outbox the ban is waiting in, and nothing
// orders the two. The guard that refuses an undo while its settle_ban is unfinished was not
// held: the one test that undoes a ban completes the ban first, so it never reaches the
// case. Without the guard a worker can run the unban before the ban, which leaves the
// applicant banned with nothing left in the queue to undo it -- the operator pressed undo,
// was told it was accepted, and the person stays out.
func TestChallengeUndoWaitsForTheBanItUndoes(t *testing.T) {
	const chatID int64 = -1009000000821
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	banned := verification.PendingRecord{
		GroupID: chatID, UserID: 7501, Name: "Banned", Nonce: "banned", Deadline: 90, Epoch: 2,
	}
	requireAuditTransition(t, state, banned, verification.ChallengeBanned, "", 100, 9,
		verification.ActionIntent{
			ID: "settle-ban", Kind: "settle_ban", Payload: `{}`, NextTryAt: 100,
			ClaimOwner: "settler", ClaimUntil: 130,
		})
	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("challenge audit = %#v, want the one banned challenge", records)
	}
	undo := verification.ActionIntent{
		ID: "undo-ban", Kind: "undo_ban", Payload: `{"chat_id":-1009000000821,"user_id":7501}`,
		NextTryAt: 120, ClaimOwner: "operator", ClaimUntil: 150,
	}

	enqueued, err := state.EnqueueChallengeUndo(ctx, records[0], undo)
	if err != nil {
		t.Fatal(err)
	}
	if enqueued {
		t.Fatalf("undo was enqueued while its ban is still %q in the outbox; the unban can "+
			"then run before the ban and the applicant stays banned with nothing left to "+
			"undo it", verification.ChallengeActionPending)
	}
	if undoAction := auditUndoAction(t, ctx, state, chatID); undoAction != verification.ChallengeActionNone {
		t.Fatalf("undo row after the refusal = %q, want none", undoAction)
	}

	// The positive control: once the ban has actually been carried out the same undo is
	// accepted, so the refusal above is about ordering and not about a malformed request.
	requireAuditActionCompletion(t, state, "settle-ban", "settler", 101)
	enqueued, err = state.EnqueueChallengeUndo(ctx, records[0], undo)
	if err != nil {
		t.Fatal(err)
	}
	if !enqueued {
		t.Fatal("undo was refused after the ban had been carried out")
	}
}

// LoadChallengeAudit reads the settlement action and the undo action with two left joins
// against the same outbox table, and only the undo side names its kind in a way any test
// noticed. Without the settlement side's kind filter an undone ban matches both rows, so the
// console lists the same ban twice and the settlement state shown against one of them is
// really the undo's -- an operator reading the list cannot tell what happened to the person.
func TestChallengeAuditReturnsOneRowPerSettledChallenge(t *testing.T) {
	const chatID int64 = -1009000000822
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	banned := verification.PendingRecord{
		GroupID: chatID, UserID: 7601, Name: "Banned", Nonce: "banned", Deadline: 90, Epoch: 2,
	}
	requireAuditTransition(t, state, banned, verification.ChallengeBanned, "", 100, 9,
		verification.ActionIntent{
			ID: "settle-ban", Kind: "settle_ban", Payload: `{}`, NextTryAt: 100,
			ClaimOwner: "settler", ClaimUntil: 130,
		})
	requireAuditActionCompletion(t, state, "settle-ban", "settler", 101)
	declined := verification.PendingRecord{
		GroupID: chatID, UserID: 7602, Name: "Declined", Nonce: "declined", Deadline: 95, Epoch: 1,
	}
	requireAuditTransition(t, state, declined, verification.ChallengeDeclined, "wrong_answer", 110, 9)

	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	// The positive control: two settled challenges, two rows, before any undo exists.
	if len(records) != 2 {
		t.Fatalf("challenge audit before the undo = %#v, want one row per settled challenge", records)
	}

	requireChallengeUndo(t, state, records[1], verification.ActionIntent{
		ID: "undo-ban", Kind: "undo_ban", Payload: `{"chat_id":-1009000000822,"user_id":7601}`,
		NextTryAt: 120, ClaimOwner: "operator", ClaimUntil: 150,
	}, true)

	records, err = state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]int, len(records))
	for _, record := range records {
		seen[record.ID]++
	}
	if len(records) != 2 || seen[challengeID(banned.Ref())] != 1 {
		t.Fatalf("challenge audit after the undo listed %d rows (%v), want one per settled "+
			"challenge; a duplicated ban tells the operator the wrong thing about a person",
			len(records), seen)
	}
	for _, record := range records {
		if record.ID != challengeID(banned.Ref()) {
			continue
		}
		if record.SettlementAction != verification.ChallengeActionDone ||
			record.UndoAction != verification.ChallengeActionPending {
			t.Fatalf("undone ban = settlement:%q undo:%q, want done and pending; the "+
				"settlement column has picked up the undo's state",
				record.SettlementAction, record.UndoAction)
		}
	}
}

func auditUndoAction(
	t *testing.T,
	ctx context.Context,
	state *VerificationStore,
	chatID int64,
) verification.ChallengeActionState {
	t.Helper()
	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("challenge audit = %#v, want one record", records)
	}
	return records[0].UndoAction
}
