package database

import (
	"context"
	"testing"
)

func TestTestConfigsIsolateStoresAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	firstConfig, secondConfig := testDatabaseConfig(t), testDatabaseConfig(t)
	first, err := Open(ctx, firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(ctx, secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	if err = NewDailyStatusStore(first).SetDailyEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if claimed, err := NewDailyStatusStore(second).ClaimDailyAttempt(ctx, "2026-10-03"); err != nil || !claimed {
		t.Fatalf("claim in independent store = %t, %v; want success", claimed, err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	first, err = Open(ctx, firstConfig)
	if err != nil {
		t.Fatal(err)
	}
	firstEnabled, firstDate, err := NewDailyStatusStore(first).LoadDailyStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	secondEnabled, secondDate, err := NewDailyStatusStore(second).LoadDailyStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if firstEnabled || firstDate != "" || !secondEnabled || secondDate != "2026-10-03" {
		t.Fatalf("isolated stores = (%t, %q), (%t, %q); want (false, empty), (true, 2026-10-03)",
			firstEnabled, firstDate, secondEnabled, secondDate)
	}
}
