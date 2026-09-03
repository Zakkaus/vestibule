package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// After the commit the import re-reads the database and proves it matches the JSON, and that proof
// is what the operator reads before stopping the old bot. Only one of the six comparisons was
// exercised. With the others silent the import prints its clean five-line report over a database
// holding none of the pending challenges, a zeroed agent tally, a zero heartbeat, or no warnings
// at all -- and the report says "verified" about each of them.
//
// Each test below makes the database disagree with the JSON in exactly one way, using a trigger,
// and asserts the import refuses. A trigger is the only way in: the comparison runs inside
// ImportLegacyState, so the disagreement has to exist by the time the commit lands.

func importWithTrigger(t *testing.T, trigger string) error {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(ctx, trigger); err != nil {
		t.Fatalf("installing the trigger this test needs: %v", err)
	}
	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: copyLegacyFixtures(t), BackupDirectory: filepath.Join(t.TempDir(), "backup"),
		Pending: PendingCarry,
	})
	return err
}

func assertRefused(t *testing.T, err error, want string, harm string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the import reported a clean migration: %s", harm)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("refusal = %q, want %q named", err, want)
	}
}

// The same fixtures with no trigger import cleanly, so every refusal below is caused by the
// disagreement the test introduced and not by the fixtures or the harness.
func TestTheFixturesImportCleanlyWithoutATrigger(t *testing.T) {
	if err := importWithTrigger(t, "CREATE TABLE unused_control (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("import of the unmodified fixtures: %v", err)
	}
}

func TestImportRefusesWhenTheDatabaseHoldsAPendingChallengeTheJSONDoesNot(t *testing.T) {
	err := importWithTrigger(t, `
		CREATE TRIGGER duplicate_imported_challenge
		AFTER INSERT ON challenge WHEN NEW.user_id = 7001
		BEGIN
			INSERT INTO challenge
				(id, chat_id, user_id, state, kind, payload, delivery, attempts, expires_at, epoch)
			VALUES (NEW.id || '-extra', NEW.chat_id, 7003, NEW.state, NEW.kind, NEW.payload,
				NEW.delivery, NEW.attempts, NEW.expires_at, NEW.epoch);
		END`)
	assertRefused(t, err, "validate pending records",
		"the database holds an open challenge for somebody the previous generation never challenged")
}

func TestImportRefusesWhenAPendingChallengeWasStoredWithADifferentDeadline(t *testing.T) {
	err := importWithTrigger(t, `
		CREATE TRIGGER shift_imported_deadline
		AFTER INSERT ON challenge
		BEGIN
			UPDATE challenge SET expires_at = NEW.expires_at + 3600 WHERE id = NEW.id;
		END`)
	assertRefused(t, err, "validate pending records",
		"every carried challenge would expire an hour later than the applicant was told")
}

func TestImportRefusesWhenTheAgentTallyWasNotStored(t *testing.T) {
	err := importWithTrigger(t, `
		CREATE TRIGGER discard_imported_agents
		BEFORE INSERT ON agent_tally
		BEGIN
			SELECT RAISE(IGNORE);
		END`)
	assertRefused(t, err, "validate agent tally",
		"the report would say the models were verified over an empty tally")
}

func TestImportRefusesWhenTheHeartbeatWasNotStored(t *testing.T) {
	err := importWithTrigger(t, `
		CREATE TRIGGER discard_imported_heartbeat
		BEFORE INSERT ON verification_runtime WHEN NEW.key = 'last_online'
		BEGIN
			SELECT RAISE(IGNORE);
		END`)
	assertRefused(t, err, "validate heartbeat",
		"the new bot would read a zero last-online and treat the whole gap as an outage")
}

func TestImportRefusesWhenTheWarningCountsWereChanged(t *testing.T) {
	err := importWithTrigger(t, `
		CREATE TRIGGER shift_imported_warning
		AFTER INSERT ON warning_counter
		BEGIN
			UPDATE warning_counter SET count = NEW.count + 1
			WHERE chat_id = NEW.chat_id AND user_id = NEW.user_id;
		END`)
	assertRefused(t, err, "validate warning records",
		"members would carry a warning count nobody ever gave them")
}

// The report is the operator's only record of the migration, and each line is read as a fact about
// one snapshot. Permute the arguments and an import that lost every warning still prints a
// plausible non-zero figure on the warns line and is accepted.
func TestValidationTextLabelsEachCountWithItsOwnSnapshot(t *testing.T) {
	// Six distinct values, so no two lines can be swapped without the text changing.
	report := ImportReport{
		PendingRows: 11, FailureRows: 22, AgentModels: 33, AgentTotal: 44,
		LastOnline: 55, WarningRows: 66,
	}
	text := report.ValidationText()
	for _, want := range []string{
		"pending: rows=11", "verifyfail: rows=22", "agents: models=33 total=44",
		"heartbeat: last_online=55", "warns: rows=66",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the report does not carry %q; a count is printed on a line that names a "+
				"different snapshot\n%s", want, text)
		}
	}
}

// The five replacements are five deletes and five sets of inserts. Run outside a transaction, a
// failure part-way leaves pending challenges, verification failures, the agent tally and the
// heartbeat already replaced while the warnings still hold the previous generation's rows — a
// state neither bot was written for, with nothing to roll back to and no report, because
// validation never runs on the error path.
func TestTheFiveTableReplacementsCommitAsOneTransaction(t *testing.T) {
	ctx, db, _ := importedFixtureDatabase(t)
	before := snapshotCounts(t, db)

	if _, err := db.Exec(ctx, `
		CREATE TRIGGER refuse_imported_warning
		BEFORE INSERT ON warning_counter
		BEGIN
			SELECT RAISE(ABORT, 'the warnings table refused this row');
		END`); err != nil {
		t.Fatal(err)
	}
	// A second generation of snapshots: everything emptied except the warnings, whose insert the
	// trigger refuses. The failure therefore lands after the other four replacements have run.
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"pending.json": "[]", "verifyfail.json": "[]", "agents.json": `{"total":0,"counts":{}}`,
		"heartbeat.json": `{"last_online":0}`,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	warnings, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", "warns.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "warns.json"), warnings, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "partial"),
		Pending: PendingCarry,
	}); err == nil {
		t.Fatal("the import succeeded although the warnings could not be written")
	}
	if after := snapshotCounts(t, db); after != before {
		t.Errorf("the failed import left the database at %+v, want it untouched at %+v: four "+
			"tables were replaced and the fifth was not, and nothing says which", after, before)
	}
}

type snapshotSizes struct {
	pending, failures, agentModels, warnings int
	agentTotal                               int
	lastOnline                               int64
}

func snapshotCounts(t *testing.T, db *Database) snapshotSizes {
	t.Helper()
	store := NewVerificationStore(db)
	pending, err := store.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	failures, err := store.LoadFailures("")
	if err != nil {
		t.Fatal(err)
	}
	agents, err := store.LoadAgents("")
	if err != nil {
		t.Fatal(err)
	}
	heartbeat, err := store.LoadHeartbeat("")
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		t.Fatal(err)
	}
	return snapshotSizes{
		pending: len(pending), failures: len(failures), agentModels: len(agents.Counts),
		warnings: len(warnings), agentTotal: agents.Total, lastOnline: heartbeat.LastOnline,
	}
}
