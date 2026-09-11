package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/status"
)

type apiDailyService struct {
	value    status.DailyStatus
	setCalls int
}

func (s *apiDailyService) Status(_ context.Context) (status.DailyStatus, error) {
	return s.value, nil
}

func (s *apiDailyService) SetEnabled(_ context.Context, enabled bool) error {
	s.setCalls++
	s.value.Enabled = enabled
	return nil
}

type closeAfterSetDailyService struct {
	delegate DailyService
	closeDB  func() error
}

func (s *closeAfterSetDailyService) Status(ctx context.Context) (status.DailyStatus, error) {
	return s.delegate.Status(ctx)
}

func (s *closeAfterSetDailyService) SetEnabled(ctx context.Context, enabled bool) error {
	if err := s.delegate.SetEnabled(ctx, enabled); err != nil {
		return err
	}
	return s.closeDB()
}

func newDailyAPIServer(t *testing.T, role auth.Role, daily DailyService) (*Server, []*http.Cookie, string) {
	t.Helper()
	server, cookies := diagnosticsTestServer(t, role, diagnosticsHealth(), &apiTestPersistenceService{})
	authenticator := server.routes.Load().server.authenticator
	server.ReplaceRoutes(Config{Authenticator: authenticator, Daily: daily})
	sessionResult := getAuthenticatedPath(server, cookies, "/api/session")
	var session sessionResponse
	if err := json.Unmarshal(sessionResult.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if sessionResult.Code != http.StatusOK || session.CSRFToken == "" {
		t.Fatalf("session response=%d csrf=%q", sessionResult.Code, session.CSRFToken)
	}
	return server, cookies, session.CSRFToken
}

func dailyAPIRequest(server *Server, cookies []*http.Cookie, method, body, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/status/daily", strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func TestDailyAPIPersistsSwitchAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	config := database.Config{StateDirectory: t.TempDir()}
	db, err := database.Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	daily := status.NewDailyService(status.DailyConfig{
		Store: database.NewDailyStatusStore(db), Location: time.UTC,
	})
	server, cookies, csrf := newDailyAPIServer(t, auth.RoleOperator, daily)
	patch := dailyAPIRequest(server, cookies, http.MethodPatch, `{"enabled":false}`, csrf)
	if patch.Code != http.StatusOK {
		t.Fatalf("PATCH daily status=%d, want 200: %s", patch.Code, patch.Body.String())
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	failed := dailyAPIRequest(server, cookies, http.MethodPatch, `{"enabled":true}`, csrf)
	if failed.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed database write status=%d, want 503", failed.Code)
	}
	db, err = database.Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	server.ReplaceRoutes(Config{
		Authenticator: server.routes.Load().server.authenticator,
		Daily: status.NewDailyService(status.DailyConfig{
			Store: database.NewDailyStatusStore(db), Location: time.UTC,
		}),
	})
	read := dailyAPIRequest(server, cookies, http.MethodGet, "", "")
	var body dailyResponse
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || body.Enabled || body.Time != "09:00" || body.Timezone != "UTC" {
		t.Fatalf("daily settings after reopen=%+v status=%d; failed writes must not enable delivery", body, read.Code)
	}
}

func TestDailyAPIPatchReturnsCommittedStateWhenDatabaseClosesAfterSet(t *testing.T) {
	ctx := context.Background()
	config := database.Config{StateDirectory: t.TempDir()}
	db, err := database.Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	base := status.NewDailyService(status.DailyConfig{
		Store: database.NewDailyStatusStore(db), Location: time.UTC,
	})
	daily := &closeAfterSetDailyService{
		delegate: base,
		closeDB:  db.Close,
	}
	server, cookies, csrf := newDailyAPIServer(t, auth.RoleOperator, daily)
	patch := dailyAPIRequest(server, cookies, http.MethodPatch, `{"enabled":false}`, csrf)
	var patchBody dailyResponse
	if err := json.Unmarshal(patch.Body.Bytes(), &patchBody); err != nil {
		t.Fatal(err)
	}
	if patch.Code != http.StatusOK || patchBody.Enabled || patchBody.Time != "09:00" || patchBody.Timezone != "UTC" {
		t.Fatalf("PATCH after committed close response=%d body=%+v, want 200 and committed metadata", patch.Code, patchBody)
	}

	db, err = database.Open(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	server.ReplaceRoutes(Config{
		Authenticator: server.routes.Load().server.authenticator,
		Daily: status.NewDailyService(status.DailyConfig{
			Store: database.NewDailyStatusStore(db), Location: time.UTC,
		}),
	})
	read := dailyAPIRequest(server, cookies, http.MethodGet, "", "")
	var readBody dailyResponse
	if err := json.Unmarshal(read.Body.Bytes(), &readBody); err != nil {
		t.Fatal(err)
	}
	if read.Code != http.StatusOK || readBody.Enabled {
		t.Fatalf("reopened daily status=%d body=%+v, want committed disabled state", read.Code, readBody)
	}
}

func TestDailyAPIEnforcesOperatorCSRFAndStrictPayload(t *testing.T) {
	daily := &apiDailyService{value: status.DailyStatus{Enabled: true, Time: "09:00", Timezone: "UTC+8"}}
	server, cookies, csrf := newDailyAPIServer(t, auth.RoleOperator, daily)
	anonymous := dailyAPIRequest(server, nil, http.MethodGet, "", "")
	if anonymous.Code != http.StatusUnauthorized || decodeError(anonymous) != "authentication_expired" {
		t.Fatalf("anonymous daily GET=%d %s, want authentication failure", anonymous.Code, anonymous.Body.String())
	}
	cases := []struct {
		name string
		body string
		code string
	}{
		{name: "missing", body: `{}`, code: "invalid_request"},
		{name: "null", body: `{"enabled":null}`, code: "invalid_request"},
		{name: "unknown", body: `{"enabled":false,"other":true}`, code: "invalid_json"},
		{name: "trailing", body: `{"enabled":false}{}`, code: "invalid_json"},
		{name: "nonboolean", body: `{"enabled":"false"}`, code: "invalid_json"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := dailyAPIRequest(server, cookies, http.MethodPatch, test.body, csrf)
			if response.Code != http.StatusBadRequest || decodeError(response) != test.code {
				t.Fatalf("body=%s response=%d code=%q, want 400 %q", test.body, response.Code, decodeError(response), test.code)
			}
		})
	}
	if daily.setCalls != 0 {
		t.Fatalf("invalid daily requests wrote %d times", daily.setCalls)
	}
	withoutCSRF := dailyAPIRequest(server, cookies, http.MethodPatch, `{"enabled":false}`, "")
	if withoutCSRF.Code != http.StatusForbidden || decodeError(withoutCSRF) != "csrf_invalid" || daily.setCalls != 0 {
		t.Fatalf("without CSRF response=%d code=%q writes=%d", withoutCSRF.Code, decodeError(withoutCSRF), daily.setCalls)
	}
}

func TestDailyAPIRejectsManagerAndUnavailableStore(t *testing.T) {
	managerDaily := &apiDailyService{value: status.DailyStatus{Enabled: true, Time: "09:00", Timezone: "UTC+8"}}
	manager, cookies, csrf := newDailyAPIServer(t, auth.RoleManager, managerDaily)
	denied := dailyAPIRequest(manager, cookies, http.MethodGet, "", "")
	if denied.Code != http.StatusForbidden || decodeError(denied) != "diagnostics_access_denied" {
		t.Fatalf("manager GET response=%d code=%q", denied.Code, decodeError(denied))
	}
	denied = dailyAPIRequest(manager, cookies, http.MethodPatch, `{"enabled":false}`, csrf)
	if denied.Code != http.StatusForbidden || decodeError(denied) != "diagnostics_access_denied" || managerDaily.setCalls != 0 {
		t.Fatalf("manager PATCH response=%d code=%q writes=%d", denied.Code, decodeError(denied), managerDaily.setCalls)
	}
	unavailable, unavailableCookies, _ := newDailyAPIServer(t, auth.RoleOperator, nil)
	got := dailyAPIRequest(unavailable, unavailableCookies, http.MethodGet, "", "")
	if got.Code != http.StatusServiceUnavailable || decodeError(got) != "diagnostics_unavailable" {
		t.Fatalf("unavailable GET response=%d code=%q", got.Code, decodeError(got))
	}
}

func TestDailyAPIRejectsReadOnlyPersistenceWithoutChangingSwitch(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.RawDB.SetMaxOpenConns(1)
	daily := status.NewDailyService(status.DailyConfig{
		Store: database.NewDailyStatusStore(db), Location: time.UTC,
	})
	server, cookies, csrf := newDailyAPIServer(t, auth.RoleOperator, daily)
	if _, err := db.Exec(ctx, "PRAGMA query_only = ON"); err != nil {
		t.Fatal(err)
	}
	patch := dailyAPIRequest(server, cookies, http.MethodPatch, `{"enabled":false}`, csrf)
	if patch.Code != http.StatusServiceUnavailable || decodeError(patch) != "diagnostics_unavailable" {
		t.Fatalf("read-only PATCH = %d %s, want persistence failure", patch.Code, patch.Body.String())
	}
	response := dailyAPIRequest(server, cookies, http.MethodGet, "", "")
	var current dailyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !current.Enabled {
		t.Fatalf("read-only failure changed switch: status=%d current=%+v", response.Code, current)
	}
}
