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

func TestStalePendingWritesAreLostRacesNotStorageErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settle   bool
		staleRef func(verification.PendingRecord) verification.PendingRef
		write    func(*VerificationStore, verification.PendingRecord, verification.PendingRef) (bool, error)
	}{
		{
			name: "update after settlement", settle: true,
			staleRef: func(record verification.PendingRecord) verification.PendingRef { return record.Ref() },
			write: func(state *VerificationStore, record verification.PendingRecord, ref verification.PendingRef) (bool, error) {
				record.Tries++
				return state.UpdatePending("ignored", ref, record)
			},
		},
		{
			name: "delete after settlement", settle: true,
			staleRef: func(record verification.PendingRecord) verification.PendingRef { return record.Ref() },
			write: func(state *VerificationStore, record verification.PendingRecord, ref verification.PendingRef) (bool, error) {
				return state.DeletePending("ignored", ref)
			},
		},
		{
			name: "update with stale epoch",
			staleRef: func(record verification.PendingRecord) verification.PendingRef {
				ref := record.Ref()
				ref.Epoch--
				return ref
			},
			write: func(state *VerificationStore, record verification.PendingRecord, ref verification.PendingRef) (bool, error) {
				record.Tries++
				return state.UpdatePending("ignored", ref, record)
			},
		},
		{
			name: "delete with stale epoch",
			staleRef: func(record verification.PendingRecord) verification.PendingRef {
				ref := record.Ref()
				ref.Epoch--
				return ref
			},
			write: func(state *VerificationStore, record verification.PendingRecord, ref verification.PendingRef) (bool, error) {
				return state.DeletePending("ignored", ref)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := guardStore(t)
			target := verification.PendingRecord{
				GroupID: -1009000000101, UserID: 101, Nonce: "stale", Deadline: 100, Epoch: 2,
			}
			control := target
			control.UserID, control.Nonce = 102, "control"
			requirePendingInsert(t, state, target)
			requirePendingInsert(t, state, control)

			if changed, err := tc.write(state, control, control.Ref()); err != nil || !changed {
				t.Fatalf("write to a live challenge = %t, %v; want success", changed, err)
			}
			if tc.settle {
				if changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
					Expected: target.Ref(), Record: target, From: verification.ChallengePending, To: verification.ChallengeApproved,
					SettledAt: 101,
				}); err != nil || !changed {
					t.Fatalf("settle target challenge = %t, %v", changed, err)
				}
			}
			if changed, err := tc.write(state, target, tc.staleRef(target)); err != nil || changed {
				t.Fatalf("stale %s = %t, %v; want false, nil so a lost race cannot overwrite or remove another settlement",
					tc.name, changed, err)
			}
		})
	}
}

func TestChallengeTransitionOnlyChangesTheChallengeItNames(t *testing.T) {
	state := guardStore(t)
	neighbour := verification.PendingRecord{
		GroupID: -1009000000102, UserID: 102, Nonce: "neighbour", Deadline: 100, Epoch: 1,
	}
	missing := verification.PendingRecord{
		GroupID: -1009000000103, UserID: 103, Nonce: "missing", Deadline: 100, Epoch: 1,
	}
	requirePendingInsert(t, state, neighbour)

	if changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: missing.Ref(), Record: missing, From: verification.ChallengePending, To: verification.ChallengeApproved,
		SettledAt: 101,
	}); err != nil || changed {
		t.Fatalf("a transition for a missing challenge settled someone else's pending challenge = %t, %v; want false, nil",
			changed, err)
	}
	pending, err := state.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Nonce != neighbour.Nonce {
		t.Fatalf("a missing challenge transition changed the neighbour's pending record = %#v", pending)
	}
	if changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: neighbour.Ref(), Record: neighbour, From: verification.ChallengePending, To: verification.ChallengeApproved,
		SettledAt: 102,
	}); err != nil || !changed {
		t.Fatalf("a transition for the named challenge = %t, %v; want success", changed, err)
	}
}

func TestActionSettlersOnlyChangeTheActionTheyName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		settle func(*VerificationStore, string) (bool, error)
	}{
		{"complete", func(state *VerificationStore, id string) (bool, error) {
			return state.CompleteAction("ignored", id, "worker-a", 102, nil)
		}},
		{"retry", func(state *VerificationStore, id string) (bool, error) {
			return state.RetryAction("ignored", id, "worker-a", 1, 110, "temporary")
		}},
		{"fail", func(state *VerificationStore, id string) (bool, error) {
			return state.FailAction("ignored", id, "worker-a", 102, "permanent")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := storeWithTwoClaimedActions(t)
			if changed, err := tc.settle(state, "missing-action"); err != nil || changed {
				t.Fatalf("%s for an action the worker does not hold = %t, %v; want false, nil so it cannot settle another action",
					tc.name, changed, err)
			}
			for _, id := range []string{"action-one", "action-two"} {
				if changed, err := tc.settle(state, id); err != nil || !changed {
					t.Fatalf("%s for the named action %q = %t, %v; want success", tc.name, id, changed, err)
				}
			}
		})
	}
}

func storeWithTwoClaimedActions(t *testing.T) *VerificationStore {
	t.Helper()
	state := guardStore(t)
	record := verification.PendingRecord{
		GroupID: -1009000000104, UserID: 104, Nonce: "two-actions", Deadline: 100, Epoch: 1,
	}
	requirePendingInsert(t, state, record)
	changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: record.Ref(), Record: record, From: verification.ChallengePending, To: verification.ChallengeApproved,
		SettledAt: 101,
		Actions: []verification.ActionIntent{
			{ID: "action-one", Kind: "settle_approve", Payload: `{}`, NextTryAt: 101},
			{ID: "action-two", Kind: "settle_decline", Payload: `{}`, NextTryAt: 101},
		},
	})
	if err != nil || !changed {
		t.Fatalf("transition two actions = %t, %v", changed, err)
	}
	claimed, err := state.ClaimActions("ignored", "worker-a", 101, 131, 2)
	if err != nil || len(claimed) != 2 {
		t.Fatalf("claim two actions = %#v, %v", claimed, err)
	}
	return state
}
