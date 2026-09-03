package database

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// When the compare-and-swap writes no row the store reads the revision back so the caller can
// tell a stale write from a group that has no row at all, and the console shows that number to
// the operator as the revision to retry against. The read-back's chat predicate was not held:
// without it the answer is whichever chat row the scan reaches first, so an operator editing
// one group is told to retry against another group's revision and their edit never lands.
func TestASettingsConflictReportsTheRevisionOfTheChatThatWasWritten(t *testing.T) {
	const neighbour, edited int64 = -1009000000831, -1009000000832
	store := newTestSettingsStore(t)
	// The neighbour is seeded first, so it is the row an unscoped read-back reaches first,
	// and its revision is one no correct answer for the edited chat can be.
	enabled := false
	if err := store.SeedSettings([]settings.Record{
		{ChatID: neighbour, Revision: 11, Overrides: settings.GroupOverrides{Enabled: &enabled}},
		{ChatID: edited, Revision: 4, Overrides: settings.GroupOverrides{Enabled: &enabled}},
	}); err != nil {
		t.Fatal(err)
	}

	next := true
	actual, written, err := store.CompareAndSwapSettings(
		edited, 1, settings.GroupOverrides{Enabled: &next})
	if err != nil {
		t.Fatal(err)
	}
	if written {
		t.Fatal("a write at revision 1 landed on a chat at revision 4")
	}
	if actual != 4 {
		t.Fatalf("conflict on chat %d reported revision %d, want 4; the operator is being "+
			"told to retry against a revision that belongs to another group, so the edit "+
			"never lands", edited, actual)
	}

	// The positive control: retrying at the revision the store just reported succeeds, which
	// is the whole point of reporting it.
	actual, written, err = store.CompareAndSwapSettings(
		edited, actual, settings.GroupOverrides{Enabled: &next})
	if err != nil || !written || actual != 5 {
		t.Fatalf("retry at the reported revision = actual:%d written:%v error:%v", actual, written, err)
	}
	records, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.ChatID == neighbour && record.Revision != 11 {
			t.Fatalf("neighbour chat revision = %d, want 11", record.Revision)
		}
	}
}
