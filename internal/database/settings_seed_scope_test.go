package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// Seeding imports a group's configured values into its own row, and only while that row is
// still untouched. The write is scoped by chat id and by the row being empty; only the second
// half was held. Dropping the id predicate writes each record into every chat still empty at
// that moment, so several groups seeded together all end up holding whichever record was
// processed last -- with every test in the repository still passing.
func TestSeedSettingsWritesEachRecordIntoItsOwnChat(t *testing.T) {
	const first, second int64 = -1009000000901, -1009000000902
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewSettingsStore(db)
	// Both rows have to exist and be empty, because seeding takes the insert path for a chat
	// it creates and only reaches the scoped update for one that is already there. Created
	// here, the update is what runs, which is the statement under test.
	for _, chatID := range []int64{first, second} {
		if err := ensureChat(ctx, db, chatID); err != nil {
			t.Fatal(err)
		}
	}

	firstBan, secondBan := 111, 222
	if err := store.SeedSettings([]settings.Record{
		{ChatID: first, Revision: 1, Overrides: settings.GroupOverrides{BanSeconds: &firstBan}},
		{ChatID: second, Revision: 1, Overrides: settings.GroupOverrides{BanSeconds: &secondBan}},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64]int{}
	for _, record := range loaded {
		if record.Overrides.BanSeconds != nil {
			got[record.ChatID] = *record.Overrides.BanSeconds
		}
	}
	if got[first] != firstBan || got[second] != secondBan {
		t.Fatalf("seeded settings = %v, want %d for %d and %d for %d; one group's configuration "+
			"reached another's row", got, firstBan, first, secondBan, second)
	}
}

// The same write refuses a row somebody has already changed, so seeding never overwrites a
// setting an administrator made.
func TestSeedSettingsLeavesATouchedRowAlone(t *testing.T) {
	const chatID int64 = -1009000000903
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewSettingsStore(db)

	chosen := 333
	if err := store.SeedSettings([]settings.Record{
		{ChatID: chatID, Revision: 1, Overrides: settings.GroupOverrides{BanSeconds: &chosen}},
	}); err != nil {
		t.Fatal(err)
	}

	seeded := 444
	if err := store.SeedSettings([]settings.Record{
		{ChatID: chatID, Revision: 2, Overrides: settings.GroupOverrides{BanSeconds: &seeded}},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range loaded {
		if record.ChatID != chatID {
			continue
		}
		if record.Overrides.BanSeconds == nil || *record.Overrides.BanSeconds != chosen {
			t.Fatalf("a second seed overwrote a row that already held a value: %+v",
				record.Overrides.BanSeconds)
		}
	}
}
