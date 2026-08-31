package database

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestVerificationStoreRoundTrip(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewVerificationStore(db)
	pending := []verification.PendingRecord{{
		GroupID: -100, UserID: 7, GroupMsgID: 11, PrivateMsgID: 12,
		ChallengeDelivered: true, Mode: "kernel", Lang: "en", FbAnswers: []string{"gentoo.org"},
		Prompted: true, Tries: 2, QText: "question", CorrectIdx: -1, Nonce: "nonce", Deadline: 1234, Epoch: 4,
	}}
	inserted, err := store.InsertPending("ignored", pending[0])
	if err != nil {
		t.Fatal(err)
	}
	if !inserted {
		t.Fatal("first pending insert was rejected")
	}
	gotPending, err := store.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotPending, pending) {
		t.Fatalf("pending round trip = %#v, want %#v", gotPending, pending)
	}
	failures := []verification.FailureRecord{{GroupID: -100, UserID: 8, Count: 3, Last: 5678}}
	if err = store.SaveFailures("ignored", func() []verification.FailureRecord { return failures }); err != nil {
		t.Fatal(err)
	}
	gotFailures, err := store.LoadFailures("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotFailures, failures) {
		t.Fatalf("failure round trip = %#v, want %#v", gotFailures, failures)
	}
}
func TestVerificationStoreRejectsRepeatedOpenChallenge(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)
	first := verification.PendingRecord{
		GroupID: -100, UserID: 7, Nonce: "first", Deadline: 1234, Epoch: 1,
	}
	repeat := first
	repeat.Nonce = "repeat"

	firstInserted, err := state.InsertPending("ignored", first)
	if err != nil {
		t.Fatal(err)
	}
	repeatInserted, err := state.InsertPending("ignored", repeat)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := state.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("database open-challenge dedup rows=%#v, want one", pending)
	}
	t.Logf("first insert=%t repeat insert=%t pending rows=%d active nonce=%q",
		firstInserted, repeatInserted, len(pending), pending[0].Nonce)
	if !firstInserted || repeatInserted || pending[0].Nonce != first.Nonce {
		t.Fatalf("database open-challenge dedup failed: first=%t repeat=%t rows=%#v",
			firstInserted, repeatInserted, pending)
	}
}

func TestVerificationStoreRejectsStaleChallengeTransitions(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)
	old := verification.PendingRecord{
		GroupID: -100, UserID: 7, Nonce: "old", Deadline: 1234, Epoch: 3,
	}
	if inserted, insertErr := state.InsertPending("ignored", old); insertErr != nil || !inserted {
		t.Fatalf("insert old challenge = %t, %v", inserted, insertErr)
	}
	oldSettled, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: old.Ref(), Record: old, From: verification.ChallengePending, To: verification.ChallengeApproved,
		SettledAt: 2000,
	})
	if err != nil || !oldSettled {
		t.Fatalf("settle old challenge = %t, %v", oldSettled, err)
	}
	current := old
	current.Nonce = "current"
	current.Epoch = 8
	current.Deadline = 5678
	if inserted, insertErr := state.InsertPending("ignored", current); insertErr != nil || !inserted {
		t.Fatalf("insert current challenge = %t, %v", inserted, insertErr)
	}
	staleNonce, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: old.Ref(), Record: old, From: verification.ChallengePending, To: verification.ChallengeDeclined,
		SettledAt: 2001,
	})
	if err != nil {
		t.Fatal(err)
	}
	staleEpochRef := current.Ref()
	staleEpochRef.Epoch--
	staleEpoch, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: staleEpochRef, Record: current, From: verification.ChallengePending, To: verification.ChallengeExpired,
		SettledAt: 2002,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := state.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("stale transition left pending rows=%#v, want one", pending)
	}
	t.Logf("old settled=%t stale nonce affected=%t stale epoch affected=%t active nonce=%q epoch=%d",
		oldSettled, staleNonce, staleEpoch, pending[0].Nonce, pending[0].Epoch)
	if staleNonce || staleEpoch {
		t.Fatalf("stale transition affected rows: nonce=%t epoch=%t", staleNonce, staleEpoch)
	}
	if !reflect.DeepEqual(pending[0], current) {
		t.Fatalf("stale transition changed current challenge: got=%#v want=%#v", pending[0], current)
	}
}

