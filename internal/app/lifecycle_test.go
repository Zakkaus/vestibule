package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/feed"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

func TestStreamEndedUnexpectedly(t *testing.T) {
	if !streamEndedUnexpectedly(nil) {
		t.Error("nil ctx error should signal an unexpected end => restart")
	}
	if streamEndedUnexpectedly(context.Canceled) {
		t.Error("a cancelled ctx is a graceful shutdown => no restart")
	}
}

func TestRetentionOutageObserverUsesDurableHeartbeatOncePerOutage(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "heartbeat.json")
	writeHeartbeatRecord(t, path, now.Add(-25*time.Hour))
	alerts := make(chan time.Duration, 2)
	observer := retentionOutageObserver{
		loadHeartbeat: heartbeatFileLoader(path),
		alert: func(outage time.Duration) {
			alerts <- outage
		},
	}

	observer.observe(now)
	if got := <-alerts; got != 25*time.Hour {
		t.Fatalf("outage = %v, want 25h", got)
	}
	observer.observe(now)
	select {
	case got := <-alerts:
		t.Fatalf("duplicate alert for the same outage: %v", got)
	default:
	}

	writeHeartbeatRecord(t, path, now.Add(-time.Hour))
	observer.observe(now)
	writeHeartbeatRecord(t, path, now.Add(-26*time.Hour))
	observer.observe(now)
	if got := <-alerts; got != 26*time.Hour {
		t.Fatalf("outage after recovery = %v, want 26h", got)
	}
}

func writeHeartbeatRecord(t *testing.T, path string, lastOnline time.Time) {
	t.Helper()
	data, err := json.Marshal(struct {
		LastOnline int64 `json:"last_online"`
	}{LastOnline: lastOnline.Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func heartbeatFileLoader(path string) func() (verification.HeartbeatRecord, error) {
	return func() (verification.HeartbeatRecord, error) {
		var heartbeat verification.HeartbeatRecord
		data, err := os.ReadFile(path)
		if err != nil {
			return heartbeat, err
		}
		err = json.Unmarshal(data, &heartbeat)
		return heartbeat, err
	}
}

type lifecycleCaller struct {
	mu        sync.Mutex
	botID     int64
	nextID    int
	sent      []telego.SendMessageParams
	approvals int
}

func (c *lifecycleCaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	switch method {
	case "getMe":
		return lifecycleAPIResponse(&telego.User{ID: c.botID, IsBot: true, Username: "lifecycle_bot"})
	case "getChat":
		var params struct {
			ChatID int64 `json:"chat_id"`
		}
		if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
			return nil, err
		}
		return lifecycleAPIResponse(&telego.ChatFullInfo{ID: params.ChatID, Type: telego.ChatTypeSupergroup})
	case "getChatMember":
		return lifecycleAPIResponse(&telego.ChatMemberAdministrator{
			Status:          telego.MemberStatusAdministrator,
			User:            telego.User{ID: c.botID, IsBot: true},
			CanPostMessages: true,
		})
	case "sendMessage":
		var params struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.nextID++
		messageID := c.nextID
		c.sent = append(c.sent, telego.SendMessageParams{ChatID: telego.ChatID{ID: params.ChatID}, Text: params.Text})
		c.mu.Unlock()
		chatType := telego.ChatTypePrivate
		if params.ChatID < 0 {
			chatType = telego.ChatTypeSupergroup
		}
		return lifecycleAPIResponse(&telego.Message{
			MessageID: messageID,
			Chat:      telego.Chat{ID: params.ChatID, Type: chatType},
			Text:      params.Text,
		})
	case "approveChatJoinRequest":
		c.mu.Lock()
		c.approvals++
		c.mu.Unlock()
		return lifecycleAPIResponse(true)
	case "deleteMessage":
		return lifecycleAPIResponse(true)
	default:
		return nil, fmt.Errorf("unexpected Telegram method %q", method)
	}
}

func lifecycleAPIResponse(value any) (*ta.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &ta.Response{Ok: true, Result: raw}, nil
}

func newLifecycleBot(t *testing.T, caller ta.Caller) *telego.Bot {
	t.Helper()
	bot, err := telego.NewBot(
		"1:"+strings.Repeat("a", 35),
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bot
}
func (c *lifecycleCaller) sentMessages() []telego.SendMessageParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]telego.SendMessageParams(nil), c.sent...)
}

