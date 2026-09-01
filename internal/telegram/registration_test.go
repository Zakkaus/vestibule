package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

const (
	testBotID = int64(900)
	testOwner = int64(42)
)

type synchronizedLog struct {
	mu      sync.Mutex
	text    strings.Builder
	updated chan struct{}
}

func newSynchronizedLog() *synchronizedLog {
	return &synchronizedLog{updated: make(chan struct{}, 16)}
}

func (w *synchronizedLog) Write(value []byte) (int, error) {
	w.mu.Lock()
	n, err := w.text.Write(value)
	w.mu.Unlock()
	select {
	case w.updated <- struct{}{}:
	default:
	}
	return n, err
}

func (w *synchronizedLog) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}

func waitForLog(t *testing.T, output *synchronizedLog, text string) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		if strings.Contains(output.String(), text) {
			return
		}
		select {
		case <-output.updated:
		case <-timer.C:
			t.Fatalf("timed out waiting for log %q in %q", text, output.String())
		}
	}
}

type registrationCaller struct {
	mu              sync.Mutex
	members         map[[2]int64]telego.ChatMember
	memberErrors    map[[2]int64]error
	memberRequests  [][2]int64
	sent            []telego.SendMessageParams
	sendAttempts    []telego.SendMessageParams
	sendErrors      map[int64]error
	left            []int64
	leaveAttempts   []int64
	leaveErrors     map[int64]error
	commandScopeIDs []int64
	events          chan string
	memberLookups   chan [2]int64
	lookupBlocks    map[[2]int64]<-chan struct{}
	leaveStarted    chan int64
	releaseLeave    <-chan struct{}
	botID           int64
}

func (c *registrationCaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	switch method {
	case "getChatMember":
		return c.getChatMember(data, method)
	case "sendMessage":
		return c.sendMessage(data, method)
	case "leaveChat":
		return c.leaveChat(data, method)
	case "deleteMessage":
		return registrationAPIResponse(true)
	case "setMyCommands":
		return c.setMyCommands(data)
	default:
		return nil, fmt.Errorf("unexpected Telegram method %q", method)
	}
}

func (c *registrationCaller) getChatMember(data *ta.RequestData, method string) (*ta.Response, error) {
	var params struct {
		ChatID int64 `json:"chat_id"`
		UserID int64 `json:"user_id"`
	}
	if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
		return nil, err
	}
	key := [2]int64{params.ChatID, params.UserID}
	c.mu.Lock()
	member := c.members[key]
	memberErr := c.memberErrors[key]
	c.memberRequests = append(c.memberRequests, key)
	block := c.lookupBlocks[key]
	c.mu.Unlock()
	if c.memberLookups != nil {
		c.memberLookups <- key
	}
	if memberErr != nil {
		return nil, memberErr
	}
	if member == nil {
		return nil, fmt.Errorf("no member response for chat %d user %d", params.ChatID, params.UserID)
	}
	if block != nil {
		<-block
	}
	c.signal(method)
	return registrationAPIResponse(member)
}

func (c *registrationCaller) sendMessage(data *ta.RequestData, method string) (*ta.Response, error) {
	var wire struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(data.BodyRaw, &wire); err != nil {
		return nil, err
	}
	message := telego.SendMessageParams{ChatID: telego.ChatID{ID: wire.ChatID}, Text: wire.Text}
	c.mu.Lock()
	c.sendAttempts = append(c.sendAttempts, message)
	sendErr := c.sendErrors[wire.ChatID]
	messageID := 0
	if sendErr == nil {
		c.sent = append(c.sent, message)
		messageID = len(c.sent)
	}
	c.mu.Unlock()
	if sendErr != nil {
		return nil, sendErr
	}
	c.signal(method)
	return registrationAPIResponse(&telego.Message{MessageID: messageID})
}

func (c *registrationCaller) leaveChat(data *ta.RequestData, method string) (*ta.Response, error) {
	var params struct {
		ChatID int64 `json:"chat_id"`
	}
	if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.leaveAttempts = append(c.leaveAttempts, params.ChatID)
	leaveErr := c.leaveErrors[params.ChatID]
	c.mu.Unlock()
	if c.leaveStarted != nil {
		c.leaveStarted <- params.ChatID
	}
	if leaveErr != nil {
		return nil, leaveErr
	}
	if c.releaseLeave != nil {
		<-c.releaseLeave
	}
	c.recordLeftChat(params.ChatID)
	c.signal(method)
	return registrationAPIResponse(true)
}

