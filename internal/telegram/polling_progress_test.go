package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
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
