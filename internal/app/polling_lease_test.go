package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeUpdatePollLease struct {
	mu       sync.Mutex
	acquired bool
	renewed  bool
	release  []string
}

func (f *fakeUpdatePollLease) Acquire(context.Context, string, int64, int64) (bool, error) {
	return f.acquired, nil
}

func (f *fakeUpdatePollLease) Renew(context.Context, string, int64, int64) (bool, error) {
	return f.renewed, nil
}

func (f *fakeUpdatePollLease) Release(_ context.Context, owner string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.release = append(f.release, owner)
	return nil
}

func TestNonHolderDoesNotExposePollingHandler(t *testing.T) {
	lease := &fakeUpdatePollLease{}
	polling, err := acquirePollingLease(context.Background(), lease)
	if err != nil {
		t.Fatal(err)
	}
	if polling.held || polling.Done() != nil {
		t.Fatal("non-holder must not construct or expose an update handler")
	}
	if err := polling.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(lease.release) != 0 {
		t.Fatalf("non-holder released %v, want no owner release", lease.release)
	}
}

func TestPollingLeaseLossCancelsAndGracefulStopReleases(t *testing.T) {
	lease := &fakeUpdatePollLease{renewed: false}
	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	polling := &pollingLease{
		lease: lease, owner: "holder", held: true,
		now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		cancel: func() { cancelOnce.Do(func() { close(cancelled) }) },
	}
	if polling.renewOnce(context.Background()) {
		t.Fatal("lost renewal reported success")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("lost renewal did not cancel long polling")
	}
	if !polling.leaseLost {
		t.Fatal("lost renewal was not recorded")
	}
	if err := polling.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if len(lease.release) != 1 || lease.release[0] != "holder" {
		t.Fatalf("graceful stop releases = %v, want holder only", lease.release)
	}
}
