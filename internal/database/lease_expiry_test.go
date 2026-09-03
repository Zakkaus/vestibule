package database

import (
	"context"
	"testing"
)

// Renewing extends only a lease that has not run out. The holder predicate is held by a test,
// the expiry is not: neutralising it left every test passing, because no test has ever renewed
// after its own lease expired.
//
// A lease that has expired is work the next instance is entitled to take, and the point of the
// expiry is that whoever wants it back competes for it again. Renewing straight through the
// expiry skips that: an instance that was paused past its deadline — a long stop-the-world
// pause, a suspended host, a stalled disk — puts itself back in charge without asking, and if a
// successor took the stream in the meantime, both of them believe they hold it. Telegram accepts
// one update stream per bot token, so the two would then take turns losing each other's updates.
func TestAnExpiredLeaseIsNotRenewed(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	lease := NewUpdatePollLease(db)
	ctx := context.Background()
	const (
		start   = int64(1_700_000_000)
		expiry  = int64(1_700_000_045)
		expired = expiry + 1
	)
	requirePollLeaseAcquire(t, lease, "instance-a", start, expiry, true)

	// Still inside its own window, the holder renews.
	if renewed, err := lease.Renew(ctx, "instance-a", expiry-1, expiry+30); err != nil || !renewed {
		t.Fatalf("renewal inside the window = %t, %v; want true, nil", renewed, err)
	}
	// Past the window it has to acquire again rather than renew.
	if renewed, err := lease.Renew(ctx, "instance-a", expiry+60, expiry+90); err != nil || renewed {
		t.Fatalf("renewal after the lease expired = %t, %v; want false, nil", renewed, err)
	}
	holder, err := lease.Holder(ctx, expiry+60)
	if err != nil {
		t.Fatal(err)
	}
	if holder != "" {
		t.Fatalf("holder after the lease expired = %q, want nobody", holder)
	}
	// And the expired lease is there to be taken, which is what the refusal is protecting.
	requirePollLeaseAcquire(t, lease, "instance-b", expiry+61, expiry+120, true)
}
