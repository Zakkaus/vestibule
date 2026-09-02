package database

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The import deletes and rebuilds every table it owns, so running it against a database an
// instance is polling for would replace verifications in flight with a snapshot of the
// generation being replaced. Migration happens with the old bot stopped; an unexpired polling
// lease says this is not that moment. A cold database is unaffected, which is what the
// phase-ten acceptance about repeating the import depends on.
func TestImportRefusesWhileAnInstanceHoldsThePollingLease(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	lease := NewUpdatePollLease(db)
	now := time.Now().Unix()
	if taken, err := lease.Acquire(ctx, "running-instance", now, now+300); err != nil || !taken {
		t.Fatalf("Acquire = %v, %v; the test needs the lease held", taken, err)
	}

	_, err = ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "held"),
		Pending:         PendingCarry,
	})
	if err == nil {
		t.Fatal("import ran while an instance held the polling lease; it would have replaced " +
			"verifications that instance is running")
	}
	if !strings.Contains(err.Error(), "running-instance") {
		t.Errorf("refusal = %q, want the lease holder named so an operator knows what to stop", err)
	}

	if err := lease.Release(ctx, "running-instance"); err != nil {
		t.Fatal(err)
	}
	report, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "released"),
		Pending:         PendingCarry,
	})
	if err != nil {
		t.Fatalf("import after the lease was released: %v", err)
	}
	if report.PendingRows != 2 {
		t.Errorf("pending rows after the lease was released = %d, want 2: releasing the lease "+
			"must leave an ordinary import, not a half-refused one", report.PendingRows)
	}
}

// An expired lease is a stopped instance, not a running one.
func TestImportProceedsWhenThePollingLeaseHasExpired(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().Unix()
	if taken, err := NewUpdatePollLease(db).Acquire(ctx, "stopped-instance", now-600, now-300); err != nil || !taken {
		t.Fatalf("Acquire = %v, %v; the test needs an expired lease on the row", taken, err)
	}

	if _, err := ImportLegacyState(ctx, db, ImportOptions{
		StateDirectory:  copyLegacyFixtures(t),
		BackupDirectory: filepath.Join(t.TempDir(), "expired"),
		Pending:         PendingCarry,
	}); err != nil {
		t.Fatalf("import with an expired lease: %v; an expired holder is a stopped instance", err)
	}
}
