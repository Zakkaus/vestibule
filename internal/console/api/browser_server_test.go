package api

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const trialBrowserChatID int64 = -1009000000101

type trialBrowserSession struct {
	Cookies []trialBrowserCookie `json:"cookies"`
	ChatID  string               `json:"chat_id"`
}

type trialBrowserCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// TestQuestionTrialBrowserServer serves the actual API for the Playwright trial journey.
// It intentionally blocks until the external browser runner terminates this test process.
func TestQuestionTrialBrowserServer(t *testing.T) {
	address := os.Getenv("VESTIBULE_TRIAL_E2E_ADDRESS")
	if address == "" {
		t.Skip("VESTIBULE_TRIAL_E2E_ADDRESS is not set")
	}
	handler := newTrialBrowserHandler(t)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	t.Logf("trial browser server listening at %s", listener.Addr())
	defer func() {
		_ = httpServer.Shutdown(context.Background())
		_ = listener.Close()
	}()
	select {}
}

func newTrialBrowserHandler(t *testing.T) http.Handler {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := database.NewSettingsStore(db)
	if err := repository.SeedSettings([]settings.Record{{ChatID: trialBrowserChatID}}); err != nil {
		t.Fatal(err)
	}
	store := newTrialSettingsStoreForChat(t, trialBrowserChatID, repository)
	verificationService := newTrialBrowserVerification(t, store)
	manager, cookies := newTrialBrowserAuth(t)
	health := status.NewHealth(func(ctx context.Context) error { return db.RawDB.PingContext(ctx) })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	apiServer := New(Config{
		Authenticator: manager, Verification: verificationService, Settings: store, Health: health,
	})
	return trialBrowserMux(apiServer, cookies)
}

func newTrialBrowserVerification(t *testing.T, store *settings.Store) *verification.Service {
	t.Helper()
	enabled := true
	fallbackBuiltin := false
	fallback := []settings.ShortQuestion{{
		Q: "Name a package manager", Answers: []string{"Portage", "emerge"},
	}}
	cfg := &settings.Config{Groups: []settings.GroupConfig{{
		ID: trialBrowserChatID, Enabled: &enabled, VerifyMode: settings.ModeQuiz,
		DeliveryMode: settings.DeliveryGroup,
		Questions: []settings.Question{{
			Q: "Pick the triangle", Options: []string{"triangle", "square"}, Answer: 0,
		}},
		FallbackQuestions: &fallback, FallbackBuiltin: &fallbackBuiltin,
	}}}
	service, err := verification.New(
		store, &trialParityGateway{}, nil, cfg, &i18n.Messages, nil, verification.Identity{}, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func newTrialBrowserAuth(t *testing.T) (*auth.Manager, []trialBrowserCookie) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken, Now: func() time.Time { return now },
		AdminChecker:  &apiTestAdminChecker{allowed: true},
		RightsChecker: &apiTestAdminChecker{allowed: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.IssueManagerSession(apiSignedInitData(now, 9))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	manager.SetCookies(recorder, grant)
	cookies := make([]trialBrowserCookie, 0, len(recorder.Result().Cookies()))
	for _, cookie := range recorder.Result().Cookies() {
		cookies = append(cookies, trialBrowserCookie{Name: cookie.Name, Value: cookie.Value})
	}
	return manager, cookies
}
func trialBrowserMux(apiServer *Server, cookies []trialBrowserCookie) http.Handler {
	mux := http.NewServeMux()
	// The fixture is initialized once per test process; session reads do not reset it.
	mux.HandleFunc("/__trial/session", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writeError(writer, http.StatusNotFound, "not_found")
			return
		}
		writeJSON(writer, http.StatusOK, trialBrowserSession{
			Cookies: cookies, ChatID: strconv.FormatInt(trialBrowserChatID, 10),
		})
	})
	mux.Handle("/", apiServer.Handler())
	return mux
}
