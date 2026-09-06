package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

const titleTestChatID int64 = -1009000001981

func TestChatTitleCachesPositiveAnswersUntilExpiry(t *testing.T) {
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat": {
			{value: &telego.ChatFullInfo{Title: "Configured group"}},
			{value: &telego.ChatFullInfo{Title: "Renamed group"}},
		},
	}}
	client := newTestClient(t, caller)
	now := time.Unix(1_800_000_000, 0)
	client.titleNow = func() time.Time { return now }

	if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Configured group" {
		t.Fatalf("first title = %q, %v; want configured group", title, err)
	}
	now = now.Add(chatTitleTTL - time.Second)
	if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Configured group" {
		t.Fatalf("cached title = %q, %v; want configured group", title, err)
	}
	if calls := len(caller.methodCalls("getChat")); calls != 1 {
		t.Fatalf("cached title made %d GetChat calls, want 1", calls)
	}

	now = now.Add(2 * time.Second)
	if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Renamed group" {
		t.Fatalf("expired title = %q, %v; want renamed group", title, err)
	}
	if calls := len(caller.methodCalls("getChat")); calls != 2 {
		t.Fatalf("expired title made %d GetChat calls, want 2", calls)
	}
}

func TestChatTitleCachesFailuresBriefly(t *testing.T) {
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat": {
			{err: errors.New("telegram unavailable")},
			{value: &telego.ChatFullInfo{Title: "Recovered group"}},
		},
	}}
	client := newTestClient(t, caller)
	now := time.Unix(1_800_000_000, 0)
	client.titleNow = func() time.Time { return now }

	if _, err := client.ChatTitle(context.Background(), titleTestChatID); err == nil {
		t.Fatal("failed title lookup returned nil error")
	}
	now = now.Add(chatTitleFailTTL - time.Second)
	if _, err := client.ChatTitle(context.Background(), titleTestChatID); err == nil {
		t.Fatal("cached failed title lookup returned nil error")
	}
	if calls := len(caller.methodCalls("getChat")); calls != 1 {
		t.Fatalf("cached failure made %d GetChat calls, want 1", calls)
	}

	now = now.Add(2 * time.Second)
	if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Recovered group" {
		t.Fatalf("expired failure = %q, %v; want recovered group", title, err)
	}
}

func TestChatTitleCoalescesConcurrentMisses(t *testing.T) {
	caller := &blockingTitleCaller{started: make(chan struct{}), release: make(chan struct{})}
	client := newBlockingTitleClient(t, caller)
	results := make(chan string, 2)
	errorsSeen := make(chan error, 2)
	go func() {
		title, err := client.ChatTitle(context.Background(), titleTestChatID)
		results <- title
		errorsSeen <- err
	}()
	select {
	case <-caller.started:
	case <-time.After(time.Second):
		t.Fatal("coalesced GetChat call did not start")
	}
	waiter := &observedWaiterContext{Context: context.Background(), entered: make(chan struct{})}
	go func() {
		title, err := client.ChatTitle(waiter, titleTestChatID)
		results <- title
		errorsSeen <- err
	}()
	select {
	case <-waiter.entered:
	case <-time.After(time.Second):
		t.Fatal("second caller did not enter the coalesced wait")
	}
	close(caller.release)
	for range 2 {
		if title := <-results; title != "Concurrent group" {
			t.Fatalf("concurrent title = %q; want Concurrent group", title)
		}
		if err := <-errorsSeen; err != nil {
			t.Fatalf("concurrent title error = %v", err)
		}
	}
	if caller.callCount() != 1 {
		t.Fatalf("concurrent miss made %d GetChat calls, want 1", caller.callCount())
	}
}

func TestChatTitleCanceledInitiatorDoesNotAbortSharedLookup(t *testing.T) {
	caller := &blockingTitleCaller{started: make(chan struct{}), release: make(chan struct{})}
	client := newBlockingTitleClient(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan error, 1)
	go func() {
		_, err := client.ChatTitle(ctx, titleTestChatID)
		firstDone <- err
	}()
	<-caller.started
	waiter := &observedWaiterContext{Context: context.Background(), entered: make(chan struct{})}
	secondDone := make(chan string, 1)
	go func() {
		title, _ := client.ChatTitle(waiter, titleTestChatID)
		secondDone <- title
	}()
	<-waiter.entered
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled initiator error = %v; want context canceled", err)
	}
	close(caller.release)
	if title := <-secondDone; title != "Concurrent group" {
		t.Fatalf("live waiter title = %q; want Concurrent group", title)
	}
	if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Concurrent group" {
		t.Fatalf("cached shared title = %q, %v", title, err)
	}
	if caller.callCount() != 1 {
		t.Fatalf("shared lookup made %d GetChat calls, want 1", caller.callCount())
	}
}

func TestChatTitleCachesResolverTimeoutUntilFailureExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		caller := &blockingTitleCaller{started: make(chan struct{}), release: make(chan struct{})}
		client := newBlockingTitleClient(t, caller)
		if _, err := client.ChatTitle(context.Background(), titleTestChatID); err == nil {
			t.Fatal("timed-out lookup returned no error")
		}
		if _, err := client.ChatTitle(context.Background(), titleTestChatID); err == nil {
			t.Fatal("cached timeout returned no error")
		}
		if caller.callCount() != 1 {
			t.Fatalf("cached timeout made %d GetChat calls, want 1", caller.callCount())
		}
		close(caller.release)
		time.Sleep(chatTitleFailTTL)
		if title, err := client.ChatTitle(context.Background(), titleTestChatID); err != nil || title != "Concurrent group" {
			t.Fatalf("recovered title = %q, %v", title, err)
		}
		if caller.callCount() != 2 {
			t.Fatalf("expired timeout made %d GetChat calls, want 2", caller.callCount())
		}
	})
}

type observedWaiterContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observedWaiterContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

type blockingTitleCaller struct {
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	calls   int
}

func (c *blockingTitleCaller) Call(ctx context.Context, url string, _ *ta.RequestData) (*ta.Response, error) {
	if !strings.HasSuffix(url, "/getChat") {
		return nil, errors.New("unexpected Telegram method")
	}
	c.mu.Lock()
	c.calls++
	first := c.calls == 1
	c.mu.Unlock()
	if first {
		close(c.started)
	}
	select {
	case <-c.release:
		body, err := json.Marshal(&telego.ChatFullInfo{Title: "Concurrent group"})
		if err != nil {
			return nil, err
		}
		return &ta.Response{Ok: true, Result: body}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *blockingTitleCaller) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func newBlockingTitleClient(t *testing.T, caller *blockingTitleCaller) *Connector {
	t.Helper()
	bot, err := telego.NewBot(
		"1:"+strings.Repeat("a", 35),
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return NewConnector(bot)
}
