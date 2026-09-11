package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
)

// TestDailyAPIHarness serves the real API and proxies the frontend for an optional browser journey.
// Run it with RUN_DAILY_API_HARNESS=1 and DAILY_E2E_FRONTEND_URL set to a running frontend URL.
// It prints one DAILY_E2E_READY JSON line containing a loopback URL and one-use operator entry URL,
// then waits for SIGINT/SIGTERM before closing the server and database.
func TestDailyAPIHarness(t *testing.T) {
	if os.Getenv("RUN_DAILY_API_HARNESS") != "1" {
		t.Skip("set RUN_DAILY_API_HARNESS=1 to start the real API harness")
	}
	frontendAddress := strings.TrimSpace(os.Getenv("DAILY_E2E_FRONTEND_URL"))
	if frontendAddress == "" {
		t.Fatal("DAILY_E2E_FRONTEND_URL is required for the real API harness")
	}
	frontend, err := url.Parse(frontendAddress)
	if err != nil || frontend.Scheme == "" || frontend.Host == "" {
		t.Fatalf("invalid DAILY_E2E_FRONTEND_URL %q", frontendAddress)
	}
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	realAPI, entryToken := dailyHarnessAPI(t, db)
	proxy := httputil.NewSingleHostReverseProxy(frontend)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") || strings.HasPrefix(request.URL.Path, "/enter/") {
			realAPI.Handler().ServeHTTP(writer, request)
			return
		}
		proxy.ServeHTTP(writer, request)
	})
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	ready := struct {
		URL      string `json:"url"`
		EntryURL string `json:"entry_url"`
	}{URL: server.URL, EntryURL: server.URL + "/enter/" + entryToken}
	payload, err := json.Marshal(ready)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("DAILY_E2E_READY=%s\n", payload)

	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	select {
	case <-signalCtx.Done():
	case <-timer.C:
		t.Fatal("real API harness timed out after five minutes")
	}
}

func dailyHarnessAPI(t *testing.T, db *database.Database) (*Server, string) {
	t.Helper()
	manager, err := auth.New(auth.Config{
		BotToken:        "123:harness-token",
		OperatorAllowed: func(telegramID int64) bool { return telegramID == 9001 },
	})
	if err != nil {
		t.Fatal(err)
	}
	entryToken, _, err := manager.IssueOperatorLink(9001)
	if err != nil {
		t.Fatal(err)
	}
	observations := status.NewRollbackObservations(time.Now)
	health := status.NewHealth(func(ctx context.Context) error { return db.RawDB.PingContext(ctx) })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	daily := status.NewDailyService(status.DailyConfig{
		Store: database.NewDailyStatusStore(db), Location: time.FixedZone("UTC+8", 8*60*60),
	})
	return New(Config{
		Authenticator: manager, Verification: &apiTestQueueService{},
		Daily: daily, Health: health,
		Persistence: &apiTestPersistenceService{value: settings.PersistenceStatus{
			Configured: true, Durable: true, Writable: true,
		}},
		RollbackObservations: observations,
		RollbackRejections:   &apiTestRollbackRejectionService{},
		Version:              "harness",
	}), entryToken
}
