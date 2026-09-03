package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The import re-reads what it wrote and compares it with the JSON it read, and refuses to
// report success when the two differ. Only the verification-failure comparison was held; the
// pending one was not, in either direction. What it protects is the applicants who were
// mid-verification when the previous generation was stopped: a row the migration loses, or
// keeps when it was told not to, is presented to the operator as verified, and nobody
// challenges those people again.
func TestImportRefusesSuccessWhenOpenChallengesDisagreeWithTheJSON(t *testing.T) {
	t.Run("a row the database kept when the import was told to drop it", func(t *testing.T) {
		ctx := context.Background()
		db, err := Open(ctx, testSQLiteConfig(t))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		stateDirectory := copyLegacyFixtures(t)
		// The positive control: the same import against the same database validates before
		// anything is made to disagree, so the refusal below is about the disagreement.
		carried, err := ImportLegacyState(ctx, db, ImportOptions{
			StateDirectory:  stateDirectory,
			BackupDirectory: filepath.Join(t.TempDir(), "carry-backup"),
			Pending:         PendingCarry,
		})
		if err != nil || carried.PendingRows != 2 {
			t.Fatalf("carrying import = %+v, %v; want the two fixture challenges", carried, err)
		}
		if _, err = db.Exec(ctx, `
			CREATE TRIGGER keep_open_challenges
			BEFORE DELETE ON challenge
			BEGIN
				SELECT RAISE(IGNORE);
			END`); err != nil {
			t.Fatal(err)
		}

		_, err = ImportLegacyState(ctx, db, ImportOptions{
			StateDirectory:  stateDirectory,
			BackupDirectory: filepath.Join(t.TempDir(), "drop-backup"),
			Pending:         PendingDrop,
		})
		requirePendingValidationRefusal(t, err,
			"the import reported success while the database still held open challenges it "+
				"was told to leave behind")
	})

	t.Run("a row the database mangled on the way in", func(t *testing.T) {
		ctx := context.Background()
		db, err := Open(ctx, testSQLiteConfig(t))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		// Same number of challenges, different content: the count says nothing is wrong and
		// only the field-by-field comparison can see it.
		if _, err = db.Exec(ctx, `
			CREATE TRIGGER mangle_imported_challenge
			AFTER INSERT ON challenge
			BEGIN
				UPDATE challenge SET attempts=attempts+7 WHERE id=NEW.id;
			END`); err != nil {
			t.Fatal(err)
		}

		_, err = ImportLegacyState(ctx, db, ImportOptions{
			StateDirectory:  copyLegacyFixtures(t),
			BackupDirectory: filepath.Join(t.TempDir(), "backup"),
			Pending:         PendingCarry,
		})
		requirePendingValidationRefusal(t, err,
			"the import reported success after an open challenge was written with a field "+
				"the JSON did not carry")
	})
}

func requirePendingValidationRefusal(t *testing.T, err error, harm string) {
	t.Helper()
	if err == nil {
		t.Fatal(harm)
	}
	if !strings.Contains(err.Error(), "validate pending records") {
		t.Fatalf("import error = %v, want the pending validation to be what refused; %s", err, harm)
	}
}

// internal/store/json.go renames a snapshot it cannot parse to <name>.corrupt and starts
// fresh, so a missing file beside a .corrupt sibling is exactly what a previously corrupted
// bot leaves on disk. Treating that as an empty snapshot drops every open challenge, warning
// counter and failure count the corrupted file held, and tells the operator the import
// validated. Nothing held the check.
func TestImportRefusesASnapshotThatSurvivesOnlyAsACorruptSibling(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stateDirectory := copyLegacyFixtures(t)
	snapshot := filepath.Join(stateDirectory, "pending.json")
	contents, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(snapshot+".corrupt", contents, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(snapshot); err != nil {
		t.Fatal(err)
	}

	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "corrupt-backup"),
		Pending:         PendingCarry,
	})
	if err == nil {
		t.Fatal("import ran with pending.json present only as pending.json.corrupt; every " +
			"applicant that file was still holding is dropped and the operator is told the " +
			"import validated")
	}
	if !strings.Contains(err.Error(), "pending.json") || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("refusal = %v, want the corrupt sibling named so the operator can repair it", err)
	}
	pending, err := NewVerificationStore(db).LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("refused import still wrote %d challenge(s)", len(pending))
	}

	// The positive control: a snapshot that is simply absent, with no corrupt sibling beside
	// it, is a bot that never wrote one. That still imports, so the refusal above is about
	// the sibling and not about the file being missing.
	if err = os.Remove(snapshot + ".corrupt"); err != nil {
		t.Fatal(err)
	}
	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "absent-backup"),
		Pending:         PendingCarry,
	})
	if err != nil {
		t.Fatalf("import with pending.json simply absent = %v", err)
	}
	if report.PendingRows != 0 || report.FailureRows != 2 || report.WarningRows != 3 {
		t.Fatalf("import with pending.json simply absent = %+v, want the other snapshots carried", report)
	}
}
