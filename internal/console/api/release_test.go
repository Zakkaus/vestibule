package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/migrations"
)

type apiTestReleaseService struct {
	info  status.ReleaseInfo
	err   error
	calls int
}

func (service *apiTestReleaseService) Latest(context.Context) (status.ReleaseInfo, error) {
	service.calls++
	return service.info, service.err
}

func TestGetLatestReleaseReturnsStructuredRollbackAssessment(t *testing.T) {
	publishedAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	service := &apiTestReleaseService{info: status.ReleaseInfo{
		Version: "v5.2.0", URL: "https://github.com/Zakkaus/vestibule/releases/tag/v5.2.0",
		Notes: "Release notes", PublishedAt: publishedAt, UpdateAvailable: true,
		Rollback: &status.ReleaseRollback{
			Available: false, Reason: migrations.RollbackIncompatible,
			TargetSchemaVersion: 3, RetainedSchemaVersion: 2, MinimumRollbackSchemaVersion: 3,
		},
	}}
	server, cookies := releaseTestServer(t, auth.RoleOperator, service)
	response := releaseRequest(server, cookies)
	var body releaseResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || service.calls != 1 || body.Version != "v5.2.0" ||
		body.Rollback == nil || body.Rollback.Available || body.Rollback.Reason != string(migrations.RollbackIncompatible) ||
		body.Rollback.TargetSchemaVersion != 3 || body.Rollback.RetainedSchemaVersion != 2 ||
		body.Rollback.MinimumRollbackSchemaVersion != 3 {
		t.Fatalf("status=%d calls=%d body=%+v", response.Code, service.calls, body)
	}
}

func TestGetLatestReleaseRejectsManagerBeforeLookup(t *testing.T) {
	service := &apiTestReleaseService{}
	server, cookies := releaseTestServer(t, auth.RoleManager, service)
	response := releaseRequest(server, cookies)
	if response.Code != http.StatusForbidden || decodeError(response) != "release_lookup_access_denied" || service.calls != 0 {
		t.Fatalf("status=%d code=%s calls=%d, want 403, release_lookup_access_denied, 0", response.Code, decodeError(response), service.calls)
	}
}

func TestGetLatestReleaseFailsClosedWithoutService(t *testing.T) {
	server, cookies := releaseTestServer(t, auth.RoleOperator, nil)
	response := releaseRequest(server, cookies)
	if response.Code != http.StatusServiceUnavailable || decodeError(response) != "release_lookup_unavailable" {
		t.Fatalf("status=%d code=%s, want 503, release_lookup_unavailable", response.Code, decodeError(response))
	}
}

func releaseTestServer(t *testing.T, role auth.Role, service ReleaseService) (*Server, []*http.Cookie) {
	t.Helper()
	server, cookies := diagnosticsTestServer(t, role, diagnosticsHealth(), &apiTestPersistenceService{})
	authenticator := server.routes.Load().server.authenticator
	server.ReplaceRoutes(Config{Authenticator: authenticator, Release: service})
	return server, cookies
}

func releaseRequest(server *Server, cookies []*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/status/release", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
