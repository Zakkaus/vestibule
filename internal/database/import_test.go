package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var legacyStateFiles = []string{
	"pending.json",
	"verifyfail.json",
	"agents.json",
	"heartbeat.json",
	"warns.json",
}

func copyLegacyFixtures(t *testing.T) string {
	t.Helper()
	destination := t.TempDir()
	for _, name := range legacyStateFiles {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(destination, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return destination
}

func TestImportLegacyStateReplay(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stateDirectory := copyLegacyFixtures(t)

	first, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "first-backup"),
		Pending:         PendingCarry,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "second-backup"),
		Pending:         PendingCarry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ValidationText() != second.ValidationText() {
		t.Fatalf("replayed import changed validation\nfirst:\n%s\nsecond:\n%s", first.ValidationText(), second.ValidationText())
	}
	for _, report := range []ImportReport{first, second} {
		for _, name := range legacyStateFiles {
			if _, err := os.Stat(filepath.Join(report.BackupDirectory, name)); err != nil {
				t.Errorf("backup %s: %v", name, err)
			}
		}
	}
	t.Logf("first import:\n%s", first.ValidationText())
	t.Logf("second import:\n%s", second.ValidationText())
}

func TestImportLegacyStateAcceptsEmptySnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stateDirectory := t.TempDir()
	files := map[string]string{
		"pending.json": "[]", "verifyfail.json": "[]", "agents.json": `{"total":0,"counts":{}}`,
		"heartbeat.json": `{"last_online":0}`, "warns.json": "[]",
	}
	for name, contents := range files {
		if err = os.WriteFile(filepath.Join(stateDirectory, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory: stateDirectory, BackupDirectory: filepath.Join(t.TempDir(), "backup"),
		Pending: PendingCarry,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.PendingRows != 0 || report.FailureRows != 0 || report.AgentModels != 0 || report.WarningRows != 0 {
		t.Fatalf("non-empty validation report: %+v", report)
	}
}

func TestImportPreservesCorruptJSON(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stateDirectory := copyLegacyFixtures(t)
	bad := []byte(`[{"group_id":`)
	if err = os.WriteFile(filepath.Join(stateDirectory, "warns.json"), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	backupDirectory := filepath.Join(t.TempDir(), "corrupt-backup")

	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: backupDirectory,
		Pending:         PendingCarry,
	})
	if err == nil || !strings.Contains(err.Error(), "warns.json") {
		t.Fatalf("corrupt import error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(stateDirectory, "warns.json.corrupt"),
		filepath.Join(backupDirectory, "warns.json"),
	} {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read preserved corrupt state %s: %v", path, readErr)
		}
		if string(data) != string(bad) {
			t.Fatalf("preserved corrupt state %s = %q, want %q", path, data, bad)
		}
	}
}

// A corrupt snapshot is renamed so its bytes remain recoverable. A rerun must not treat the
// resulting missing source as an empty snapshot, because that would replace the table that still
// holds the previous import's warning counters.
func TestImportRefusesRerunWhenRecoveryQuarantinedASnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "valid-backup"),
		Pending:         PendingCarry,
	}); err != nil {
		t.Fatalf("valid import: %v", err)
	}
	before, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		t.Fatal(err)
	}

	stateDirectory := copyLegacyFixtures(t)
	warnings := filepath.Join(stateDirectory, "warns.json")
	if err := os.WriteFile(warnings, []byte(`[{"group_id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "corrupt-backup"),
		Pending:         PendingCarry,
	}); err == nil {
		t.Fatal("setup import accepted corrupt JSON; it must quarantine the source before a rerun")
	}
	if _, err := os.Stat(warnings + ".corrupt"); err != nil {
		t.Fatalf("corrupt source was not quarantined: %v", err)
	}

	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "rerun-backup"),
		Pending:         PendingCarry,
	})
	const want = "warns.json is missing while warns.json.corrupt exists; repair the source before importing"
	if err == nil {
		t.Fatal("import reran after a snapshot was quarantined; it would replace warning counters with an empty snapshot")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("rerunning after corrupt JSON recovery error = %q, want %q", err, want)
	}
	after, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("refusing the rerun changed warning counters from %#v to %#v", before, after)
	}
}

func TestImportBackupWritesToANewDirectory(t *testing.T) {
	stateDirectory := copyLegacyFixtures(t)
	backupDirectory := filepath.Join(t.TempDir(), "backup")
	created, err := backupLegacyJSON(ImportOptions{
		StateDirectory: stateDirectory, BackupDirectory: backupDirectory,
	})
	if err != nil {
		t.Fatalf("back up to a new directory: %v", err)
	}
	if created != backupDirectory {
		t.Fatalf("backup directory = %q, want %q", created, backupDirectory)
	}
	want, err := os.ReadFile(filepath.Join(stateDirectory, "pending.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(backupDirectory, "pending.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("backup of a new directory did not preserve the pending snapshot")
	}
}

func TestImportBackupRefusesAnExistingDirectory(t *testing.T) {
	stateDirectory := copyLegacyFixtures(t)
	if _, err := backupLegacyJSON(ImportOptions{
		StateDirectory: stateDirectory, BackupDirectory: filepath.Join(t.TempDir(), "new-backup"),
	}); err != nil {
		t.Fatalf("back up to a new directory: %v", err)
	}
	backupDirectory := filepath.Join(t.TempDir(), "backup")
	if err := os.Mkdir(backupDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(backupDirectory, "previous-import")
	if err := os.WriteFile(sentinel, []byte("keep this backup"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := backupLegacyJSON(ImportOptions{
		StateDirectory: stateDirectory, BackupDirectory: backupDirectory,
	})
	if err == nil {
		t.Fatal("backup reused an existing directory; a retry could overwrite the only previous-generation copy")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing backup directory error = %v, want an already-exists refusal", err)
	}
	got, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep this backup" {
		t.Fatalf("existing backup directory changed sentinel to %q", got)
	}
}

func TestImportBackupRefusesAnExistingFile(t *testing.T) {
	directory := t.TempDir()
	fresh := filepath.Join(directory, "fresh.json")
	if err := writeBackup(fresh, []byte("new backup")); err != nil {
		t.Fatalf("write backup to a new file: %v", err)
	}
	freshData, err := os.ReadFile(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if string(freshData) != "new backup" {
		t.Fatalf("new backup file = %q, want %q", freshData, "new backup")
	}

	existing := filepath.Join(directory, "previous.json")
	if err := os.WriteFile(existing, []byte("previous backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	err = writeBackup(existing, []byte("replacement"))
	if err == nil {
		t.Fatal("backup replaced an existing file; a retry could destroy the only pre-migration copy")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing backup file error = %v, want an already-exists refusal", err)
	}
	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous backup" {
		t.Fatalf("existing backup file changed to %q", got)
	}
}
