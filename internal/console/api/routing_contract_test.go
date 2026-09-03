package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	storedrules "github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const routingContractChatID int64 = -1009000000301

type routingRulesService struct{}

func (routingRulesService) ListRules(context.Context, int64, string) ([]storedrules.Record, error) {
	return []storedrules.Record{}, nil
}

func (routingRulesService) ReplaceRules(
	_ context.Context,
	_ int64,
	_ string,
	_ []storedrules.Record,
	next []storedrules.Record,
) ([]storedrules.Record, bool, error) {
	return next, true, nil
}

func (routingRulesService) UpdateRule(
	_ context.Context,
	_ int64,
	_ storedrules.Record,
	next storedrules.Record,
) (storedrules.Record, bool, error) {
	return next, true, nil
}

type routingRequest struct {
	name                string
	method              string
	path                string
	body                string
	contentType         string
	cookies             []*http.Cookie
	csrf                string
	want                int
	wrongMethodResponse int
}

type routingContractHarness struct {
	server          *Server
	managerCookies  []*http.Cookie
	operatorCookies []*http.Cookie
	managerGrant    auth.Grant
	operatorGrant   auth.Grant
	entryLink       string
	initData        string
}

func TestEveryLiveRouteRequiresItsExactMethodAndPath(t *testing.T) {
	harness := newRoutingContractHarness(t)
	for _, route := range routingContractRequests(harness) {
		t.Run(route.name, func(t *testing.T) {
			assertRoutingContract(t, harness.server, route)
		})
	}
}

