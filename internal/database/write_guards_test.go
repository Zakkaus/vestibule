package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func guardStore(t *testing.T) *VerificationStore {
	t.Helper()
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewVerificationStore(db)
}

// A settlement statement carries claim_owner because the outbox is worked by more than one
// process. Removing it from all three settlement statements left every test passing: the one
// test that works actions never lets a second worker touch an action the first one holds, so
// nothing said what the guard is for. Without it a worker that lost its lease could mark done,
// retry, or fail an action another worker is executing right now, and the person on the other
// end would be approved or banned twice.
func TestOnlyTheWorkerHoldingTheClaimMaySettleAnAction(t *testing.T) {
	state := guardStore(t)
	record := verification.PendingRecord{GroupID: -100, UserID: 7, Nonce: "guard", Deadline: 100, Epoch: 1}
	requirePendingInsert(t, state, record)
	requireTransitionAction(t, state, record)
	requireClaimedAction(t, state, "worker-a", 101, 131, "approve-action")

	if changed, err := state.CompleteAction("ignored", "approve-action", "worker-b", 102, nil); err != nil || changed {
		t.Fatalf("a second worker completed an action it does not hold = %t, %v", changed, err)
	}
	if changed, err := state.RetryAction("ignored", "approve-action", "worker-b", 1, 110, "x"); err != nil || changed {
		t.Fatalf("a second worker retried an action it does not hold = %t, %v", changed, err)
	}
	if changed, err := state.FailAction("ignored", "approve-action", "worker-b", 103, "x"); err != nil || changed {
		t.Fatalf("a second worker failed an action it does not hold = %t, %v", changed, err)
	}
	// The holder still settles it, so the three refusals above are about ownership and not
	// about an action that had already left the pending state.
	if changed, err := state.CompleteAction("ignored", "approve-action", "worker-a", 104, nil); err != nil || !changed {
		t.Fatalf("the holder could not complete its own action = %t, %v", changed, err)
	}
}

// Settling an action twice must change nothing: a worker that crashes between making the
// Telegram call and recording the outcome comes back and settles again. Two things hold that
// today — the state guard, and settling clearing claim_owner — so neither can be isolated, and
// this test goes red only when both are broken. That is what a property test should do; nothing
// asserted the property at all before.
func TestAnActionIsNotSettledTwice(t *testing.T) {
	state := guardStore(t)
	record := verification.PendingRecord{GroupID: -100, UserID: 8, Nonce: "guard", Deadline: 100, Epoch: 1}
	requirePendingInsert(t, state, record)
	requireTransitionAction(t, state, record)
	requireClaimedAction(t, state, "worker-a", 101, 131, "approve-action")
	if changed, err := state.CompleteAction("ignored", "approve-action", "worker-a", 102, nil); err != nil || !changed {
		t.Fatalf("first completion = %t, %v", changed, err)
	}

	for _, tc := range []struct {
		name string
		call func() (bool, error)
	}{
		{"complete", func() (bool, error) {
			return state.CompleteAction("ignored", "approve-action", "worker-a", 103, nil)
		}},
		{"retry", func() (bool, error) {
			return state.RetryAction("ignored", "approve-action", "worker-a", 1, 110, "x")
		}},
		{"fail", func() (bool, error) {
			return state.FailAction("ignored", "approve-action", "worker-a", 103, "x")
		}},
	} {
		if changed, err := tc.call(); err != nil || changed {
			t.Fatalf("%s of an action that is already done = %t, %v", tc.name, changed, err)
		}
	}
}

// Two statements keep a claim from being taken twice: the query that picks candidates and the
// update that takes each one. Both spell out that the lease is unset or spent, and neutralising
// the condition in both left every test passing, because no test ever asks a second worker for
// work while the first one still holds its lease.
func TestClaimingLeavesAnActionAnotherWorkerStillHolds(t *testing.T) {
	state := guardStore(t)
	record := verification.PendingRecord{GroupID: -100, UserID: 9, Nonce: "guard", Deadline: 100, Epoch: 1}
	requirePendingInsert(t, state, record)
	requireTransitionAction(t, state, record)
	requireClaimedAction(t, state, "worker-a", 101, 200, "approve-action")

	requireNoClaimedActions(t, state, "worker-b", 150, 250)
	// Once the first worker's lease has run out the action is work again, so the refusal above
	// is about the live lease and not about an action nobody can ever claim.
	requireClaimedAction(t, state, "worker-b", 201, 300, "approve-action")
}

// UpdatePending and DeletePending name their challenge three ways over: by identifier, by group,
// and by user. The identifier is derived from the other two, so dropping any one of them alone
// changes nothing — but neutralising all three at once also left every test passing, because no
// test has ever held two pending challenges at the same time. Either statement would then land
// on whichever row the database happened to reach first: somebody else's verification, in
// somebody else's group.
func TestPendingWritesLandOnTheChallengeTheyNamed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(*VerificationStore, verification.PendingRecord) (bool, error)
	}{
		{"update", func(s *VerificationStore, r verification.PendingRecord) (bool, error) {
			next := r
			next.Tries = 2
			return s.UpdatePending("ignored", r.Ref(), next)
		}},
		{"delete", func(s *VerificationStore, r verification.PendingRecord) (bool, error) {
			return s.DeletePending("ignored", r.Ref())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := guardStore(t)
			target := verification.PendingRecord{GroupID: -100, UserID: 21, Nonce: "target", Deadline: 100, Epoch: 1}
			neighbour := verification.PendingRecord{GroupID: -200, UserID: 22, Nonce: "neighbour", Deadline: 100, Epoch: 1}
			requirePendingInsert(t, state, target)
			requirePendingInsert(t, state, neighbour)

			if changed, err := tc.write(state, target); err != nil || !changed {
				t.Fatalf("write against the named challenge = %t, %v", changed, err)
			}
			remaining, err := state.LoadPending("ignored")
			if err != nil {
				t.Fatal(err)
			}
			var found verification.PendingRecord
			for _, record := range remaining {
				if record.GroupID == neighbour.GroupID && record.UserID == neighbour.UserID {
					found = record
				}
			}
			if found.Nonce != neighbour.Nonce || found.Tries != neighbour.Tries || found.Deadline != neighbour.Deadline {
				t.Fatalf("the other group's challenge after a %s = %#v, want it untouched", tc.name, found)
			}
		})
	}
}
