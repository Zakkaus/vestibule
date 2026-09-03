package telegram

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func TestPrepareUpdateHandlerRegistersBeforePolling(t *testing.T) {
	source := make(chan telego.Update, 1)
	source <- telego.Update{UpdateID: 1}
	close(source)
	handled := make(chan int, 1)
	var registeredHandler *th.BotHandler
	handler, handlerDone, err := prepareUpdateHandler(
		context.Background(),
		&telego.Bot{},
		func(handler *th.BotHandler) {
			registeredHandler = handler
			handler.Handle(func(_ *th.Context, update telego.Update) error {
				handled <- update.UpdateID
				return nil
			})
		},
		func() (<-chan telego.Update, error) {
			if registeredHandler == nil {
				t.Fatal("long polling started before handlers were registered")
			}
			if !registeredHandler.IsRunning() {
				t.Fatal("long polling started before the handler was consuming updates")
			}
			return source, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if handler == nil {
		t.Fatal("prepareUpdateHandler returned a nil handler")
	}
	if got := <-handled; got != 1 {
		t.Fatalf("handled update ID = %d, want 1", got)
	}
	if err := <-handlerDone; err != nil {
		t.Fatal(err)
	}
	if err := handler.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareUpdateHandlerRollsBackRunningHandlerWhenPollingStartFails(t *testing.T) {
	startErr := errors.New("polling unavailable")

	t.Run("failed polling start", func(t *testing.T) {
		var started *th.BotHandler
		handler, done, err := prepareUpdateHandler(
			context.Background(),
			&telego.Bot{},
			func(handler *th.BotHandler) {
				started = handler
			},
			func() (<-chan telego.Update, error) {
				return nil, startErr
			},
		)
		if !errors.Is(err, startErr) {
			t.Fatalf("polling start error = %v, want %v; callers would continue after a failed poll start", err, startErr)
		}
		if handler != nil || done != nil {
			t.Fatalf("failed polling start returned handler %p and done %v; neither may escape a rolled-back start", handler, done)
		}
		if started == nil {
			t.Fatal("failed polling start did not reach the started handler")
		}
		if started.IsRunning() {
			if stopErr := started.Stop(); stopErr != nil {
				t.Fatalf("cleanup after leaked update handler: %v", stopErr)
			}
			t.Fatal("failed polling start left the update handler running; a retry would create a second consumer")
		}
	})

	t.Run("valid polling start", func(t *testing.T) {
		source := make(chan telego.Update)
		close(source)
		handler, done, err := prepareUpdateHandler(
			context.Background(),
			&telego.Bot{},
			func(*th.BotHandler) {},
			func() (<-chan telego.Update, error) {
				return source, nil
			},
		)
		if err != nil {
			t.Fatalf("valid polling start failed: %v", err)
		}
		if handler == nil || done == nil {
			t.Fatal("valid polling start did not return its running handler")
		}
		select {
		case handlerErr := <-done:
			if handlerErr != nil {
				t.Fatalf("valid polling handler stopped with error: %v", handlerErr)
			}
		case <-time.After(time.Second):
			t.Fatal("valid polling source closed but its handler did not stop")
		}
		if err := handler.Stop(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestForwardUpdatesDrainsConfirmedBufferedUpdateOnStartupCancellation(t *testing.T) {
	ctx := newBlockingCancelContext()
	update := telego.Update{UpdateID: 41}
	source := make(chan telego.Update, 1)
	source <- update
	laterPollConfirmed := make(chan struct{})
	close(laterPollConfirmed)
	handled := make(chan int, 1)
	var registeredHandler *th.BotHandler

	handler, _, err := prepareUpdateHandler(
		ctx,
		&telego.Bot{},
		func(handler *th.BotHandler) {
			registeredHandler = handler
			handler.Handle(func(_ *th.Context, update telego.Update) error {
				handled <- update.UpdateID
				return nil
			})
		},
		func() (<-chan telego.Update, error) {
			<-laterPollConfirmed
			return source, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	waitForBufferedSourceRead(source)
	ctx.cancel()
	close(source)

	var handlerDone <-chan error
	if !registeredHandler.IsRunning() {
		done := make(chan error, 1)
		handlerDone = done
		go func() {
			done <- handler.Start()
		}()
	}
	select {
	case got := <-handled:
		if got != update.UpdateID {
			t.Fatalf("handled update ID = %d, want %d", got, update.UpdateID)
		}
	case err := <-handlerDone:
		t.Fatalf("handler stopped before processing confirmed buffered update: %v", err)
	}
	if err := handler.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestForwardUpdatesDrainsConfirmedBufferedUpdateOnShutdown(t *testing.T) {
	ctx := newBlockingCancelContext()
	update := telego.Update{UpdateID: 73}
	source := make(chan telego.Update, 1)
	source <- update
	laterPollConfirmed := make(chan struct{})
	close(laterPollConfirmed)
	destination := make(chan telego.Update)
	inFlight := make(chan struct{}, 1)
	inFlight <- struct{}{}
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		forwardUpdates(ctx, source, destination, inFlight)
	}()

	<-laterPollConfirmed
	waitForBufferedSourceRead(source)
	ctx.cancel()
	close(source)
	<-inFlight

	got, ok := <-destination
	if !ok {
		t.Fatal("confirmed buffered update was discarded during shutdown")
	}
	if got.UpdateID != update.UpdateID {
		t.Fatalf("forwarded update ID = %d, want %d", got.UpdateID, update.UpdateID)
	}
	<-inFlight
	<-forwardDone
}

func TestForwardUpdatesDrainsConfirmedUpdateAfterCancellationWinsSourceWait(t *testing.T) {
	ctx := newBlockingCancelContext()
	source := make(chan telego.Update, 1)
	destination := make(chan telego.Update, 1)
	inFlight := make(chan struct{}, 1)
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		forwardUpdates(ctx, source, destination, inFlight)
	}()

	ctx.cancel()
	update := telego.Update{UpdateID: 89}
	source <- update
	close(source)

	select {
	case got, ok := <-destination:
		if !ok {
			t.Fatal("cancellation won before the source read and discarded a confirmed update")
		}
		if got.UpdateID != update.UpdateID {
			t.Fatalf("forwarded update ID = %d, want %d", got.UpdateID, update.UpdateID)
		}
	case <-time.After(time.Second):
		t.Fatal("confirmed update remained blocked after cancellation won the source wait")
	}
	<-inFlight
	select {
	case <-forwardDone:
	case <-time.After(time.Second):
		t.Fatal("forwarder did not finish after draining the canceled source")
	}
}

func TestForwardUpdatesDeliversCurrentUpdateWhenCancellationInterruptsBlockedSend(t *testing.T) {
	ctx := newBlockingCancelContext()
	update := telego.Update{UpdateID: 97}
	source := make(chan telego.Update, 1)
	source <- update
	destination := make(chan telego.Update)
	inFlight := make(chan struct{}, 1)
	forwardDone := make(chan struct{})
	go func() {
		defer close(forwardDone)
		forwardUpdates(ctx, source, destination, inFlight)
	}()

	waitForBufferedSourceRead(source)
	waitForInFlightPermit(inFlight)
	ctx.cancel()
	close(source)

	select {
	case got, ok := <-destination:
		if !ok {
			t.Fatal("cancellation interrupted a blocked send and discarded the update already taken from Telegram")
		}
		if got.UpdateID != update.UpdateID {
			t.Fatalf("forwarded update ID = %d, want %d", got.UpdateID, update.UpdateID)
		}
	case <-time.After(time.Second):
		t.Fatal("update already taken from Telegram remained blocked after cancellation")
	}
	<-inFlight
	select {
	case <-forwardDone:
	case <-time.After(time.Second):
		t.Fatal("forwarder did not finish after delivering the interrupted send")
	}
}

type blockingCancelContext struct {
	done     chan struct{}
	canceled atomic.Bool
}

func newBlockingCancelContext() *blockingCancelContext {
	return &blockingCancelContext{done: make(chan struct{})}
}

func (c *blockingCancelContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (c *blockingCancelContext) Done() <-chan struct{} {
	return c.done
}

func (c *blockingCancelContext) Err() error {
	if c.canceled.Load() {
		return context.Canceled
	}
	return nil
}

func (c *blockingCancelContext) Value(any) any {
	return nil
}

func (c *blockingCancelContext) cancel() {
	c.canceled.Store(true)
	c.done <- struct{}{}
}

func waitForBufferedSourceRead(source <-chan telego.Update) {
	for len(source) != 0 {
		runtime.Gosched()
	}
}

func waitForInFlightPermit(inFlight <-chan struct{}) {
	for len(inFlight) == 0 {
		runtime.Gosched()
	}
}

func TestUpdateHandlerConcurrencyIsBounded(t *testing.T) {
	const (
		handlerCap = 64
		updateN    = handlerCap + 16
	)
	source := make(chan telego.Update, updateN)
	for id := range updateN {
		source <- telego.Update{UpdateID: id + 1}
	}
	close(source)

	started := make(chan struct{}, updateN)
	release := make(chan struct{})
	handler, handlerDone, err := prepareUpdateHandler(
		context.Background(),
		&telego.Bot{},
		func(handler *th.BotHandler) {
			handler.Handle(func(_ *th.Context, _ telego.Update) error {
				started <- struct{}{}
				<-release
				return nil
			})
		},
		func() (<-chan telego.Update, error) {
			return source, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for range handlerCap {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("handler concurrency never reached its bound")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d update handlers ran concurrently", handlerCap)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-handlerDone; err != nil {
		t.Fatal(err)
	}
	if err := handler.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateHandlerErrorsReturnInFlightPermit(t *testing.T) {
	const updateN = maxConcurrentUpdateHandlers + 1
	source := make(chan telego.Update, updateN)
	for id := range updateN {
		source <- telego.Update{UpdateID: id + 1}
	}
	close(source)

	bot, err := telego.NewBot(
		"123456:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		telego.WithLogger(silentPollingLogger{}),
	)
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, updateN)
	handler, handlerDone, err := prepareUpdateHandler(
		context.Background(),
		bot,
		func(handler *th.BotHandler) {
			handler.Handle(func(_ *th.Context, _ telego.Update) error {
				started <- struct{}{}
				return errors.New("handler failed")
			})
		},
		func() (<-chan telego.Update, error) {
			return source, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = handler.StopWithContext(stopCtx)
	})
	for count := range updateN {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("only %d of %d updates started after handler errors; failed handlers retained in-flight permits and stalled polling", count, updateN)
		}
	}
	if err := <-handlerDone; err != nil {
		t.Fatal(err)
	}
	if err := handler.Stop(); err != nil {
		t.Fatal(err)
	}
}
