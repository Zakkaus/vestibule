package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
)

type apiTestReplacementService struct {
	snapshot status.ReplacementStatus
	request  error
	requests []string
}

func (s *apiTestReplacementService) Status() status.ReplacementStatus {
	return s.snapshot
}

func (s *apiTestReplacementService) Request(version string) error {
	if s.request != nil {
		return s.request
	}
	if strings.Contains(version, "://") {
		return status.ErrInvalidReplacementVersion
	}
	s.requests = append(s.requests, version)
	return nil
}

func TestGetDiagnosticsReportsReplacementUnitAndHostResult(t *testing.T) {
	replacement := &apiTestReplacementService{snapshot: status.ReplacementStatus{
		UnitAvailable: true,
		LastResult: &status.ReplacementResult{
			Status: "rolled_back", RequestedVersion: "v2.0.0", Reason: "healthcheck_failed",
		},
	}}
	server, cookies := diagnosticsTestServer(t, auth.RoleOperator, diagnosticsHealth(), &apiTestPersistenceService{}, replacement)
	response := diagnosticsRequest(server, cookies, http.MethodGet)
	body := decodeDiagnostics(t, response)

	if response.Code != http.StatusOK || !body.Replacement.UnitAvailable || body.Replacement.LastResult == nil ||
		body.Replacement.LastResult.Status != "rolled_back" ||
		body.Replacement.LastResult.RequestedVersion != "v2.0.0" ||
		body.Replacement.LastResult.Reason != "healthcheck_failed" {
		t.Fatalf("diagnostics replacement = %+v, want available rolled-back host result", body.Replacement)
	}
}

func TestRequestUpgradeWritesOnlyThroughReplacementService(t *testing.T) {
	replacement := &apiTestReplacementService{snapshot: status.ReplacementStatus{UnitAvailable: true}}
	server, cookies := diagnosticsTestServer(t, auth.RoleOperator, diagnosticsHealth(), &apiTestPersistenceService{}, replacement)
	csrf := replacementCSRFToken(t, server, cookies)

	response := postUpgrade(server, cookies, csrf, `{"version":"v2.0.0"}`)
	var body upgradeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusAccepted || body.Status != "requested" || len(replacement.requests) != 1 || replacement.requests[0] != "v2.0.0" {
		t.Fatalf("upgrade response=%d body=%+v requests=%q, want 202/requested/[v2.0.0]", response.Code, body, replacement.requests)
	}

	invalid := postUpgrade(server, cookies, csrf, `{"version":"https://attacker.example/vestibule"}`)
	if invalid.Code != http.StatusBadRequest || decodeError(invalid) != "invalid_upgrade_version" || len(replacement.requests) != 1 {
		t.Fatalf("URL request response=%d code=%q requests=%q", invalid.Code, decodeError(invalid), replacement.requests)
	}

	withURLField := postUpgrade(server, cookies, csrf, `{"version":"v3.0.0","url":"https://attacker.example/vestibule"}`)
	if withURLField.Code != http.StatusBadRequest || decodeError(withURLField) != "invalid_json" || len(replacement.requests) != 1 {
		t.Fatalf("URL field response=%d code=%q requests=%q", withURLField.Code, decodeError(withURLField), replacement.requests)
	}

	withoutCSRF := postUpgrade(server, cookies, "", `{"version":"v3.0.0"}`)
	if withoutCSRF.Code != http.StatusForbidden || decodeError(withoutCSRF) != "csrf_invalid" || len(replacement.requests) != 1 {
		t.Fatalf("no CSRF response=%d code=%q requests=%q", withoutCSRF.Code, decodeError(withoutCSRF), replacement.requests)
	}
}

func TestRequestUpgradeRequiresOperatorAndAvailableHostUnit(t *testing.T) {
	replacement := &apiTestReplacementService{request: status.ErrReplacementUnavailable}
	operator, operatorCookies := diagnosticsTestServer(t, auth.RoleOperator, diagnosticsHealth(), &apiTestPersistenceService{}, replacement)
	unavailable := postUpgrade(operator, operatorCookies, replacementCSRFToken(t, operator, operatorCookies), `{"version":"v2.0.0"}`)
	if unavailable.Code != http.StatusConflict || decodeError(unavailable) != "upgrade_unavailable" {
		t.Fatalf("unavailable upgrade response=%d code=%q", unavailable.Code, decodeError(unavailable))
	}

	manager, managerCookies := diagnosticsTestServer(t, auth.RoleManager, diagnosticsHealth(), &apiTestPersistenceService{}, replacement)
	denied := postUpgrade(manager, managerCookies, replacementCSRFToken(t, manager, managerCookies), `{"version":"v2.0.0"}`)
	if denied.Code != http.StatusForbidden || decodeError(denied) != "upgrade_access_denied" {
		t.Fatalf("manager upgrade response=%d code=%q", denied.Code, decodeError(denied))
	}
	if len(replacement.requests) != 0 {
		t.Fatalf("denied requests reached replacement service: %q", replacement.requests)
	}
}

func replacementCSRFToken(t *testing.T, server *Server, cookies []*http.Cookie) string {
	t.Helper()
	response := getAuthenticatedPath(server, cookies, "/api/session")
	var session sessionResponse
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || session.CSRFToken == "" {
		t.Fatalf("session response=%d body=%+v", response.Code, session)
	}
	return session.CSRFToken
}

func postUpgrade(server *Server, cookies []*http.Cookie, csrf, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, diagnosticsPath+"/upgrade", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