func (c *lifecycleCaller) approvalCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.approvals
}

func lifecycleHandleUpdate(t *testing.T, bot *telego.Bot, handler th.Handler, update telego.Update) {
	t.Helper()
	botHandler, err := th.NewBotHandler(bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	botHandler.Handle(handler)
	if err := botHandler.BaseGroup().HandleUpdate(context.Background(), bot, update); err != nil {
		t.Fatal(err)
	}
}

const lifecycleChildEnvironment = "VERIFYBOT_TEST_UNEXPECTED_STREAM_EXIT"

type lifecycleVerificationFixture struct {
	groupID        int64
	applicantID    int64
	botID          int64
	stateDirectory string
	configPath     string
	cfg            *config.Config
	settings       *store.Settings
	caller         *lifecycleCaller
	bot            *telego.Bot
	connector      *telegram.Connector
	verification   *verification.Service
	answer         telego.Update
}

func TestRuntimeLifecycleStopRestart(t *testing.T) {
	if runUnexpectedStreamChild() {
		return
	}
	assertUnexpectedStreamExit(t)
	fixture := newLifecycleVerificationFixture(t)
	assertRetentionOutageAlert(t, fixture)
	shutdown := newLifecycleShutdown(t, fixture)
	assertLifecycleShutdownOrder(t, shutdown)
	assertRestartedVerification(t, fixture)
}

func runUnexpectedStreamChild() bool {
	if os.Getenv(lifecycleChildEnvironment) != "1" {
		return false
	}
	handlerDone := make(chan error, 1)
	handlerDone <- nil
	if err := runRuntimeLifecycle(context.Background(), runtimeLifecycle{handlerDone: handlerDone}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}

func assertUnexpectedStreamExit(t *testing.T) {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestRuntimeLifecycleStopRestart$")
	child.Env = append(os.Environ(), lifecycleChildEnvironment+"=1")
	output, err := child.CombinedOutput()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() == 0 {
		t.Fatalf("unexpected update-stream end exit = %v, output %q; want non-zero", err, output)
	}
}

func newLifecycleVerificationFixture(t *testing.T) *lifecycleVerificationFixture {
	t.Helper()
	const groupID int64 = -1009000000951
	const applicantID int64 = 951
	const botID int64 = 950
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	configData := []byte(`{"lang":"en","groups":[{"id":-1009000000951,"lang":"en","verify_mode":"kernel"}]}`)
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	caller := &lifecycleCaller{botID: botID}
	bot := newLifecycleBot(t, caller)
	connector := telegram.NewConnector(bot)
	verification := newTestVerifier(
		settings, connector, cfg,
		verification.Identity{ID: botID, Username: "lifecycle_bot"}, stateDirectory,
	)
	join := telego.Update{ChatJoinRequest: &telego.ChatJoinRequest{
		Chat:       telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup},
		From:       telego.User{ID: applicantID, FirstName: "Applicant", LanguageCode: "en"},
		UserChatID: applicantID,
	}}
	lifecycleHandleUpdate(t, bot, telegram.NewVerificationHandlers(verification, telegram.NewVerificationGateway(connector)).JoinRequest, join)
	answer := telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: applicantID, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: applicantID, LanguageCode: "en"},
		Text: "6.12.3",
	}}
	if !verification.KernelAnswerDM(applicantID, answer.Message.Text, true) {
		t.Fatal("lifecycle fixture did not establish a gradeable verification")
	}
	return &lifecycleVerificationFixture{
		groupID: groupID, applicantID: applicantID, botID: botID,
		stateDirectory: stateDirectory, configPath: configPath,
		cfg: cfg, settings: settings, caller: caller, bot: bot,
		connector: connector, verification: verification, answer: answer,
	}
}