func (c *registrationCaller) recordLeftChat(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.left = append(c.left, chatID)
	botID := c.botID
	if botID == 0 {
		botID = testBotID
	}
	c.members[[2]int64{chatID, botID}] = &telego.ChatMemberLeft{
		Status: telego.MemberStatusLeft,
		User:   telego.User{ID: botID},
	}
}

func (c *registrationCaller) setMyCommands(data *ta.RequestData) (*ta.Response, error) {
	var params struct {
		Scope struct {
			ChatID int64 `json:"chat_id"`
		} `json:"scope"`
	}
	if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
		return nil, err
	}
	if params.Scope.ChatID != 0 {
		c.mu.Lock()
		c.commandScopeIDs = append(c.commandScopeIDs, params.Scope.ChatID)
		c.mu.Unlock()
	}
	return registrationAPIResponse(true)
}

func (c *registrationCaller) signal(method string) {
	if c.events != nil {
		c.events <- method
	}
}

func (c *registrationCaller) leftChats() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.left...)
}

func (c *registrationCaller) leaveAttemptsForTest() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.leaveAttempts...)
}

func (c *registrationCaller) membershipRequestsForTest() [][2]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][2]int64(nil), c.memberRequests...)
}

func (c *registrationCaller) messagesTo(chatID int64) []telego.SendMessageParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	var messages []telego.SendMessageParams
	for _, message := range c.sent {
		if message.ChatID.ID == chatID {
			messages = append(messages, message)
		}
	}
	return messages
}

func (c *registrationCaller) sendAttemptsTo(chatID int64) []telego.SendMessageParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	var messages []telego.SendMessageParams
	for _, message := range c.sendAttempts {
		if message.ChatID.ID == chatID {
			messages = append(messages, message)
		}
	}
	return messages
}

func registrationAPIResponse(value any) (*ta.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &ta.Response{Ok: true, Result: raw}, nil
}

func newRegistrationBot(t *testing.T, caller ta.Caller) *telego.Bot {
	t.Helper()
	bot, err := telego.NewBot("1:"+strings.Repeat("a", 35), telego.WithAPICaller(caller), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	return bot
}

func runRegistrationUpdate(t *testing.T, bot *telego.Bot, service *registrationService, update telego.Update) {
	t.Helper()
	updates := make(chan telego.Update, 1)
	handler, err := th.NewBotHandler(bot, updates)
	if err != nil {
		t.Fatal(err)
	}
	processed := make(chan error, 1)
	handler.Use(func(ctx *th.Context, update telego.Update) error {
		err := ctx.Next(update)
		processed <- err
		return err
	})
	service.Register(handler)
	started := make(chan error, 1)
	go func() { started <- handler.Start() }()
	updates <- update
	if err := <-processed; err != nil {
		t.Fatalf("handler returned %v", err)
	}
	close(updates)
	if err := <-started; err != nil {
		t.Fatalf("handler returned %v", err)
	}
}
func TestRegistrationGlobalDispatch(t *testing.T) {
	const (
		knownGroup   int64 = -1009000000901
		unknownGroup int64 = -1009000000902
		userID       int64 = 901
	)
	cfg, store := registrationFixture(t)
	registration := store.Registrations()
	registration.RegisteredGroups = []settings.RegisteredGroup{{ID: knownGroup, RegisteredBy: testOwner}}
	if _, err := store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	service := newRegistrationService(
		context.Background(), newRegistrationBot(t, &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}),
		store, cfg, "test_bot", testBotID, nil, nil, nil,
	)

	privateStart := func(payload string) telego.Update {
		text := "/start"
		if payload != "" {
			text += " " + payload
		}
		return telego.Update{Message: &telego.Message{
			Chat: telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
			From: &telego.User{ID: userID},
			Text: text,
		}}
	}
	membership := func(groupID int64) telego.Update {
		return telego.Update{MyChatMember: &telego.ChatMemberUpdated{
			Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup},
			From: telego.User{ID: testOwner},
			OldChatMember: &telego.ChatMemberLeft{
				Status: telego.MemberStatusLeft,
				User:   telego.User{ID: testBotID},
			},
			NewChatMember: &telego.ChatMemberMember{
				Status: telego.MemberStatusMember,
				User:   telego.User{ID: testBotID},
			},
		}}
	}

	fallbackHandler := th.Handler(func(_ *th.Context, _ telego.Update) error { return nil })
	tests := []struct {
		name   string
		update telego.Update
		want   string
	}{
		{name: "owner payload", update: privateStart("owner_nonce"), want: handlerFunctionName(service.onOwnerClaim)},
		{name: "enrollment payload", update: privateStart("enroll_nonce"), want: handlerFunctionName(service.onEnrollmentStart)},
		{name: "panel payload", update: privateStart("panel_token"), want: handlerFunctionName(fallbackHandler)},
		{name: "scoped verification payload", update: privateStart("verify_-100"), want: handlerFunctionName(fallbackHandler)},
		{name: "bare verification payload", update: privateStart("verify"), want: handlerFunctionName(fallbackHandler)},
		{name: "no payload", update: privateStart(""), want: handlerFunctionName(fallbackHandler)},
		{name: "unknown group membership", update: membership(unknownGroup), want: handlerFunctionName(service.onMyChatMember)},
		{name: "known group membership", update: membership(knownGroup), want: handlerFunctionName(service.onEffectiveMembershipUpdate)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handled := registrationDispatchResult(t, service, fallbackHandler, test.update)
			if !reflect.DeepEqual(handled, []string{test.want}) {
				t.Fatalf("handlers = %v, want only %v", handled, test.want)
			}
		})
	}
}

