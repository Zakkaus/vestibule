package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// A settled challenge keeps the expires_at it carried while it was open, so the only thing
// that stops the expiry sweeper reclaiming it forever is the state predicate -- once in the
// select that finds due rows, once in the conditional update that claims one. Neither was
// held: with both neutralised the sweeper hands an applicant who was approved days ago back
// to the expiry path, which times the challenge out and declines or bans a member who is
// already in the group.
func TestExpirySweeperClaimsOnlyChallengesStillOpen(t *testing.T) {
	const chatID int64 = -1009000000811
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	// Settled first and with the earlier deadline, so a lost predicate shows up as this
	// record being claimed ahead of the one that is genuinely due.
	settled := verification.PendingRecord{
		GroupID: chatID, UserID: 7401, Name: "Approved", Nonce: "approved", Deadline: 100, Epoch: 3,
	}
	requireAuditTransition(t, state, settled, verification.ChallengeApproved, "", 150, 9)
	open := verification.PendingRecord{
		GroupID: chatID, UserID: 7402, Name: "Still waiting", Nonce: "open", Deadline: 110, Epoch: 1,
	}
	requirePendingInsert(t, state, open)

	claimed, err := state.ClaimExpired("ignored", 200, 260, 10)
	if err != nil {
		t.Fatal(err)
	}
	// The positive control: the challenge that really is due is claimed, so an empty result
	// would not be mistaken for the property holding.
	if len(claimed) != 1 || claimed[0].UserID != open.UserID {
		t.Fatalf("expiry sweep claimed %d challenge(s) %#v, want only user %d; a settled "+
			"challenge handed back to the expiry path times out a member who was already "+
			"let in", len(claimed), claimed, open.UserID)
	}

	var settledState string
	var expiresAt, epoch int64
	if err = db.QueryRow(ctx, "SELECT state, expires_at, epoch FROM challenge WHERE id=$1",
		challengeID(settled.Ref())).Scan(&settledState, &expiresAt, &epoch); err != nil {
		t.Fatal(err)
	}
	if settledState != string(verification.ChallengeApproved) || expiresAt != settled.Deadline ||
		epoch != int64(settled.Epoch) {
		t.Fatalf("settled challenge after the sweep = state:%q expires_at:%d epoch:%d, want "+
			"%q/%d/%d; the sweeper re-claimed a settlement it must not touch",
			settledState, expiresAt, epoch,
			verification.ChallengeApproved, settled.Deadline, settled.Epoch)
	}
}
