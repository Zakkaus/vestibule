package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	consoleapi "github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func TestNewDailyServiceUsesRuntimePersistenceAndStatsLocation(t *testing.T) {
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(telegramAPI.Close)
	fixture := newDailyAppFixture(t, telegramAPI.URL)
	defer fixture.database.Close()

	got, err := fixture.daily.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Time != "09:00" || got.Timezone != fixture.verification.StatsLocation().String() {
		t.Fatalf("assembled daily status = %+v, want enabled at stats timezone %s", got, fixture.verification.StatsLocation())
	}
	if err := fixture.daily.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	got, err = fixture.daily.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("assembled daily switch did not persist false")
	}
}

func TestNewDailyServiceSendsOnceForHTTP500And429(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var sends atomic.Int32
			telegramAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, "/sendMessage") {
					sends.Add(1)
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(code)
					_, _ = writer.Write([]byte(`{"ok":false,"error_code":` + strconv.Itoa(code) + `,"description":"failure"}`))
					return
				}
				writer.WriteHeader(http.StatusNotFound)
			}))
			t.Cleanup(telegramAPI.Close)
			fixture := newDailyAppFixture(t, telegramAPI.URL)
			defer fixture.database.Close()
			if err := fixture.daily.Check(context.Background()); err == nil {
				t.Fatalf("daily HTTP %d failure was swallowed", code)
			}
			if got := sends.Load(); got != 1 {
				t.Fatalf("HTTP %d send requests after first failure = %d, want one", code, got)
			}
			if err := fixture.daily.Check(context.Background()); err != nil {
				// The durable claim makes the same-date retry a no-op.
				t.Fatalf("same-date check after HTTP %d failure = %v, want nil", code, err)
			}
			if got := sends.Load(); got != 1 {
				t.Fatalf("HTTP %d same-date send requests = %d, want one", code, got)
			}
		})
	}
}

func TestStartDailyRuntimePathChecksImmediatelyAndCancels(t *testing.T) {
	sent := make(chan struct{})
	var recipient atomic.Int64
	var text string
	telegramAPI := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/sendMessage") {
			var body struct {
				ChatID int64  `json:"chat_id"`
				Text   string `json:"text"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			recipient.Store(body.ChatID)
			text = body.Text
			close(sent)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"ok":false,"error_code":500,"description":"failure"}`))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(telegramAPI.Close)
	fixture := newDailyAppFixture(t, telegramAPI.URL)
	t.Cleanup(func() { _ = fixture.database.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startDaily(ctx, fixture.daily)
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("runtime daily loop did not perform its immediate send")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime daily loop did not stop after cancellation")
	}
	if got := recipient.Load(); got != 9001 {
		t.Fatalf("runtime daily recipient=%d, want owner 9001", got)
	}
	labels := &i18n.Messages.Bot.Daily
	want := labels.Report.Render(i18n.LangEN, "2026-09-11",
		labels.Online.For(i18n.LangEN), labels.Ready.For(i18n.LangEN),
		labels.Never.For(i18n.LangEN), labels.NoFailure.For(i18n.LangEN), 0)
	if text != want {
		t.Fatalf("runtime report=%q, want configured English report %q", text, want)
	}
}

type blockedDailyRuntime struct {
	runtime     *startupTestServices
	active      *activeRuntime
	console     *consoleapi.Server
	entered     <-chan struct{}
	release     chan struct{}
	sendAttempt <-chan struct{}
}

