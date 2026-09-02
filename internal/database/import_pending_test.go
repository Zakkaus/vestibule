package database

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// Whether a migration carries the previous generation's open challenges is a decision with
// two real answers, and for several phases the plan said one and the command silently did
// the other. The import now refuses to assume: the disposition has no default, and refusing
// happens before anything is backed up or written.
func TestImportRequiresAPendingDisposition(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	backup := filepath.Join(t.TempDir(), "backup")
	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: backup,
	})
	if err == nil {
		t.Fatal("import ran without being told what to do with open challenges")
	}
	if !strings.Contains(err.Error(), "carry") || !strings.Contains(err.Error(), "drop") {
		t.Errorf("refusal = %q, want both answers named so the operator can pick one", err)
	}
	if report.BackupDirectory != "" {
		t.Errorf("report names a backup at %q; the refusal must come before anything is "+
			"written", report.BackupDirectory)
	}
}

// Dropping leaves the open challenges behind and imports everything else. The plan's
// parallel run keeps the previous generation on, so the applicants it drops are applicants
// that bot is still answering for.
func TestImportDroppingPendingKeepsTheOtherSnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "backup"),
		Pending:         PendingDrop,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.PendingRows != 0 {
		t.Errorf("pending rows = %d, want none: the import was told to drop them",
			report.PendingRows)
	}
	if report.FailureRows != 2 || report.AgentModels != 3 || report.WarningRows != 3 {
		t.Errorf("report = %+v, want failures=2 agent_models=3 warnings=3: dropping open "+
			"challenges must not drop anything else", report)
	}
	pending, err := NewVerificationStore(db).LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Errorf("database holds %d pending records after a drop", len(pending))
	}
}