func assertRetentionOutageAlert(t *testing.T, fixture *lifecycleVerificationFixture) {
	t.Helper()
	heartbeatPath := filepath.Join(fixture.stateDirectory, "heartbeat.json")
	writeHeartbeatRecord(t, heartbeatPath, time.Now().Add(-25*time.Hour))
	alerted := make(chan time.Duration, 1)
	observer := &retentionOutageObserver{loadHeartbeat: heartbeatFileLoader(heartbeatPath)}
	observer.alert = func(outage time.Duration) {
		alertRetentionOutage(
			context.Background(),
			fixture.bot,
			fixture.cfg,
			fixture.settings.GroupIDs(),
			outage,
		)
		alerted <- outage
	}
	before := len(fixture.caller.sentMessages())
	if _, err := (&outageAwareBot{Bot: fixture.bot, observer: observer}).GetMe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if outage := <-alerted; outage <= telegramUpdateRetention {
		t.Fatalf("retention outage = %v, want longer than %v", outage, telegramUpdateRetention)
	}
	messages := fixture.caller.sentMessages()[before:]
	want := i18n.Messages.Verification.Admin.OutageBacklog.Render(i18n.LangEN, fixture.groupID)
	if len(messages) != 1 || messages[0].Text != want {
		t.Fatalf("retention alerts = %#v, want one catalogue alert %q", messages, want)
	}
}

type lifecycleShutdown struct {
	fixture                  *lifecycleVerificationFixture
	cancel                   context.CancelFunc
	pendingPath              string
	feedPath                 string
	handlerDone              chan error
	stopEntered              chan struct{}
	releaseStop              chan struct{}
	stopReturned             chan struct{}
	registrationWaitEntered  chan struct{}
	releaseRegistrationWait  chan struct{}
	registrationWaitReturned chan struct{}
	heartbeatStopping        chan struct{}
	releaseHeartbeat         chan struct{}
	heartbeatDone            chan struct{}
	actualFeedFlushed        chan struct{}
	releaseFeed              chan struct{}
	feedDone                 chan struct{}
	verificationFlushed      chan struct{}
	notifierDone             chan error
	lifecycleResult          chan error
}

func newLifecycleShutdown(t *testing.T, fixture *lifecycleVerificationFixture) *lifecycleShutdown {
	t.Helper()
	pendingPath := filepath.Join(fixture.stateDirectory, "pending.json")
	if err := os.Remove(pendingPath); err != nil {
		t.Fatal(err)
	}
	root, cancel := context.WithCancel(context.Background())
	shutdown := &lifecycleShutdown{
		fixture: fixture, cancel: cancel, pendingPath: pendingPath,
		handlerDone: make(chan error), stopEntered: make(chan struct{}),
		releaseStop: make(chan struct{}), stopReturned: make(chan struct{}),
		registrationWaitEntered: make(chan struct{}), releaseRegistrationWait: make(chan struct{}),
		registrationWaitReturned: make(chan struct{}), heartbeatStopping: make(chan struct{}),
		releaseHeartbeat: make(chan struct{}), heartbeatDone: make(chan struct{}),
		actualFeedFlushed: make(chan struct{}), releaseFeed: make(chan struct{}),
		feedDone: make(chan struct{}), verificationFlushed: make(chan struct{}),
		notifierDone: make(chan error, 1), lifecycleResult: make(chan error, 1),
	}
	shutdown.startHeartbeatGate(root)
	shutdown.feedPath = shutdown.startFeedGate(t, root)
	shutdown.startLifecycle(root)
	return shutdown
}

func (s *lifecycleShutdown) startHeartbeatGate(root context.Context) {
	go func() {
		<-root.Done()
		close(s.heartbeatStopping)
		<-s.releaseHeartbeat
		close(s.heartbeatDone)
	}()
}

func (s *lifecycleShutdown) startFeedGate(t *testing.T, root context.Context) string {
	t.Helper()
	const feedChatID int64 = -1009000000952
	off := false
	feedConfig := &config.FeedConfig{
		ChatID: feedChatID, Lang: "en", IntervalSeconds: 60, Bugs: &off, News: &off,
	}
	feedPath := filepath.Join(s.fixture.stateDirectory, fmt.Sprintf("feed-%d.json", feedChatID))
	actualDone := make(chan struct{})
	go func() {
		feed.New(s.fixture.bot, []*config.FeedConfig{feedConfig}, s.fixture.stateDirectory).Run(root)
		close(actualDone)
	}()
	waitForLifecycleFile(t, feedPath)
	if err := os.Remove(feedPath); err != nil {
		t.Fatal(err)
	}
	go func() {
		<-actualDone
		close(s.actualFeedFlushed)
		<-s.releaseFeed
		close(s.feedDone)
	}()
	return feedPath
}