func TestVerificationStatsAndWarningsRoundTrip(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	verificationStore := NewVerificationStore(db)
	agents := verification.AgentTally{Total: 4, Counts: map[string]int{"gpt": 3, "other": 1}}
	if err = verificationStore.SaveAgents("ignored", func() verification.AgentTally { return agents }); err != nil {
		t.Fatal(err)
	}
	gotAgents, err := verificationStore.LoadAgents("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAgents, agents) {
		t.Fatalf("agent round trip = %#v, want %#v", gotAgents, agents)
	}
	heartbeat := verification.HeartbeatRecord{LastOnline: 9012}
	if err = verificationStore.SaveHeartbeat("ignored", heartbeat); err != nil {
		t.Fatal(err)
	}
	if got, err := verificationStore.LoadHeartbeat("ignored"); err != nil || got != heartbeat {
		t.Fatalf("heartbeat round trip = %#v, %v; want %#v", got, err, heartbeat)
	}
	warnings := []moderate.WarningRecord{{GroupID: -200, UserID: 9, Count: 2}}
	warningStore := NewWarningStore(db)
	if err = warningStore.SaveWarnings(func() []moderate.WarningRecord { return warnings }); err != nil {
		t.Fatal(err)
	}
	if got, err := warningStore.LoadWarnings(); err != nil || !reflect.DeepEqual(got, warnings) {
		t.Fatalf("warning round trip = %#v, %v; want %#v", got, err, warnings)
	}
}

func TestVerificationStoreClaimsExpiredChallenges(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)
	const now, claimUntil = int64(1_700_000_000), int64(1_700_000_030)
	due := verification.PendingRecord{GroupID: -100, UserID: 7, Nonce: "due", Deadline: now, Epoch: 4}
	future := verification.PendingRecord{GroupID: -100, UserID: 8, Nonce: "future", Deadline: now + 1, Epoch: 9}
	for _, record := range []verification.PendingRecord{due, future} {
		inserted, insertErr := state.InsertPending("ignored", record)
		if insertErr != nil || !inserted {
			t.Fatalf("insert %q = %t, %v", record.Nonce, inserted, insertErr)
		}
	}
	claimed, err := state.ClaimExpired("ignored", now, claimUntil, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("due scanner claim count=%d, want 1: %#v", len(claimed), claimed)
	}
	t.Logf("due scanner claim: count=%d nonce=%q epoch=%d deadline=%d", len(claimed), claimed[0].Nonce, claimed[0].Epoch, claimed[0].Deadline)
	if claimed[0].Nonce != due.Nonce || claimed[0].Epoch != due.Epoch+1 || claimed[0].Deadline != claimUntil {
		t.Fatalf("claimed due rows = %#v, want only updated due record", claimed)
	}
	pending, err := state.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || !reflect.DeepEqual(pending[1], future) {
		t.Fatalf("future challenge changed after due claim: %#v", pending)
	}
}

