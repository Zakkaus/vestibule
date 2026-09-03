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

// The settlement join accepts three action kinds, but the existing audit fixtures exercised
// only settle_ban. Losing either other member makes a completed approval or decline look as if
// it never reached Telegram, so an operator cannot distinguish completed work from missing work.
func TestChallengeAuditReportsEverySettlementActionKind(t *testing.T) {
	const chatID int64 = -1009000000831
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	cases := []struct {
		nonce  string
		userID int64
		state  verification.ChallengeState
		reason string
		kind   string
	}{
		{nonce: "approved", userID: 7701, state: verification.ChallengeApproved, kind: "settle_approve"},
		{nonce: "declined", userID: 7702, state: verification.ChallengeDeclined, reason: "wrong_answer", kind: "settle_decline"},
		{nonce: "banned", userID: 7703, state: verification.ChallengeBanned, kind: "settle_ban"},
	}
	for index, test := range cases {
		record := verification.PendingRecord{
			GroupID: chatID, UserID: test.userID, Name: test.nonce, Nonce: test.nonce,
			Deadline: 90, Epoch: 1,
		}
		actionID := "settlement-" + test.nonce
		requireAuditTransition(t, state, record, test.state, test.reason, int64(100+index), 9,
			verification.ActionIntent{
				ID: actionID, Kind: test.kind, Payload: `{}`, NextTryAt: 100,
				ClaimOwner: "settler", ClaimUntil: 130,
			})
		requireAuditActionCompletion(t, state, actionID, "settler", int64(110+index))
	}

	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(cases) {
		t.Fatalf("challenge audit returned %d records, want %d completed decisions", len(records), len(cases))
	}
	byNonce := make(map[string]verification.ChallengeAuditRecord, len(records))
	for _, record := range records {
		byNonce[record.Record.Nonce] = record
	}
	for _, test := range cases {
		record, ok := byNonce[test.nonce]
		if !ok {
			t.Fatalf("challenge audit omitted the completed %s decision", test.state)
		}
		if record.State != test.state || record.SettlementAction != verification.ChallengeActionDone {
			t.Fatalf("completed %s decision appears as state %q with settlement action %q; "+
				"the operator cannot tell whether it reached Telegram",
				test.state, record.State, record.SettlementAction)
		}
	}
}

// An undo row belongs to one challenge. Without the undo join's challenge identity, every
// settled challenge in the group inherits that row and the console tells operators that an
// unrelated person's ban is also being undone.
func TestChallengeAuditKeepsUndoWithItsChallenge(t *testing.T) {
	const chatID int64 = -1009000000832
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	target := verification.PendingRecord{
		GroupID: chatID, UserID: 7801, Name: "Target", Nonce: "target", Deadline: 90, Epoch: 1,
	}
	neighbour := verification.PendingRecord{
		GroupID: chatID, UserID: 7802, Name: "Neighbour", Nonce: "neighbour", Deadline: 90, Epoch: 1,
	}
	for index, record := range []verification.PendingRecord{target, neighbour} {
		actionID := "settle-ban-" + record.Nonce
		requireAuditTransition(t, state, record, verification.ChallengeBanned, "", int64(100+index), 9,
			verification.ActionIntent{
				ID: actionID, Kind: "settle_ban", Payload: `{}`, NextTryAt: 100,
				ClaimOwner: "settler", ClaimUntil: 130,
			})
		requireAuditActionCompletion(t, state, actionID, "settler", int64(110+index))
	}

	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	var expected verification.ChallengeAuditRecord
	for _, record := range records {
		if record.ID == challengeID(target.Ref()) {
			expected = record
			break
		}
	}
	if expected.ID == "" {
		t.Fatal("challenge audit omitted the ban selected for undo")
	}
	inserted, err := state.EnqueueChallengeUndo(ctx, expected, verification.ActionIntent{
		ID: "undo-target", Kind: "undo_ban",
		Payload:   `{"chat_id":-1009000000832,"user_id":7801}`,
		NextTryAt: 120, ClaimOwner: "operator", ClaimUntil: 150,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("valid undo was refused")
	}

	records, err = state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	undoByID := make(map[string]verification.ChallengeActionState, len(records))
	for _, record := range records {
		undoByID[record.ID] = record.UndoAction
	}
	if undoByID[challengeID(target.Ref())] != verification.ChallengeActionPending {
		t.Fatalf("selected ban has undo state %q, want pending", undoByID[challengeID(target.Ref())])
	}
	if got := undoByID[challengeID(neighbour.Ref())]; got != verification.ChallengeActionNone {
		t.Fatalf("unrelated ban inherited undo state %q; the console now claims the wrong "+
			"person's ban is being undone", got)
	}
}

// Settlement timestamps have one-second precision, so ties are normal. The identifier
// tie-break keeps repeated reads stable instead of letting equally recent people swap places.
func TestChallengeAuditBreaksEqualSettlementTimesByID(t *testing.T) {
	const chatID int64 = -1009000000833
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	first := verification.PendingRecord{
		GroupID: chatID, UserID: 7901, Name: "First", Nonce: "first", Deadline: 90, Epoch: 1,
	}
	second := verification.PendingRecord{
		GroupID: chatID, UserID: 7902, Name: "Second", Nonce: "second", Deadline: 90, Epoch: 1,
	}
	requireAuditTransition(t, state, first, verification.ChallengeApproved, "", 100, 9)
	requireAuditTransition(t, state, second, verification.ChallengeApproved, "", 100, 9)

	records, err := state.LoadChallengeAudit(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("challenge audit returned %d tied records, want 2", len(records))
	}
	wantFirst := challengeID(second.Ref())
	if records[0].ID != wantFirst || records[1].ID != challengeID(first.Ref()) {
		t.Fatalf("equal-time audit order = [%q, %q], want identifier-descending order "+
			"starting with %q; otherwise people swap places between console reads",
			records[0].ID, records[1].ID, wantFirst)
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
