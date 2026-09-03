package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

type callerFunc func(context.Context, string, *ta.RequestData) (*ta.Response, error)

func (f callerFunc) Call(ctx context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	return f(ctx, url, data)
}

type silentPollingLogger struct{}

func (silentPollingLogger) Debugf(string, ...any) {}
func (silentPollingLogger) Errorf(string, ...any) {}

type observedPollingRequest struct {
	url  string
	body []byte
}

type blockingGetUpdatesCaller struct {
	requests chan observedPollingRequest
}

func (c *blockingGetUpdatesCaller) Call(
	ctx context.Context,
	url string,
	data *ta.RequestData,
) (*ta.Response, error) {
	request := observedPollingRequest{url: url, body: append([]byte(nil), data.BodyRaw...)}
	select {
	case c.requests <- request:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestPollingProgressCallerSignalsOnlyCompletedPolls(t *testing.T) {
	progress := make(chan struct{}, 1)
	calls := 0
	caller := pollingProgressCaller{
		next: callerFunc(func(context.Context, string, *ta.RequestData) (*ta.Response, error) {
			calls++
			return &ta.Response{Ok: true}, nil
		}),
		timeout: time.Second,
		progress: func() {
			progress <- struct{}{}
		},
	}
	data := &ta.RequestData{BodyRaw: []byte("{}")}
	if _, err := caller.Call(context.Background(), "https://api.telegram.org/botTOKEN/getMe", data); err != nil {
		t.Fatal(err)
	}
	select {
	case <-progress:
		t.Fatal("a non-poll API call signaled update-loop progress")
	default:
	}
	if _, err := caller.Call(context.Background(), "https://api.telegram.org/botTOKEN/getUpdates", data); err != nil {
		t.Fatal(err)
	}
	select {
	case <-progress:
	default:
		t.Fatal("a completed getUpdates call did not signal progress")
	}
	if calls != 2 {
		t.Fatalf("underlying calls = %d, want 2", calls)
	}
}

func TestPollingProgressCallerBoundsEachGetUpdatesAttempt(t *testing.T) {
	const timeout = 250 * time.Millisecond
	var (
		deadline    time.Time
		hasDeadline bool
	)
	caller := pollingProgressCaller{
		next: callerFunc(func(ctx context.Context, _ string, _ *ta.RequestData) (*ta.Response, error) {
			deadline, hasDeadline = ctx.Deadline()
			return &ta.Response{Ok: true}, nil
		}),
		timeout: timeout,
	}
	started := time.Now()
	if _, err := caller.Call(
		context.Background(),
		"https://api.telegram.org/botTOKEN/getUpdates",
		&ta.RequestData{BodyRaw: []byte("{}")},
	); err != nil {
		t.Fatal(err)
	}
	if !hasDeadline {
		t.Fatal("getUpdates call had no deadline; a wedged Bot API request would stop polling and watchdog progress forever")
	}
	if remaining := deadline.Sub(started); remaining <= 0 || remaining > timeout+100*time.Millisecond {
		t.Fatalf("getUpdates deadline was %v from call start, want a live deadline bounded by %v", remaining, timeout)
	}
}

func TestStartPollingRequestsThirtySecondLongPoll(t *testing.T) {
	caller := &blockingGetUpdatesCaller{requests: make(chan observedPollingRequest, 1)}
	bot, err := telego.NewBot(
		"123456:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		telego.WithAPICaller(caller),
		telego.WithLogger(silentPollingLogger{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	polling, err := StartPolling(ctx, bot, func(*th.BotHandler) {})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
		defer stopCancel()
		_ = polling.Stop(stopCtx)
	})

	var request observedPollingRequest
	select {
	case request = <-caller.requests:
	case <-time.After(time.Second):
		t.Fatal("long polling started without issuing getUpdates")
	}
	if !strings.HasSuffix(request.url, "/getUpdates") {
		t.Fatalf("polling called %q, want getUpdates", request.url)
	}
	var params struct {
		Timeout int `json:"timeout"`
	}
	if err := json.Unmarshal(request.body, &params); err != nil {
		t.Fatalf("decode getUpdates request: %v", err)
	}
	if params.Timeout != 30 {
		t.Fatalf("getUpdates timeout = %d seconds, want 30; zero would turn long polling into a hot request loop", params.Timeout)
	}

	cancel()
	select {
	case handlerErr := <-polling.Done():
		if handlerErr != nil {
			t.Fatalf("polling handler stopped with error: %v", handlerErr)
		}
	case <-time.After(time.Second):
		t.Fatal("polling handler did not stop after its request context was canceled")
	}
}

func TestBlockedBotAPICallDoesNotPreventHandlerShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	defer server.Close()
	defer close(release)

	progress := make(chan struct{}, 1)
	bot, err := telego.NewBot(
		"123456:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		WithPollingProgress(progress),
		telego.WithAPIServer(server.URL),
	)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan telego.Update, 1)
	handler, err := th.NewBotHandler(bot, updates)
	if err != nil {
		t.Fatal(err)
	}
	handler.Handle(func(ctx *th.Context, _ telego.Update) error {
		_, callErr := bot.GetMe(ctx.Context())
		return callErr
	})
	handlerDone := make(chan error, 1)
	go func() {
		handlerDone <- handler.Start()
	}()
	updates <- telego.Update{UpdateID: 1}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler did not enter the blocked Bot API call")
	}
	close(updates)

	stopCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := handler.StopWithContext(stopCtx); err != nil {
		t.Fatalf("handler shutdown waited on the blocked Bot API call: %v", err)
	}
	if err := <-handlerDone; err != nil {
		t.Fatal(err)
	}
}
