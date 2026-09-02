package database

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestImportLegacyStatePersistsFixtureSnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "backup"),
	})
	if err != nil {
		t.Errorf("ImportLegacyState fixture: %v", err)
	}
	if report.PendingRows != 2 || report.FailureRows != 2 || report.AgentModels != 3 ||
		report.AgentTotal != 6 || report.LastOnline != 1_787_574_896 || report.WarningRows != 3 {
		t.Errorf("fixture import report = %+v, want pending=2 failures=2 agent_models=3 agent_total=6 last_online=1787574896 warnings=3", report)
	}

	assertImportedVerificationFixture(t, db)
	assertImportedWarningFixture(t, db)
}
func assertImportedVerificationFixture(t *testing.T, db *Database) {
	t.Helper()
	verificationStore := NewVerificationStore(db)
	pending, err := verificationStore.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	wantPending := []verification.PendingRecord{
		{
			UserID: 7002, GroupID: -1009999900005, GroupMsgID: 502, PrivateMsgID: 602,
			Mode: "quiz", Lang: "zh-hant", Prompted: true,
			QText: "Select the package manager", QOpts: []string{"apt", "Portage", "dnf"}, CorrectIdx: 1,
			Nonce: "quiz-compat-nonce", Name: "Quiz Applicant", Deadline: 4_071_009_906,
		},
		{
			UserID: 7001, GroupID: -1009999900004, GroupMsgID: 501, PrivateMsgID: 601,
			Mode: "kernel", Lang: "en", FbAnswers: []string{"gentoozh.org", "gentoozh"},
			Prompted: true, Hinted: true, SampleBounced: true, NoLinuxReminded: true, OSClarified: true, Tries: 2,
			QText: "Name the Gentoo Chinese community website", CorrectIdx: -1, Nonce: "kernel-compat-nonce",
			Name: "Kernel Applicant", Deadline: 4_071_006_245, DeferredSince: 4_070_919_845,
		},
	}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Errorf("pending snapshot = %#v, want %#v", pending, wantPending)
	}

	failures, err := verificationStore.LoadFailures("")
	if err != nil {
		t.Fatal(err)
	}
	wantFailures := []verification.FailureRecord{
		{GroupID: -1009999900005, UserID: 7202, Count: 3, Last: 1_787_569_933},
		{GroupID: -1009999900004, UserID: 7201, Count: 2, Last: 1_787_566_272},
	}
	if !reflect.DeepEqual(failures, wantFailures) {
		t.Errorf("verification failure snapshot = %#v, want %#v", failures, wantFailures)
	}

	agents, err := verificationStore.LoadAgents("")
	if err != nil {
		t.Fatal(err)
	}
	wantAgents := verification.AgentTally{
		Total: 6,
		Counts: map[string]int{
			"claude-opus-4.5": 2,
			"gemini-2.5-pro":  1,
			"gpt-5":           3,
		},
	}
	if !reflect.DeepEqual(agents, wantAgents) {
		t.Errorf("agent snapshot = %#v, want %#v", agents, wantAgents)
	}

	heartbeat, err := verificationStore.LoadHeartbeat("")
	if err != nil {
		t.Fatal(err)
	}
	if want := (verification.HeartbeatRecord{LastOnline: 1_787_574_896}); heartbeat != want {
		t.Errorf("heartbeat snapshot = %#v, want %#v", heartbeat, want)
	}
}

func assertImportedWarningFixture(t *testing.T, db *Database) {
	t.Helper()
	warnings, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		t.Fatal(err)
	}
	wantWarnings := []moderate.WarningRecord{
		{GroupID: -1009999900005, UserID: 7101, Count: 4},
		{GroupID: -1009999900004, UserID: 7101, Count: 1},
		{GroupID: -1009999900004, UserID: 7102, Count: 2},
	}
	if !reflect.DeepEqual(warnings, wantWarnings) {
		t.Errorf("warning snapshot = %#v, want %#v", warnings, wantWarnings)
	}
}

func TestImportLegacyStateRejectsSilentlyDroppedSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(ctx, `
		CREATE TRIGGER discard_imported_failures
		BEFORE INSERT ON verification_failure
		BEGIN
			SELECT RAISE(IGNORE);
		END`); err != nil {
		t.Fatal(err)
	}

	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "backup"),
	})
	if err == nil {
		t.Fatal("ImportLegacyState succeeded after the verification failure snapshot was silently dropped")
	}
	if !strings.Contains(err.Error(), "validate verification failures") {
		t.Fatalf("ImportLegacyState error = %v, want validation mismatch", err)
	}
}
