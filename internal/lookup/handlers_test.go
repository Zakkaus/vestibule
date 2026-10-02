package lookup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

type lookupTelegramResult struct {
	messageID int
	Err       error
}

type lookupTelegramCall struct {
	Method string
	Body   []byte
}

type lookupTelegramCaller struct {
	mu        sync.Mutex
	Responses map[string][]lookupTelegramResult
	Calls     []lookupTelegramCall
	Deletes   chan telego.DeleteMessageParams
	nextID    int
}

func (c *lookupTelegramCaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	body := append([]byte(nil), data.BodyRaw...)
	c.mu.Lock()
	c.Calls = append(c.Calls, lookupTelegramCall{Method: method, Body: body})
	var result lookupTelegramResult
	if queue := c.Responses[method]; len(queue) != 0 {
		result = queue[0]
		c.Responses[method] = queue[1:]
	}
	if result.messageID == 0 {
		c.nextID++
		result.messageID = 100 + c.nextID
	}
	c.mu.Unlock()
	if result.Err != nil {
		return nil, result.Err
	}
	var value any = true
	switch method {
	case "sendMessage", "sendRichMessage":
		value = &telego.Message{MessageID: result.messageID, Chat: telego.Chat{ID: -100}}
	case "deleteMessage":
		var params telego.DeleteMessageParams
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		if c.Deletes != nil {
			c.Deletes <- params
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &ta.Response{Ok: true, Result: raw}, nil
}

func (c *lookupTelegramCaller) MethodCalls(method string) []lookupTelegramCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	var calls []lookupTelegramCall
	for _, call := range c.Calls {
		if call.Method == method {
			calls = append(calls, call)
		}
	}
	return calls
}

func newLookupTestBot(t *testing.T, caller ta.Caller) *telego.Bot {
	t.Helper()
	bot, err := telego.NewBot("1:"+strings.Repeat("a", 35), telego.WithAPICaller(caller), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return bot
}

func runLookupHandler(t *testing.T, bot *telego.Bot, handler th.Handler, update telego.Update) {
	t.Helper()
	updates := make(chan telego.Update, 1)
	botHandler, err := th.NewBotHandler(bot, updates)
	if err != nil {
		t.Fatal(err)
	}
	handled := make(chan error, 1)
	botHandler.Handle(func(ctx *th.Context, update telego.Update) error {
		err := handler(ctx, update)
		handled <- err
		return err
	})
	started := make(chan error, 1)
	go func() { started <- botHandler.Start() }()
	updates <- update
	close(updates)
	if err := <-handled; err != nil {
		t.Fatalf("handler returned %v", err)
	}
	if err := <-started; err != nil {
		t.Fatalf("bot handler returned %v", err)
	}
}

func lookupMessage(text string, chatID, userID int64, chatType string) telego.Update {
	return telego.Update{Message: &telego.Message{
		MessageID: 41,
		Chat:      telego.Chat{ID: chatID, Type: chatType},
		From:      &telego.User{ID: userID, LanguageCode: "en"},
		Text:      text,
	}}
}

func sentLookupMessage(t *testing.T, caller *lookupTelegramCaller, index int) telego.SendMessageParams {
	t.Helper()
	calls := caller.MethodCalls("sendMessage")
	if index >= len(calls) {
		t.Fatalf("sendMessage call %d missing; calls = %d", index, len(calls))
	}
	var params telego.SendMessageParams
	if err := json.Unmarshal(calls[index].Body, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func withFreshNews(t *testing.T, items []NewsItem) {
	t.Helper()
	newsC.mu.Lock()
	oldItems, oldFetched, oldLoading := newsC.items, newsC.fetched, newsC.loading
	newsC.items = append([]NewsItem(nil), items...)
	newsC.fetched = time.Now()
	newsC.loading = false
	newsC.mu.Unlock()
	t.Cleanup(func() {
		newsC.mu.Lock()
		newsC.items, newsC.fetched, newsC.loading = oldItems, oldFetched, oldLoading
		newsC.mu.Unlock()
	})
}

type lookupRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn lookupRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func withLookupHTTP(t *testing.T, bodyFor func(*http.Request) (int, string)) {
	t.Helper()
	oldClient := httpClient
	httpClient = &http.Client{Transport: lookupRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		status, body := bodyFor(request)
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = oldClient })
}

func resetLookupPackageCaches(t *testing.T) {
	t.Helper()
	oldOverlays := Overlays
	PkgC.mu.Lock()
	oldPkgs, oldAvailable := PkgC.pkgs, PkgC.available
	oldFetched, oldLastAttempt, oldRefreshing := PkgC.fetched, PkgC.lastAttempt, PkgC.refreshing
	PkgC.pkgs = map[string]map[string]string{}
	PkgC.available = map[string]bool{}
	PkgC.fetched = time.Time{}
	PkgC.lastAttempt = time.Time{}
	PkgC.refreshing = false
	PkgC.mu.Unlock()
	verC.mu.Lock()
	oldVer := verC.m
	verC.m = map[string]verInfo{}
	verC.mu.Unlock()
	Overlays = nil
	t.Cleanup(func() {
		Overlays = oldOverlays
		PkgC.mu.Lock()
		PkgC.pkgs = oldPkgs
		PkgC.available = oldAvailable
		PkgC.fetched = oldFetched
		PkgC.lastAttempt = oldLastAttempt
		PkgC.refreshing = oldRefreshing
		PkgC.mu.Unlock()
		verC.mu.Lock()
		verC.m = oldVer
		verC.mu.Unlock()
	})
}
