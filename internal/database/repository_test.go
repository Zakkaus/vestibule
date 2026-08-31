package database

import (
	"context"
	"reflect"
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
