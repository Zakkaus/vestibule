package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestConfiguredKnownChatSurvivesUnknownGroupCleanup(t *testing.T) {
	const (
		knownChat   = int64(-1009000001661)
		unknownChat = int64(-1009000001662)
	)
	cfg, store := registrationFixture(t)
	cfg.KnownChatIDs = []int64{knownChat}
	if store.IsKnownChat(knownChat) {
		t.Fatal("test known chat unexpectedly entered the runtime store")
	}
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	service := newRegistrationService(
		context.Background(), newRegistrationBot(t, caller), store, cfg,
		"verify_test_bot", testBotID, nil, nil, nil,
	)
	now := time.Unix(2_000_000_000, 0)
	service.now = func() time.Time { return now }
	state := store.Registrations()
	state.UnknownGroupLeaves = []settings.UnknownGroupLeave{
		{GroupID: knownChat, Title: "Configured", ExpiresAt: now.Add(-time.Second).Unix()},
		{GroupID: unknownChat, Title: "Unknown", ExpiresAt: now.Add(-time.Second).Unix()},
	}
	if _, err := store.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}

	service.handleUnknownLeaveDeadline(knownChat, "Configured")
	if left := caller.leftChats(); len(left) != 0 {
		t.Fatalf("configuration-known chat was left by unknown-group cleanup: %v", left)
	}
	if _, ok := unknownGroupLeave(store.Registrations(), knownChat); ok {
		t.Fatal("configuration-known chat kept stale unknown-group cleanup state")
	}

	service.handleUnknownLeaveDeadline(unknownChat, "Unknown")
	if left := caller.leftChats(); len(left) != 1 || left[0] != unknownChat {
		t.Fatalf("unconfigured unknown chat leaves = %v, want [%d]", left, unknownChat)
	}
}

type timerCountingContext struct {
	context.Context
	doneCalls chan struct{}
}

func (c *timerCountingContext) Done() <-chan struct{} {
	c.doneCalls <- struct{}{}
	return c.Context.Done()
}

func TestUnknownLeaveRescheduleKeepsLaterDeadlineWithoutSecondWaiter(t *testing.T) {
	const groupID = int64(-1009000001691)
	cfg, store := registrationFixture(t)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	base, cancel := context.WithCancel(context.Background())
	root := &timerCountingContext{Context: base, doneCalls: make(chan struct{}, 4)}
	service := newRegistrationService(
		root, newRegistrationBot(t, caller), store, cfg,
		"verify_test_bot", testBotID, nil, nil, nil,
	)
	t.Cleanup(func() {
		cancel()
		service.Wait()
	})
	later := time.Now().Add(time.Hour)
	earlier := later.Add(-time.Minute)
	latest := later.Add(time.Minute)

	service.scheduleUnknownLeave(groupID, "Unknown", later)
	select {
	case <-root.doneCalls:
	case <-time.After(time.Second):
		t.Fatal("initial unknown-group leave did not start its timer waiter")
	}

	service.scheduleUnknownLeave(groupID, "Unknown", earlier)
	select {
	case <-root.doneCalls:
		t.Fatal("earlier unknown-group leave reschedule started a second timer waiter")
	case <-time.After(20 * time.Millisecond):
	}
	service.waitingMu.Lock()
	got := service.waiting[groupID]
	service.waitingMu.Unlock()
	if !got.Equal(later) {
		t.Fatalf("earlier unknown-group leave reschedule shortened deadline to %s; want %s", got, later)
	}

	service.scheduleUnknownLeave(groupID, "Unknown", latest)
	select {
	case <-root.doneCalls:
	case <-time.After(time.Second):
		t.Fatal("later unknown-group leave reschedule did not start its replacement waiter")
	}
	service.waitingMu.Lock()
	got = service.waiting[groupID]
	service.waitingMu.Unlock()
	if !got.Equal(latest) {
		t.Fatalf("later unknown-group leave reschedule kept deadline %s; want %s", got, latest)
	}
}
