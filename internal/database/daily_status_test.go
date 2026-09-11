package database

import (
	"context"
	"sync"
	"testing"
)

func TestDailyStatusStoreDefaultsAndPersistsSwitch(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewDailyStatusStore(db)
	enabled, lastDate, err := store.LoadDailyStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || lastDate != "" {
		t.Fatalf("new daily status = enabled:%v date:%q, want enabled and empty date", enabled, lastDate)
	}
	if err := store.SetDailyEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if claimed, err := store.ClaimDailyAttempt(context.Background(), "2026-09-11"); err != nil || claimed {
		t.Fatalf("disabled daily status claim = %v, %v, want false without error", claimed, err)
	}
	if err := store.SetDailyEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimDailyAttempt(context.Background(), "2026-09-11")
	if err != nil || !claimed {
		t.Fatalf("first daily status claim = %v, %v, want true", claimed, err)
	}
	claimed, err = store.ClaimDailyAttempt(context.Background(), "2026-09-11")
	if err != nil || claimed {
		t.Fatalf("duplicate daily status claim = %v, %v, want false", claimed, err)
	}
}

func TestDailyStatusStoreKeepsDateWhenSwitchChanges(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewDailyStatusStore(db)
	if claimed, err := store.ClaimDailyAttempt(context.Background(), "2026-09-11"); err != nil || !claimed {
		t.Fatalf("seed daily status claim = %v, %v", claimed, err)
	}
	if err := store.SetDailyEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDailyEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	_, lastDate, err := store.LoadDailyStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lastDate != "2026-09-11" {
		t.Fatalf("switch toggle reset attempt date to %q", lastDate)
	}
	if claimed, err := store.ClaimDailyAttempt(context.Background(), "2026-09-10"); err != nil || claimed {
		t.Fatalf("backward daily date claim = %v, %v, want false", claimed, err)
	}
	if claimed, err := store.ClaimDailyAttempt(context.Background(), "2026-09-12"); err != nil || !claimed {
		t.Fatalf("next-day daily date claim = %v, %v, want true", claimed, err)
	}
}

func TestDailyStatusStoreConcurrentHandlesClaimOnlyOnce(t *testing.T) {
	ctx := context.Background()
	cfg := testSQLiteConfig(t)
	first, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	stores := []*DailyStatusStore{NewDailyStatusStore(first), NewDailyStatusStore(second)}
	results := make(chan bool, len(stores))
	errs := make(chan error, len(stores))
	var group sync.WaitGroup
	for _, store := range stores {
		group.Add(1)
		go func(store *DailyStatusStore) {
			defer group.Done()
			claimed, claimErr := store.ClaimDailyAttempt(ctx, "2026-09-13")
			results <- claimed
			errs <- claimErr
		}(store)
	}
	group.Wait()
	close(results)
	close(errs)
	claims := 0
	for claimed := range results {
		if claimed {
			claims++
		}
	}
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatal(claimErr)
		}
	}
	if claims != 1 {
		t.Fatalf("concurrent daily claims = %d, want exactly one", claims)
	}
}
