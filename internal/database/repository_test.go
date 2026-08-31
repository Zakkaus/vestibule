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
		Prompted: true, Tries: 2, QText: "question", CorrectIdx: -1, Nonce: "nonce", Deadline: 1234,
	}}
	if err = store.SavePending("ignored", func() []verification.PendingRecord { return pending }); err != nil {
		t.Fatal(err)
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
