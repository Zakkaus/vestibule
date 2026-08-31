package database

import (
	"context"
	"os"
	"path/filepath"
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
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: filepath.Join(t.TempDir(), "second-backup"),
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