func newRoutingContractHarness(t *testing.T) routingContractHarness {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken,
		Now:      func() time.Time { return now },
		OperatorAllowed: func(id int64) bool {
			return id == 42 || id == 43
		},
		AdminChecker: &apiTestAdminChecker{allowed: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	managerGrant, err := manager.IssueManagerSession(apiSignedInitData(now, 41))
	if err != nil {
		t.Fatal(err)
	}
	operatorLink, _, err := manager.IssueOperatorLink(42)
	if err != nil {
		t.Fatal(err)
	}
	operatorGrant, err := manager.RedeemOperatorLink(operatorLink)
	if err != nil {
		t.Fatal(err)
	}
	entryLink, _, err := manager.IssueOperatorLink(43)
	if err != nil {
		t.Fatal(err)
	}
	initData, err := json.Marshal(map[string]string{"init_data": apiSignedInitData(now, 44)})
	if err != nil {
		t.Fatal(err)
	}
	return routingContractHarness{
		server:          newRoutingContractServer(t, now, manager),
		managerCookies:  routingCookies(manager, managerGrant),
		operatorCookies: routingCookies(manager, operatorGrant),
		managerGrant:    managerGrant,
		operatorGrant:   operatorGrant,
		entryLink:       entryLink,
		initData:        string(initData),
	}
}

func newRoutingContractServer(t *testing.T, now time.Time, manager *auth.Manager) *Server {
	t.Helper()
	baseline, err := settings.LoadBaseline("", &settings.Config{GroupIDs: []int64{routingContractChatID}})
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := settings.NewStore("", baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	processConfig := &settings.Config{}
	return New(Config{
		Authenticator: manager,
		Verification: &apiTestQueueService{
			groups:       []int64{routingContractChatID},
			settledEntry: verification.ConsoleQueueEntry{ID: "settled", GroupID: routingContractChatID},
			auditEntry:   verification.ConsoleAuditEntry{ID: "audit", GroupID: routingContractChatID},
		},
		Settings: &apiTestSettingsService{store: settingsStore},
		Rules:    routingRulesService{},
		ProcessSettings: &apiTestProcessSettingsService{
			view: processConfig.ProcessSettings(),
		},
		Health:               diagnosticsHealth(),
		Persistence:          &apiTestPersistenceService{},
		RollbackObservations: status.NewRollbackObservations(func() time.Time { return now }),
		RollbackRejections:   &apiTestRollbackRejectionService{},
		Replacement:          &apiTestReplacementService{snapshot: status.ReplacementStatus{UnitAvailable: true}},
		Release:              &apiTestReleaseService{info: status.ReleaseInfo{Version: "v5.4.0"}},
		Version:              "v5.4.0",
		Setup:                &setupTestService{linkToken: setupTestLinkToken},
	})
}

func routingContractRequests(h routingContractHarness) []routingRequest {
	chatPath := "/api/chats/" + strconv.FormatInt(routingContractChatID, 10)
	return []routingRequest{
		{name: "liveness", method: http.MethodGet, path: "/livez", want: http.StatusOK},
		{name: "readiness", method: http.MethodGet, path: "/readyz", want: http.StatusOK},
		{name: "create session", method: http.MethodPost, path: "/api/session", body: h.initData, contentType: "application/json", want: http.StatusCreated},
		{name: "current session", method: http.MethodGet, path: "/api/session", cookies: h.managerCookies, want: http.StatusOK},
		{name: "operator entry", method: http.MethodGet, path: "/enter/" + h.entryLink, want: http.StatusSeeOther},
		{name: "setup form", method: http.MethodGet, path: "/setup/" + setupTestLinkToken, want: http.StatusOK, wrongMethodResponse: http.StatusMethodNotAllowed},
		{name: "setup claim", method: http.MethodPost, path: "/setup/" + setupTestLinkToken, body: "bot_token=123%3Atoken", contentType: "application/x-www-form-urlencoded", want: http.StatusOK, wrongMethodResponse: http.StatusMethodNotAllowed},
		{name: "chat list", method: http.MethodGet, path: "/api/chats", cookies: h.managerCookies, want: http.StatusOK},
		{name: "queue list", method: http.MethodGet, path: chatPath + "/queue", cookies: h.managerCookies, want: http.StatusOK},
		{name: "queue settlement", method: http.MethodPost, path: chatPath + "/queue/challenge", body: `{"expected":{"state":"pending"},"result":{"state":"approved"}}`, contentType: "application/json", cookies: h.managerCookies, csrf: h.managerGrant.CSRFToken, want: http.StatusOK},
		{name: "settings read", method: http.MethodGet, path: chatPath + "/settings", cookies: h.managerCookies, want: http.StatusOK},
		{name: "settings patch", method: http.MethodPatch, path: chatPath + "/settings", body: `{"expected_revision":0,"changes":{}}`, contentType: "application/json", cookies: h.managerCookies, csrf: h.managerGrant.CSRFToken, want: http.StatusOK},
		{name: "rules read", method: http.MethodGet, path: chatPath + "/rules", cookies: h.managerCookies, want: http.StatusOK},
		{name: "rules replace", method: http.MethodPut, path: chatPath + "/rules", body: `{"collection":"allowlist","expected":[],"items":[]}`, contentType: "application/json", cookies: h.managerCookies, csrf: h.managerGrant.CSRFToken, want: http.StatusOK},
		{name: "rule update", method: http.MethodPut, path: chatPath + "/rules/rule-1", body: `{"expected":{"collection":"allowlist","ordinal":0,"enabled":true,"definition":{}},"item":{"collection":"allowlist","ordinal":0,"enabled":true,"definition":{}}}`, contentType: "application/json", cookies: h.managerCookies, csrf: h.managerGrant.CSRFToken, want: http.StatusOK},
		{name: "audit read", method: http.MethodGet, path: chatPath + "/audit", cookies: h.managerCookies, want: http.StatusOK},
		{name: "audit undo", method: http.MethodPost, path: chatPath + "/audit/audit-1/undo", cookies: h.managerCookies, csrf: h.managerGrant.CSRFToken, want: http.StatusOK},
		{name: "statistics", method: http.MethodGet, path: chatPath + "/stats?from=2026-09-01&to=2026-09-03&timezone=UTC", cookies: h.managerCookies, want: http.StatusOK},
		{name: "diagnostics", method: http.MethodGet, path: "/api/status", cookies: h.operatorCookies, want: http.StatusOK},
		{name: "release", method: http.MethodGet, path: "/api/status/release", cookies: h.operatorCookies, want: http.StatusOK},
		{name: "process settings", method: http.MethodGet, path: "/api/process/settings", cookies: h.operatorCookies, want: http.StatusOK},
		{name: "upgrade", method: http.MethodPost, path: "/api/status/upgrade", body: `{"version":"v5.4.0"}`, contentType: "application/json", cookies: h.operatorCookies, csrf: h.operatorGrant.CSRFToken, want: http.StatusAccepted},
	}
}

func assertRoutingContract(t *testing.T, server *Server, route routingRequest) {
	t.Helper()
	wrongMethod := http.MethodDelete
	if route.method != http.MethodGet {
		wrongMethod = http.MethodOptions
	}
	wantWrongMethod := route.wrongMethodResponse
	if wantWrongMethod == 0 {
		wantWrongMethod = http.StatusNotFound
	}
	wrongMethodResponse := routingRoundTrip(server, route, wrongMethod, route.path)
	if wrongMethodResponse.Code != wantWrongMethod {
		t.Fatalf("%s admitted method %s: status=%d want=%d", route.path, wrongMethod, wrongMethodResponse.Code, wantWrongMethod)
	}
	wrongPath := routingSuffixedPath(t, route.path)
	if route.name == "rules replace" {
		wrongPath = routingSuffixedPath(t, wrongPath)
	}
	wrongPathResponse := routingRoundTrip(server, route, route.method, wrongPath)
	if wrongPathResponse.Code != http.StatusNotFound {
		t.Fatalf("%s admitted a trailing path segment: status=%d", route.path, wrongPathResponse.Code)
	}
	validResponse := routingRoundTrip(server, route, route.method, route.path)
	if validResponse.Code != route.want {
		t.Fatalf("valid %s %s failed: status=%d body=%s", route.method, route.path, validResponse.Code, validResponse.Body.String())
	}
}

func TestChatRoutesRejectMalformedIdentifiersAndActions(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{groups: []int64{routingContractChatID}}
	server, cookies, _ := apiTestServer(t, checker, service, nil)
	valid := getAuthenticatedPath(server, cookies, "/api/chats/-1009000000301/queue")
	if valid.Code != http.StatusOK {
		t.Fatalf("valid chat route failed: status=%d code=%q", valid.Code, decodeError(valid))
	}
	for _, test := range []struct {
		path   string
		status int
		code   string
	}{
		{"/api/chats/-1009000000301", http.StatusNotFound, "not_found"},
		{"/api/chats//queue", http.StatusNotFound, "not_found"},
		{"/api/chats/-1009000000301/", http.StatusNotFound, "not_found"},
		{"/api/chats/not-a-number/queue", http.StatusBadRequest, "invalid_chat_id"},
		{"/api/chats/0/queue", http.StatusBadRequest, "invalid_chat_id"},
		{"/api/chats/-1009000000301/queue/", http.StatusNotFound, "not_found"},
		{"/api/chats/-1009000000301/audit/audit-1/not-undo", http.StatusNotFound, "not_found"},
	} {
		response := getRequestWithoutPanic(t, server, cookies, test.path, "malformed chat route")
		if response.Code != test.status || decodeError(response) != test.code {
			t.Fatalf("malformed chat route %s was admitted: status=%d code=%q", test.path, response.Code, decodeError(response))
		}
	}
}

func routingCookies(manager *auth.Manager, grant auth.Grant) []*http.Cookie {
	response := httptest.NewRecorder()
	manager.SetCookies(response, grant)
	return response.Result().Cookies()
}

func routingRoundTrip(server *Server, route routingRequest, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(route.body))
	if route.contentType != "" {
		request.Header.Set("Content-Type", route.contentType)
	}
	if route.csrf != "" {
		request.Header.Set("X-CSRF-Token", route.csrf)
	}
	for _, cookie := range route.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func routingSuffixedPath(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path += "/extra"
	return parsed.String()
}