func registrationDispatchResult(
	t *testing.T,
	service *registrationService,
	fallback th.Handler,
	update telego.Update,
) []string {
	t.Helper()
	handler, err := th.NewBotHandler(service.bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	var handled []string
	for _, route := range service.handlerRoutes() {
		actual := handlerFunctionName(route.handler)
		handler.Handle(func(_ *th.Context, _ telego.Update) error {
			handled = append(handled, actual)
			return nil
		}, route.predicates...)
	}
	handler.Handle(func(_ *th.Context, _ telego.Update) error {
		handled = append(handled, handlerFunctionName(fallback))
		return nil
	}, th.CommandEqual("start"))
	if err := handler.BaseGroup().HandleUpdate(context.Background(), service.bot, update); err != nil {
		t.Fatal(err)
	}
	return handled
}

func handlerFunctionName(handler th.Handler) string {
	return runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
}

func waitForRegistrationMethod(t *testing.T, caller *registrationCaller, method string) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case got := <-caller.events:
			if got == method {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for Telegram method %q", method)
		}
	}
}

func loadRuntimeState(configPath, stateDirectory string) (*settings.Config, *settings.Store, error) {
	cfg, err := settings.LoadConfig(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	settingsPath := ""
	if stateDirectory != "" {
		if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
			log.Printf("WARNING: cannot create STATE_DIRECTORY %q (%v) — persistence will not work", stateDirectory, err)
		}
		store.ReclaimTemps(stateDirectory)
		settingsPath = filepath.Join(stateDirectory, "settings.json")
	}
	baseline, err := settings.LoadBaseline(configPath, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("settings baseline: %w", err)
	}
	settings, err := settings.NewStore(settingsPath, baseline, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: %w", err)
	}
	return cfg, settings, nil
}

func registrationFixture(t *testing.T) (*settings.Config, *settings.Store) {
	t.Helper()
	missingConfig := t.TempDir() + "/missing-config.json"
	cfg, settings, err := loadRuntimeState(missingConfig, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return cfg, settings
}

func registrationStateFromDisk(t *testing.T, configPath, stateDirectory string) settings.RegistrationState {
	t.Helper()
	_, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	return settings.Registrations()
}

func assertRegistrationStateEqual(t *testing.T, got, want settings.RegistrationState) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("registration state = %+v, want %+v", got, want)
	}
}

func bindTestOwner(t *testing.T, settings *settings.Store, now time.Time) {
	t.Helper()
	nonce, _, err := settings.EnsureOwnerClaim(now, ownerClaimLifetime)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.ClaimOwner(testOwner, nonce, now); err != nil {
		t.Fatal(err)
	}
}

func adminMember(userID int64) telego.ChatMember {
	return &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator,
		User:   telego.User{ID: userID},
	}
}

func plainMember(userID int64) telego.ChatMember {
	return &telego.ChatMemberMember{
		Status: telego.MemberStatusMember,
		User:   telego.User{ID: userID},
	}
}
