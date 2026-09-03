package database

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestValidationTextLabelsEachSnapshotCount(t *testing.T) {
	report := ImportReport{
		PendingRows: 11,
		FailureRows: 22,
		AgentModels: 33,
		AgentTotal:  44,
		LastOnline:  55,
		WarningRows: 66,
	}
	want := "pending: rows=11; verified=group_id,user_id,nonce,deadline,mode,all_payload_fields\n" +
		"verifyfail: rows=22; verified=group_id,user_id,count,last\n" +
		"agents: models=33 total=44; verified=model,count,total\n" +
		"heartbeat: last_online=55; verified=last_online\n" +
		"warns: rows=66; verified=group_id,user_id,count"
	if got := report.ValidationText(); got != want {
		t.Fatalf("validation text mislabeled snapshot counts; an operator could accept a partial import:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestPersistLegacyStateRollsBackEverySnapshotWhenTheLastReplacementFails(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	previous := atomicImportState(-1009000000401, 4101, "previous", 7)
	next := atomicImportState(-1009000000402, 4201, "next", 9)
	if err = persistLegacyState(ctx, db, previous); err != nil {
		t.Fatalf("seed previous snapshot: %v", err)
	}
	if _, err = db.Exec(ctx, `
		CREATE TRIGGER fail_imported_warnings
		BEFORE DELETE ON warning_counter
		BEGIN
			SELECT RAISE(ABORT, 'injected warning replacement failure');
		END`); err != nil {
		t.Fatal(err)
	}

	err = persistLegacyState(ctx, db, next)
	if err == nil || !strings.Contains(err.Error(), "injected warning replacement failure") {
		t.Fatalf("last replacement error = %v, want injected warning failure", err)
	}
	if _, err = validateLegacyState(db, previous); err != nil {
		t.Fatalf("failed import left a mixed-generation database instead of rolling every snapshot back: %v", err)
	}

	if _, err = db.Exec(ctx, "DROP TRIGGER fail_imported_warnings"); err != nil {
		t.Fatal(err)
	}
	if err = persistLegacyState(ctx, db, next); err != nil {
		t.Fatalf("same import after removing the injected failure: %v", err)
	}
	if _, err = validateLegacyState(db, next); err != nil {
		t.Fatalf("successful import did not commit every snapshot: %v", err)
	}
}

func atomicImportState(chatID, userID int64, marker string, count int) legacyState {
	return legacyState{
		pending: []verification.PendingRecord{{
			GroupID: chatID, UserID: userID, Nonce: marker, Deadline: int64(count), Epoch: uint64(count),
		}},
		failures: []verification.FailureRecord{{
			GroupID: chatID, UserID: userID + 1, Count: count, Last: int64(count),
		}},
		agents: verification.AgentTally{
			Total: count,
			Counts: map[string]int{
				marker: count,
			},
		},
		heartbeat: verification.HeartbeatRecord{LastOnline: int64(count)},
		warnings: []moderate.WarningRecord{{
			GroupID: chatID, UserID: userID + 2, Count: count,
		}},
	}
}

func TestBackupCreatesAParentThatDoesNotExist(t *testing.T) {
	stateDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDirectory, "pending.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	backupDirectory := filepath.Join(t.TempDir(), "missing", "parent", "backup")
	got, err := backupLegacyJSON(ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: backupDirectory,
	})
	if err != nil {
		t.Fatalf("backup into a missing parent failed instead of creating the parent: %v", err)
	}
	if got != backupDirectory {
		t.Errorf("backup directory = %q, want %q", got, backupDirectory)
	}
	if data, err := os.ReadFile(filepath.Join(backupDirectory, "pending.json")); err != nil || string(data) != "[]" {
		t.Errorf("backup in newly created parent = %q, %v, want copied pending snapshot", data, err)
	}
}

func TestBackupOmitsLegacyFilesThatDoNotExist(t *testing.T) {
	stateDirectory := t.TempDir()
	contents := []byte("present snapshot")
	if err := os.WriteFile(filepath.Join(stateDirectory, "pending.json"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	backupDirectory := filepath.Join(t.TempDir(), "backup")
	if _, err := backupLegacyJSON(ImportOptions{
		StateDirectory:  stateDirectory,
		BackupDirectory: backupDirectory,
	}); err != nil {
		t.Fatalf("backup with absent legacy files: %v", err)
	}
	if copied, err := os.ReadFile(filepath.Join(backupDirectory, "pending.json")); err != nil || string(copied) != string(contents) {
		t.Fatalf("existing legacy snapshot was not copied exactly: got %q, error %v", copied, err)
	}
	for _, name := range legacyJSONNames[1:] {
		path := filepath.Join(backupDirectory, name)
		if info, err := os.Stat(path); err == nil {
			t.Errorf("missing legacy %s became a %d-byte backup that looks like a genuine empty snapshot", name, info.Size())
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect omitted backup %s: %v", name, err)
		}
	}
}

func TestLegacyLoadReportsFilesystemInspectionErrors(t *testing.T) {
	t.Run("readable legacy file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pending.json")
		if err := os.WriteFile(path, []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := rejectMissingCorrupt(path); err != nil {
			t.Fatalf("readable legacy file was refused: %v", err)
		}
	})
	t.Run("absent legacy file", func(t *testing.T) {
		if err := rejectMissingCorrupt(filepath.Join(t.TempDir(), "pending.json")); err != nil {
			t.Fatalf("ordinary absent legacy file was refused: %v", err)
		}
	})
	t.Run("legacy file inspection error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pending.json")
		makeSelfReferentialSymlink(t, path)
		if err := rejectMissingCorrupt(path); err == nil || !strings.Contains(err.Error(), "inspect pending.json") {
			t.Fatalf("legacy file inspection error was swallowed as if the snapshot were absent: %v", err)
		}
	})
	t.Run("corrupt sidecar inspection error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "warns.json")
		makeSelfReferentialSymlink(t, path+".corrupt")
		if err := rejectMissingCorrupt(path); err == nil || !strings.Contains(err.Error(), "inspect warns.json.corrupt") {
			t.Fatalf("corrupt sidecar inspection error was swallowed as if the snapshot were absent: %v", err)
		}
	})
}

func makeSelfReferentialSymlink(t *testing.T, path string) {
	t.Helper()
	if err := os.Symlink(filepath.Base(path), path); err != nil {
		t.Fatal(err)
	}
}