func TestTransitionAndActionIntentRollbackTogether(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.RawDB.ExecContext(context.Background(), `
		CREATE TRIGGER fail_pending_action_insert
		BEFORE INSERT ON pending_action
		BEGIN
			SELECT RAISE(ABORT, 'injected action write failure');
		END`); err != nil {
		t.Fatal(err)
	}
	state := NewVerificationStore(db)
	record := verification.PendingRecord{GroupID: -100, UserID: 7, Nonce: "atomic", Deadline: 100, Epoch: 1}
	inserted, err := state.InsertPending("ignored", record)
	if err != nil || !inserted {
		t.Fatalf("insert pending = %t, %v", inserted, err)
	}
	changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: record.Ref(), Record: record, From: verification.ChallengePending, To: verification.ChallengeDeclined,
		SettledAt: 101, Actions: []verification.ActionIntent{{ID: "atomic-action", Kind: "settle_decline", Payload: `{}`, NextTryAt: 101}},
	})
	t.Logf("injected action failure: changed=%t err=%v", changed, err)
	if err == nil || changed {
		t.Fatalf("transition with injected action failure = changed:%t err:%v, want rollback error", changed, err)
	}
	pending, err := state.LoadPending("ignored")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || !reflect.DeepEqual(pending[0], record) {
		t.Fatalf("state transition escaped failed action transaction: %#v", pending)
	}
	var actions int
	if err = db.RawDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pending_action`).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if actions != 0 {
		t.Fatalf("failed transaction left %d action rows", actions)
	}
}

func TestUpdatePollLeaseExcludesConcurrentOwners(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	lease := NewUpdatePollLease(db)
	const now, until = int64(1_700_000_000), int64(1_700_000_045)

	count := acquirePollLeaseConcurrently(t, lease, now, until)
	t.Logf("simultaneous poll lease acquisitions=%d", count)
	if count != 1 {
		t.Fatalf("simultaneous poll lease owners=%d, want exactly 1", count)
	}
	assertPollLeaseOwnership(t, db, lease, now, until)
}

func acquirePollLeaseConcurrently(t *testing.T, lease *UpdatePollLease, now, until int64) int {
	t.Helper()
	var ready, acquired sync.WaitGroup
	ready.Add(8)
	acquired.Add(8)
	start := make(chan struct{})
	results := make(chan bool, 8)
	for i := range 8 {
		go func(i int) {
			defer acquired.Done()
			ready.Done()
			<-start
			ok, err := lease.Acquire(context.Background(), "instance-"+string(rune('a'+i)), now, until)
			if err != nil {
				t.Errorf("owner %d acquire: %v", i, err)
				return
			}
			results <- ok
		}(i)
	}
	ready.Wait()
	close(start)
	acquired.Wait()
	close(results)
	count := 0
	for ok := range results {
		if ok {
			count++
		}
	}
	return count
}

func assertPollLeaseOwnership(t *testing.T, db *Database, lease *UpdatePollLease, now, until int64) {
	t.Helper()
	if renewed, err := lease.Renew(context.Background(), "non-owner", now+1, until+1); err != nil || renewed {
		t.Fatalf("non-owner renewal = %t, %v; want false, nil", renewed, err)
	}
	var holder string
	if err := db.RawDB.QueryRowContext(context.Background(), `SELECT holder FROM update_poll_lease WHERE singleton=1`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(context.Background(), "non-owner"); err != nil {
		t.Fatal(err)
	}
	requirePollLeaseAcquire(t, lease, "successor", now+1, until+1, false)
	if err := lease.Release(context.Background(), holder); err != nil {
		t.Fatal(err)
	}
	requirePollLeaseAcquire(t, lease, "successor", now+1, until+1, true)
}

func requirePollLeaseAcquire(t *testing.T, lease *UpdatePollLease, owner string, now, until int64, want bool) {
	t.Helper()
	got, err := lease.Acquire(context.Background(), owner, now, until)
	if err != nil || got != want {
		t.Fatalf("acquire %q = %t, %v; want %t, nil", owner, got, err, want)
	}
}

func TestPendingActionsPersistRetriesAndFollowups(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)
	record := verification.PendingRecord{GroupID: -100, UserID: 7, Nonce: "action", Deadline: 100, Epoch: 1}
	requirePendingInsert(t, state, record)
	requireTransitionAction(t, state, record)

	requireClaimedAction(t, state, "worker-a", 101, 131, "approve-action")
	requireActionRetry(t, state, "approve-action", "worker-a", 1, 111)
	requireNoClaimedActions(t, state, "worker-b", 110, 140)
	retried := requireClaimedAction(t, state, "worker-b", 111, 141, "approve-action")
	if retried.Attempts != 1 {
		t.Fatalf("retry action attempts=%d, want 1", retried.Attempts)
	}
	followup := verification.ActionIntent{
		ID: "delete-action", Kind: "delete_group_message", Payload: `{"chat_id":-100,"message_id":42}`, NextTryAt: 111,
	}
	requireActionCompletion(t, state, "approve-action", "worker-b", 112, followup)
	claimed := requireClaimedAction(t, state, "worker-c", 112, 142, followup.ID)
	t.Logf("durable action retry attempts=%d followup=%q", retried.Attempts, claimed.ID)
}

func requirePendingInsert(t *testing.T, state *VerificationStore, record verification.PendingRecord) {
	t.Helper()
	inserted, err := state.InsertPending("ignored", record)
	if err != nil || !inserted {
		t.Fatalf("insert pending = %t, %v", inserted, err)
	}
}

func requireTransitionAction(t *testing.T, state *VerificationStore, record verification.PendingRecord) {
	t.Helper()
	changed, err := state.TransitionChallenge("ignored", verification.ChallengeTransition{
		Expected: record.Ref(), Record: record, From: verification.ChallengePending, To: verification.ChallengeApproved,
		SettledAt: 101, Actions: []verification.ActionIntent{{
			ID: "approve-action", Kind: "settle_approve", Payload: `{}`, NextTryAt: 101,
		}},
	})
	if err != nil || !changed {
		t.Fatalf("transition with action = %t, %v", changed, err)
	}
}

func requireClaimedAction(
	t *testing.T,
	state *VerificationStore,
	owner string,
	now, claimUntil int64,
	wantID string,
) verification.PendingAction {
	t.Helper()
	claimed, err := state.ClaimActions("ignored", owner, now, claimUntil, 8)
	if err != nil || len(claimed) != 1 || claimed[0].ID != wantID {
		t.Fatalf("action claim = %#v, %v; want %q", claimed, err, wantID)
	}
	return claimed[0]
}

func requireNoClaimedActions(t *testing.T, state *VerificationStore, owner string, now, claimUntil int64) {
	t.Helper()
	claimed, err := state.ClaimActions("ignored", owner, now, claimUntil, 8)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("action claim = %#v, %v; want none", claimed, err)
	}
}

func requireActionRetry(t *testing.T, state *VerificationStore, id, owner string, attempts int, nextTryAt int64) {
	t.Helper()
	changed, err := state.RetryAction("ignored", id, owner, attempts, nextTryAt, "temporary")
	if err != nil || !changed {
		t.Fatalf("retry action = %t, %v", changed, err)
	}
}

func requireActionCompletion(
	t *testing.T,
	state *VerificationStore,
	id, owner string,
	completedAt int64,
	followup verification.ActionIntent,
) {
	t.Helper()
	changed, err := state.CompleteAction("ignored", id, owner, completedAt, []verification.ActionIntent{followup})
	if err != nil || !changed {
		t.Fatalf("complete action = %t, %v", changed, err)
	}
}