func newBlockedDailyTelegramAPI(t *testing.T, sendAttempt chan struct{}) *httptest.Server {
	t.Helper()
	var sendOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.Path, "/getMe"):
			_, _ = writer.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`))
		case strings.HasSuffix(request.URL.Path, "/setMyCommands"):
			_, _ = writer.Write([]byte(`{"ok":true,"result":true}`))
		case strings.HasSuffix(request.URL.Path, "/getUpdates"):
			_, _ = writer.Write([]byte(`{"ok":true,"result":[]}`))
		case strings.HasSuffix(request.URL.Path, "/sendMessage"):
			sendOnce.Do(func() { close(sendAttempt) })
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"ok":false,"error_code":500,"description":"unexpected daily send"}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newBlockedDailyRuntime(t *testing.T, standby bool) blockedDailyRuntime {
	t.Helper()
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"lang":"en","stats_timezone":"UTC","groups":[],"modules":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sendAttempt := make(chan struct{})
	telegramAPI := newBlockedDailyTelegramAPI(t, sendAttempt)
	runtime := openStartupTestServices(t, Options{
		ConfigPath: configPath, StateDirectory: stateDirectory,
		Token: "1:" + strings.Repeat("a", 35), TelegramAPIURL: telegramAPI.URL,
	})
	registration := runtime.settings.Registrations()
	registration.OwnerID = 9001
	if _, err := runtime.settings.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	console := dailyRuntimeConsole(t, runtime)
	if standby {
		if _, err := runtime.database.Exec(context.Background(),
			"INSERT INTO update_poll_lease (singleton, holder, expires_at) VALUES (1, $1, $2)",
			"other-holder", time.Now().Add(time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	runtime.daily = newDailyService(runtime.services, runtime.verification, runtime.verificationGateway, func() time.Time {
		select {
		case <-entered:
		default:
			close(entered)
		}
		<-release
		return time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	})
	active, err := startActiveRuntime(context.Background(), runtime.services)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		stopActiveRuntime(active)
	})
	return blockedDailyRuntime{
		runtime: runtime, active: active, console: console,
		entered: entered, release: release, sendAttempt: sendAttempt,
	}
}

func dailyRuntimeConsole(t *testing.T, runtime *startupTestServices) *consoleapi.Server {
	t.Helper()
	console := consoleapi.New(claimedConsoleConfig(runtime.services))
	link, _, err := runtime.consoleAuth.IssueOperatorLink(9001)
	if err != nil {
		t.Fatal(err)
	}
	entry := httptest.NewRecorder()
	console.Handler().ServeHTTP(entry, httptest.NewRequest(http.MethodGet, "/enter/"+link, nil))
	if entry.Code != http.StatusSeeOther {
		t.Fatalf("operator entry status=%d, want %d", entry.Code, http.StatusSeeOther)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/status/daily", nil)
	for _, cookie := range entry.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	console.Handler().ServeHTTP(response, request)
	var dailyStatus struct {
		Enabled  bool   `json:"enabled"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(response.Body).Decode(&dailyStatus); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !dailyStatus.Enabled || dailyStatus.Time != "09:00" ||
		dailyStatus.Timezone != runtime.verification.StatsLocation().String() {
		t.Fatalf("claimed console daily response=%d body=%+v, want enabled 09:00 at %s",
			response.Code, dailyStatus, runtime.verification.StatsLocation())
	}
	return console
}

func TestStandbyActiveRuntimeDoesNotStartDaily(t *testing.T) {
	fixture := newBlockedDailyRuntime(t, true)
	select {
	case <-fixture.entered:
		t.Fatal("standby runtime started daily work without the polling lease")
	case <-fixture.sendAttempt:
		t.Fatal("standby runtime sent a daily report")
	case <-time.After(time.Second):
	}
	_, lastDate, err := database.NewDailyStatusStore(fixture.runtime.database).LoadDailyStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if lastDate != "" {
		t.Fatalf("standby runtime claimed a daily attempt: %q", lastDate)
	}
}

func TestStartActiveRuntimeStopsDailyWhenPollingLeaseIsLost(t *testing.T) {
	fixture := newBlockedDailyRuntime(t, false)
	select {
	case <-fixture.entered:
	case <-time.After(time.Second):
		t.Fatal("active runtime did not enter the daily clock")
	}
	if _, err := fixture.runtime.database.Exec(context.Background(),
		"UPDATE update_poll_lease SET holder=$1 WHERE singleton=1", "other-holder"); err != nil {
		t.Fatal(err)
	}
	if fixture.active.polling.renewOnce(context.Background()) {
		t.Fatal("forced lease loss unexpectedly renewed")
	}
	if err := fixture.active.context.Err(); err != nil {
		t.Fatalf("lease loss cancelled the whole active runtime: %v", err)
	}
	close(fixture.release)
	select {
	case <-fixture.active.dailyDone:
	case <-fixture.sendAttempt:
		t.Fatal("daily send started after polling lease loss")
	case <-time.After(time.Second):
		t.Fatal("daily loop did not stop after polling lease loss")
	}
	enabled, lastDate, err := database.NewDailyStatusStore(fixture.runtime.database).LoadDailyStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || lastDate != "" {
		t.Fatalf("lease-loss daily state enabled=%v date=%q, want enabled with no durable attempt", enabled, lastDate)
	}
}

