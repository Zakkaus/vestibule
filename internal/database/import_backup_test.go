package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The backup is the only copy of the state the import is about to replace, and the load path is
// what decides whether a snapshot is genuinely empty or merely unreadable. Both were exercised
// only along their happy path: every existing test hands ImportLegacyState a fresh directory with
// all five files present, so nothing said what happens on the second attempt, or when a file the
// previous run renamed is the one that is missing.

// importedFixtureDatabase returns a database holding the fixture snapshots, so a later refusal can
// be checked against what it must have left alone.
func importedFixtureDatabase(t *testing.T) (context.Context, *Database, string) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	directory := copyLegacyFixtures(t)
	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "seed"),
		Pending: PendingCarry,
	}); err != nil {
		t.Fatal(err)
	}
	assertWarningRows(t, db, 3)
	return ctx, db, directory
}

func assertWarningRows(t *testing.T, db *Database, want int) {
	t.Helper()
	warnings, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != want {
		t.Errorf("the warnings table holds %d rows, want %d; every warning ever issued in every "+
			"group is what this number counts", len(warnings), want)
	}
}

// store.Load renames a snapshot it cannot decode to <name>.corrupt and treats the now-missing file
// as an empty snapshot. So the second run of an import that first hit a corrupt warns.json would
// replace the warnings table with nothing, and report it as a clean migration: every warning count
// in every group reset to zero, and a member one warning from a kick starting over.
func TestImportRefusesASnapshotThatIsMissingBesideItsCorruptSibling(t *testing.T) {
	ctx, db, directory := importedFixtureDatabase(t)
	if err := os.Rename(filepath.Join(directory, "warns.json"), filepath.Join(directory, "warns.json.corrupt")); err != nil {
		t.Fatal(err)
	}

	_, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "second"),
		Pending: PendingCarry,
	})
	if err == nil {
		t.Fatal("the import ran with warns.json gone and warns.json.corrupt in its place; it " +
			"would have replaced the warnings table with an empty snapshot")
	}
	if !strings.Contains(err.Error(), "warns.json") || !strings.Contains(err.Error(), "repair") {
		t.Errorf("refusal = %q, want warns.json named and repairing the source asked for", err)
	}
	assertWarningRows(t, db, 3)
}

// The refusal above has to be caused by the corrupt sibling, not by the file being absent: a
// state directory that never held warns.json is an ordinary import.
func TestImportAcceptsASnapshotThatIsSimplyAbsent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	directory := copyLegacyFixtures(t)
	if err := os.Remove(filepath.Join(directory, "warns.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "absent"),
		Pending: PendingCarry,
	}); err != nil {
		t.Fatalf("import with warns.json absent and no corrupt sibling: %v", err)
	}
}

// A sibling that cannot be inspected is not a sibling that is absent. Swallow the error and the
// import decides the snapshot is empty on the strength of a question it never got an answer to,
// and the warnings table is replaced with nothing.
func TestImportRefusesWhenACorruptSiblingCannotBeInspected(t *testing.T) {
	ctx, db, directory := importedFixtureDatabase(t)
	if err := os.Remove(filepath.Join(directory, "warns.json")); err != nil {
		t.Fatal(err)
	}
	// A symlink pointing at itself: stat fails with a loop error, which is neither success nor
	// "does not exist". A permission-denied directory would do the same but not when the tests
	// run as root.
	if err := os.Symlink("warns.json.corrupt", filepath.Join(directory, "warns.json.corrupt")); err != nil {
		t.Skipf("this test needs a symlink: %v", err)
	}

	_, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "unstattable"),
		Pending: PendingCarry,
	})
	if err == nil {
		t.Fatal("the import treated a sibling it could not stat as absent and replaced the " +
			"warnings table with an empty snapshot")
	}
	if !strings.Contains(err.Error(), "warns.json") {
		t.Errorf("refusal = %q, want the file it could not inspect named", err)
	}
	assertWarningRows(t, db, 3)
}

// Running the import again after one failed partway is the normal thing to do, and the second run
// must not write over the backup the first one took. That backup is the only surviving copy of the
// previous generation's JSON; overwriting it with whatever the first run left on disk removes the
// ability to roll back to the pre-migration state at all.
func TestImportRefusesToReuseAnExistingBackupDirectory(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	directory := copyLegacyFixtures(t)
	backup := filepath.Join(t.TempDir(), "vestibule-migration")
	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: backup, Pending: PendingCarry,
	}); err != nil {
		t.Fatal(err)
	}
	original := map[string][]byte{}
	for _, name := range legacyStateFiles {
		data, err := os.ReadFile(filepath.Join(backup, name))
		if err != nil {
			t.Fatal(err)
		}
		original[name] = data
	}
	// Whatever the first run left behind: here, valid but emptied-out snapshots, so the second
	// import has nothing wrong with it except the backup directory it was pointed at.
	for name, contents := range map[string]string{
		"pending.json": "[]", "verifyfail.json": "[]", "agents.json": `{"total":0,"counts":{}}`,
		"heartbeat.json": `{"last_online":0}`, "warns.json": "[]",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: backup, Pending: PendingCarry,
	}); err == nil {
		t.Error("the second import reused the first import's backup directory")
	}
	for _, name := range legacyStateFiles {
		data, err := os.ReadFile(filepath.Join(backup, name))
		if err != nil {
			t.Fatalf("backup %s: %v", name, err)
		}
		if string(data) != string(original[name]) {
			t.Errorf("backup %s was overwritten: %q, want the copy the first import took (%q); "+
				"the pre-migration state is now unrecoverable", name, data, original[name])
		}
	}
}

// -backup-dir may name a path whose parent has not been created yet, which is what an operator
// keeping migration backups under their own directory tree does.
func TestImportCreatesTheBackupParent(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	backup := filepath.Join(t.TempDir(), "var", "backups", "vestibule-migration")

	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: copyLegacyFixtures(t), BackupDirectory: backup, Pending: PendingCarry,
	})
	if err != nil {
		t.Fatalf("import into a backup directory whose parent does not exist yet: %v", err)
	}
	if report.BackupDirectory != backup {
		t.Errorf("report names %q, want %q", report.BackupDirectory, backup)
	}
	for _, name := range legacyStateFiles {
		if _, err := os.Stat(filepath.Join(backup, name)); err != nil {
			t.Errorf("backup %s: %v", name, err)
		}
	}
}

// A snapshot the previous generation never wrote is absent, and a zero-byte file in the backup is
// an empty snapshot. Those are different facts, and whoever reads the backup later — to decide
// whether a rollback loses anything — only has the backup to read.
func TestAnAbsentSnapshotIsNotBackedUpAsAnEmptyFile(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	directory := copyLegacyFixtures(t)
	if err := os.Remove(filepath.Join(directory, "heartbeat.json")); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "partial")

	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: directory, BackupDirectory: backup, Pending: PendingCarry,
	}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(backup, "heartbeat.json")); err == nil {
		t.Errorf("the backup holds a %d-byte heartbeat.json for a snapshot that was never "+
			"written; it reads as a genuine empty snapshot to whoever opens the backup", info.Size())
	}
	// The snapshots that do exist are still backed up, so the skip is about the absent one.
	for _, name := range []string{"pending.json", "verifyfail.json", "agents.json", "warns.json"} {
		if _, err := os.Stat(filepath.Join(backup, name)); err != nil {
			t.Errorf("backup %s: %v", name, err)
		}
	}
}