func waitForLifecycleFile(t *testing.T, path string) {
	t.Helper()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		runtime.Gosched()
	}
}

func (s *lifecycleShutdown) startLifecycle(root context.Context) {
	go func() {
		s.lifecycleResult <- runRuntimeLifecycle(root, runtimeLifecycle{
			handlerDone: s.handlerDone,
			stopHandlers: func(context.Context) error {
				close(s.stopEntered)
				<-s.releaseStop
				close(s.stopReturned)
				return nil
			},
			waitRegistration: func() {
				close(s.registrationWaitEntered)
				<-s.releaseRegistrationWait
				close(s.registrationWaitReturned)
			},
			heartbeatDone: s.heartbeatDone,
			flushVerification: func() {
				s.fixture.verification.Shutdown()
				close(s.verificationFlushed)
			},
			feedDone: s.feedDone, notifierDone: s.notifierDone, shutdownDeadline: time.Minute,
		})
	}()
}

func assertLifecycleShutdownOrder(t *testing.T, shutdown *lifecycleShutdown) {
	t.Helper()
	shutdown.cancel()
	<-shutdown.heartbeatStopping
	select {
	case <-shutdown.stopEntered:
		t.Fatal("update handlers stopped before the fetched update stream drained")
	default:
	}
	close(shutdown.handlerDone)
	<-shutdown.stopEntered
	close(shutdown.releaseStop)
	<-shutdown.stopReturned
	<-shutdown.registrationWaitEntered
	close(shutdown.releaseRegistrationWait)
	<-shutdown.registrationWaitReturned
	select {
	case <-shutdown.verificationFlushed:
		t.Fatal("verification flushed before the heartbeat stopped")
	default:
	}
	close(shutdown.releaseHeartbeat)
	<-shutdown.verificationFlushed
	assertLifecycleFlushes(t, shutdown)
	close(shutdown.releaseFeed)
	<-shutdown.feedDone
	shutdown.notifierDone <- nil
	if err := <-shutdown.lifecycleResult; err != nil {
		t.Fatal(err)
	}
}

func assertLifecycleFlushes(t *testing.T, shutdown *lifecycleShutdown) {
	t.Helper()
	if _, err := os.Stat(shutdown.pendingPath); err != nil {
		t.Fatalf("verification shutdown did not flush pending state: %v", err)
	}
	<-shutdown.actualFeedFlushed
	if _, err := os.Stat(shutdown.feedPath); err != nil {
		t.Fatalf("final feed flush did not recreate state: %v", err)
	}
	select {
	case err := <-shutdown.lifecycleResult:
		t.Fatalf("lifecycle returned before the final feed flush completed: %v", err)
	default:
	}
}

func assertRestartedVerification(t *testing.T, fixture *lifecycleVerificationFixture) {
	t.Helper()
	cfg, settings, err := loadRuntimeState(fixture.configPath, fixture.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	restarted := newTestVerifier(
		settings, fixture.connector, cfg,
		verification.Identity{ID: fixture.botID, Username: "lifecycle_bot"}, fixture.stateDirectory,
	)
	defer restarted.Shutdown()
	if !restarted.KernelAnswerDM(fixture.applicantID, fixture.answer.Message.Text, true) {
		t.Fatal("verification state did not survive the stop/start cycle")
	}
	before := fixture.caller.approvalCount()
	lifecycleHandleUpdate(t, fixture.bot, telegram.NewVerificationHandlers(restarted, telegram.NewVerificationGateway(fixture.connector)).KernelAnswer, fixture.answer)
	if got := fixture.caller.approvalCount() - before; got != 1 {
		t.Fatalf("restarted verification approvals = %d, want 1", got)
	}
}