func TestActiveLifecycleWaitsForBlockedDailyClock(t *testing.T) {
	fixture := newBlockedDailyRuntime(t, false)
	select {
	case <-fixture.entered:
	case <-time.After(time.Second):
		t.Fatal("active runtime did not enter the daily clock")
	}
	result := make(chan error, 1)
	go func() {
		result <- runActiveLifecycle(fixture.active, fixture.console, nil)
	}()
	if err := fixture.active.polling.polling.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.active.context.Done():
	case <-time.After(time.Second):
		t.Fatal("active lifecycle did not cancel the runtime")
	}
	select {
	case err := <-result:
		t.Fatalf("active lifecycle completed while daily clock was blocked: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(fixture.release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("unexpected handler termination was not reported")
		}
	case <-time.After(time.Second):
		t.Fatal("active lifecycle did not wait for daily completion")
	}
}

type dailyAppFixture struct {
	database     *database.Database
	verification *verification.Service
	daily        *status.DailyService
}

func newDailyAppFixture(t *testing.T, telegramAPI string) dailyAppFixture {
	t.Helper()
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"lang":"en","stats_timezone":"UTC","groups":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, runtimeSettings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	registration := runtimeSettings.Registrations()
	registration.OwnerID = 9001
	if _, err := runtimeSettings.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(context.Background(), database.Config{StateDirectory: stateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	bot, err := telego.NewBot(
		"123456:"+strings.Repeat("a", 35),
		telego.WithAPIServer(telegramAPI), telego.WithDiscardLogger(),
	)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	connector := telegram.NewConnector(bot)
	gateway := telegram.NewVerificationGateway(connector)
	verificationService, err := verification.New(
		runtimeSettings, gateway, database.NewVerificationStore(db), cfg,
		&i18n.Messages, nil, verification.Identity{}, stateDirectory, nil,
	)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	health := status.NewHealth(func(ctx context.Context) error { return db.RawDB.PingContext(ctx) })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	runtime := &services{
		cfg: cfg, database: db, settings: runtimeSettings, health: health,
		rollbackObservations: status.NewRollbackObservations(time.Now),
	}
	return dailyAppFixture{
		database: db, verification: verificationService,
		daily: newDailyService(runtime, verificationService, gateway, func() time.Time {
			return time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
		}),
	}
}

func TestDailyRuntimeObserveOnlyRecordsWithoutTelegramWrite(t *testing.T) {
	ctx := context.Background()
	stateDirectory := t.TempDir()
	timezone := fmt.Sprintf("Etc/GMT%+d", time.Now().UTC().Hour()-12)
	if _, err := time.LoadLocation(timezone); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(stateDirectory, "config.json")
	config := fmt.Sprintf(`{"lang":"en","stats_timezone":%q,"observe_only":true,"groups":[],"modules":[]}`, timezone)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	sendAttempt := make(chan struct{})
	telegramAPI := newBlockedDailyTelegramAPI(t, sendAttempt)
	runtime := openStartupTestServices(t, Options{
		ConfigPath: configPath, StateDirectory: stateDirectory,
		Token: "1:" + strings.Repeat("a", 35), TelegramAPIURL: telegramAPI.URL,
	})
	registration := runtime.settings.Registrations()
	registration.OwnerID = 9001
	if _, err := runtime.settings.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	runtime.daily.Check(ctx)
	runtime.daily.Check(ctx)
	select {
	case <-sendAttempt:
		t.Fatal("observe-only daily report escaped to Telegram")
	default:
	}
	journal, err := database.NewObservationStore(ctx, runtime.database, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := journal.LoadObservedActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Operation != verification.ObservedSend {
		t.Fatalf("daily observations=%+v, want one suppressed send", actions)
	}
}
