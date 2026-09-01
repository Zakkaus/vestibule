package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
)

const diagnosticsPath = "/api/status"

type apiTestPersistenceService struct {
	value settings.PersistenceStatus
	calls int
}

func (s *apiTestPersistenceService) Persistence() settings.PersistenceStatus {
	s.calls++
	return s.value
}

func TestGetDiagnosticsDistinguishesUnmeasuredAndZeroLatency(t *testing.T) {
	at := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name     string
		observed bool
	}{
		{name: "unmeasured"},
		{name: "measured zero", observed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			health := diagnosticsHealth()
			if test.observed {
				health.RecordTelegramProbe(at, 0)
			}
			persistence := &apiTestPersistenceService{value: settings.PersistenceStatus{
				Configured: true, Durable: true, Writable: true,
			}}
			server, cookies := diagnosticsTestServer(t, auth.RoleOperator, health, persistence)
			response := diagnosticsRequest(server, cookies, http.MethodGet)
			body := decodeDiagnostics(t, response)

			assertDiagnosticsHealth(t, response, persistence.calls, body.Health)
			assertDiagnosticsWireShape(t, response)
			assertBotAPISample(t, body.BotAPI, test.observed, at)
		})
	}
}

func assertDiagnosticsHealth(
	t *testing.T,
	response *httptest.ResponseRecorder,
	calls int,
	health diagnosticsHealthResponse,
) {
	t.Helper()
	if response.Code != http.StatusOK || calls != 1 || !health.Live || !health.Ready ||
		!health.ConfigReady || !health.TelegramReady {
		t.Fatalf("status=%d calls=%d health=%+v, want 200, 1, ready health", response.Code, calls, health)
	}
}

func assertBotAPISample(
	t *testing.T,
	botAPI diagnosticsBotAPIResponse,
	observed bool,
	at time.Time,
) {
	t.Helper()
	if !observed {
		assertUnmeasuredBotAPI(t, botAPI)
		return
	}
	assertMeasuredBotAPI(t, botAPI, at)
}

func assertUnmeasuredBotAPI(t *testing.T, botAPI diagnosticsBotAPIResponse) {
	t.Helper()
	if botAPI.LastHeartbeatAt != nil || botAPI.LatencyMilliseconds != nil {
		t.Fatalf("unmeasured bot API response = %+v, want null fields", botAPI)
	}
}

func assertMeasuredBotAPI(t *testing.T, botAPI diagnosticsBotAPIResponse, at time.Time) {
	t.Helper()
	if botAPI.LastHeartbeatAt == nil || !botAPI.LastHeartbeatAt.Equal(at) ||
		botAPI.LatencyMilliseconds == nil || *botAPI.LatencyMilliseconds != 0 {
		t.Fatalf("measured bot API response = %+v, want %v and 0 ms", botAPI, at)
	}
}

func TestGetDiagnosticsRedactsPersistenceError(t *testing.T) {
	token := "12345:" + strings.Repeat("x", 20)
	persistence := &apiTestPersistenceService{value: settings.PersistenceStatus{
		LastError: errors.New("request https://api.example.test/bot" + token + "/getMe failed"),
	}}
	server, cookies := diagnosticsTestServer(t, auth.RoleOperator, diagnosticsHealth(), persistence)
	response := diagnosticsRequest(server, cookies, http.MethodGet)
	body := decodeDiagnostics(t, response)

	if response.Code != http.StatusOK || body.Persistence.LastError == nil ||
		strings.Contains(*body.Persistence.LastError, token) ||
		!strings.Contains(*body.Persistence.LastError, "/bot<redacted>/getMe") {
		t.Fatalf("status=%d last_error=%v", response.Code, body.Persistence.LastError)
	}
}

func TestGetDiagnosticsRejectsManager(t *testing.T) {
	persistence := &apiTestPersistenceService{}
	server, cookies := diagnosticsTestServer(t, auth.RoleManager, diagnosticsHealth(), persistence)
	response := diagnosticsRequest(server, cookies, http.MethodGet)

	if response.Code != http.StatusForbidden || decodeError(response) != "diagnostics_access_denied" || persistence.calls != 0 {
		t.Fatalf("status=%d code=%s calls=%d, want 403, diagnostics_access_denied, 0", response.Code, decodeError(response), persistence.calls)
	}
}

func TestDiagnosticsHasNoWriteRoute(t *testing.T) {
	persistence := &apiTestPersistenceService{}
	server, cookies := diagnosticsTestServer(t, auth.RoleOperator, diagnosticsHealth(), persistence)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := diagnosticsRequest(server, cookies, method)
		if response.Code != http.StatusNotFound || decodeError(response) != "not_found" {
			t.Fatalf("%s status=%d code=%s, want 404, not_found", method, response.Code, decodeError(response))
		}
	}
	if persistence.calls != 0 {
		t.Fatalf("write routes called the persistence service %d times", persistence.calls)
	}
}

func diagnosticsHealth() *status.Health {
	health := status.NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	return health
}

func diagnosticsTestServer(
	t *testing.T,
	role auth.Role,
	health *status.Health,
	persistence PersistenceService,
) (*Server, []*http.Cookie) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken,
		Now:      func() time.Time { return now },
		OperatorAllowed: func(telegramID int64) bool {
			return telegramID == 9
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var grant auth.Grant
	switch role {
	case auth.RoleManager:
		grant, err = manager.IssueManagerSession(apiSignedInitData(now, 9))
	case auth.RoleOperator:
		link, _, issueErr := manager.IssueOperatorLink(9)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		grant, err = manager.RedeemOperatorLink(link)
	default:
		t.Fatalf("unsupported role %q", role)
	}
	if err != nil {
		t.Fatal(err)
	}
	cookies := httptest.NewRecorder()
	manager.SetCookies(cookies, grant)
	return New(Config{Authenticator: manager, Health: health, Persistence: persistence}), cookies.Result().Cookies()
}

func diagnosticsRequest(server *Server, cookies []*http.Cookie, method string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, diagnosticsPath, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func decodeDiagnostics(t *testing.T, response *httptest.ResponseRecorder) diagnosticsResponse {
	t.Helper()
	var body diagnosticsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertDiagnosticsWireShape(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	if len(root) != 3 || root["health"] == nil || root["bot_api"] == nil || root["persistence"] == nil {
		t.Fatalf("diagnostics JSON root = %s, want direct health, bot_api, persistence fields", response.Body.Bytes())
	}
}
