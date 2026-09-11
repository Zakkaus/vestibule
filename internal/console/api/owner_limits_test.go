package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	ownerLimitsOwnerID    int64 = 9000000101
	ownerLimitsStrangerID int64 = 9000000102
)

var ownerLimitsFields = [...]string{
	"timeout_seconds",
	"ban_seconds",
	"mute_seconds",
	"lookup_ttl_seconds",
	"verify_retry_seconds",
	"verify_max_fails",
	"warn_limit",
	"private_query_per_min",
	"questions",
	"fallback_questions",
	"channel_whitelist",
	"trusted_member_group_ids",
	"known_chat_ids",
}

type ownerLimitsTestResponse struct {
	Revision   uint64                             `json:"revision"`
	Limits     map[string]int64                   `json:"limits"`
	Violations []ownerLimitsTestViolationResponse `json:"violations"`
}

type ownerLimitsTestViolationResponse struct {
	ChatID string `json:"chat_id"`
	Field  string `json:"field"`
	Value  int64  `json:"value"`
	Limit  int64  `json:"limit"`
}

func TestOwnerLimitsOwnerManagerCanReadAndPatchWithZeroGroups(t *testing.T) {
	store := newOwnerLimitsTestStore(t, ownerLimitsOwnerID)
	manager, grant := ownerLimitsManagerSession(t, ownerLimitsOwnerID)
	server, cookies := ownerLimitsServer(t, manager, grant, store)

	read := ownerLimitsRequest(server, http.MethodGet, "/api/owner/limits", cookies, "", "")
	assertOwnerLimitsDefaults(t, read, 0)

	withoutCSRF := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, "", `{"expected_revision":0,"changes":{"timeout_seconds":300}}`)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("owner limits PATCH without CSRF status = %d, want 403; body=%s", withoutCSRF.Code, withoutCSRF.Body.String())
	}

	updated := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, grant.CSRFToken, `{"expected_revision":0,"changes":{"timeout_seconds":300}}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("owner limits PATCH status = %d, want 200; body=%s", updated.Code, updated.Body.String())
	}
	var after ownerLimitsTestResponse
	if err := json.Unmarshal(updated.Body.Bytes(), &after); err != nil {
		t.Fatalf("decode owner limits PATCH: %v", err)
	}
	if after.Revision != 1 || after.Limits["timeout_seconds"] != 300 || len(after.Violations) != 0 {
		t.Fatalf("owner limits PATCH = %+v, want revision 1 timeout 300 and no violations", after)
	}
}

func TestOwnerLimitsRevisionBoundariesAndNullRestore(t *testing.T) {
	store := newOwnerLimitsTestStore(t, ownerLimitsOwnerID)
	manager, grant := ownerLimitsManagerSession(t, ownerLimitsOwnerID)
	server, cookies := ownerLimitsServer(t, manager, grant, store)
	updated := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, grant.CSRFToken, `{"expected_revision":0,"changes":{"timeout_seconds":300}}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("initial owner limit write = %d %s", updated.Code, updated.Body.String())
	}
	for _, revision := range []string{"0", "9007199254740991"} {
		body := `{"expected_revision":` + revision + `,"changes":{}}`
		replay := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, grant.CSRFToken, body)
		if replay.Code != http.StatusConflict || decodeError(replay) != "settings_conflict" {
			t.Fatalf("stale owner write = %d %s", replay.Code, replay.Body.String())
		}
	}
	reset := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, grant.CSRFToken, `{"expected_revision":1,"changes":{"timeout_seconds":null}}`)
	if reset.Code != http.StatusOK {
		t.Fatalf("owner limit reset = %d %s", reset.Code, reset.Body.String())
	}
	read := ownerLimitsRequest(server, http.MethodGet, "/api/owner/limits", cookies, "", "")
	assertOwnerLimitsDefaults(t, read, 2)
}

func assertOwnerLimitsDefaults(t *testing.T, response *httptest.ResponseRecorder, revision uint64) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("owner limits GET = %d %s", response.Code, response.Body.String())
	}
	var state ownerLimitsTestResponse
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode owner limits GET: %v", err)
	}
	if state.Revision != revision || len(state.Violations) != 0 {
		t.Fatalf("owner limits GET = %+v, want revision %d and no zero-group violations", state, revision)
	}
	if len(state.Limits) != len(ownerLimitsFields) {
		t.Fatalf("owner limits keys = %d, want %d: %#v", len(state.Limits), len(ownerLimitsFields), state.Limits)
	}
	for _, field := range ownerLimitsFields {
		if value, ok := state.Limits[field]; !ok || value != 0 {
			t.Fatalf("owner limits[%q] = %d (present=%v), want default 0", field, value, ok)
		}
	}
}

func TestOwnerLimitsRejectsUnclaimedInstance(t *testing.T) {
	store := newOwnerLimitsTestStore(t, 0)
	manager, grant := ownerLimitsManagerSession(t, ownerLimitsOwnerID)
	server, cookies := ownerLimitsServer(t, manager, grant, store)

	response := ownerLimitsRequest(server, http.MethodGet, "/api/owner/limits", cookies, "", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("unclaimed owner limits GET status = %d, want 403; body=%s", response.Code, response.Body.String())
	}
}

func TestOwnerLimitsRejectsNonOwnerOperator(t *testing.T) {
	store := newOwnerLimitsTestStore(t, ownerLimitsOwnerID)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken,
		Now:      func() time.Time { return ownerLimitsTestNow },
		OperatorAllowed: func(id int64) bool {
			return id == ownerLimitsStrangerID
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(ownerLimitsStrangerID)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.RedeemOperatorLink(link)
	if err != nil {
		t.Fatal(err)
	}
	server, cookies := ownerLimitsServer(t, manager, grant, store)

	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		response := ownerLimitsRequest(server, method, "/api/owner/limits", cookies, grant.CSRFToken, `{"expected_revision":0,"changes":{"timeout_seconds":300}}`)
		if response.Code != http.StatusForbidden || decodeError(response) != "owner_required" {
			t.Fatalf("non-owner operator %s = %d %s", method, response.Code, response.Body.String())
		}
	}
}

func TestOwnerLimitsRejectsInvalidRequestsWithoutPublishing(t *testing.T) {
	requests := []struct {
		name string
		body string
		code string
	}{
		{"missing revision", `{"changes":{}}`, "invalid_request"},
		{"null changes", `{"expected_revision":0,"changes":null}`, "invalid_request"},
		{"unknown field", `{"expected_revision":0,"changes":{"unknown":null}}`, "invalid_request"},
		{"negative cap", `{"expected_revision":0,"changes":{"timeout_seconds":-1}}`, "invalid_request"},
		{"fractional cap", `{"expected_revision":0,"changes":{"timeout_seconds":300.5}}`, "invalid_request"},
		{"exponent cap", `{"expected_revision":0,"changes":{"timeout_seconds":3e2}}`, "invalid_request"},
		{"unsafe cap", `{"expected_revision":0,"changes":{"warn_limit":9007199254740992}}`, "invalid_request"},
		{"unsafe revision", `{"expected_revision":9007199254740992,"changes":{}}`, "invalid_request"},
		{"unknown envelope key", `{"expected_revision":0,"changes":{},"owner_id":9000000102}`, "invalid_json"},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			store := newOwnerLimitsTestStore(t, ownerLimitsOwnerID)
			manager, grant := ownerLimitsManagerSession(t, ownerLimitsOwnerID)
			server, cookies := ownerLimitsServer(t, manager, grant, store)
			response := ownerLimitsRequest(server, http.MethodPatch, "/api/owner/limits", cookies, grant.CSRFToken, request.body)
			if response.Code != http.StatusBadRequest || decodeError(response) != request.code {
				t.Fatalf("invalid owner write = %d %s", response.Code, response.Body.String())
			}
			if revision := store.OwnerLimits().Revision; revision != 0 {
				t.Fatalf("invalid request published limits revision %d", revision)
			}
		})
	}
}

var ownerLimitsTestNow = time.Unix(1_800_000_000, 0)

func newOwnerLimitsTestStore(t *testing.T, ownerID int64) *settings.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	baseline, err := settings.LoadBaseline("", &settings.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(path, baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ownerID == 0 {
		return store
	}
	claim, _, err := store.EnsureOwnerClaim(ownerLimitsTestNow, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOwner(ownerID, claim, ownerLimitsTestNow); err != nil {
		t.Fatal(err)
	}
	return store
}

func ownerLimitsManagerSession(t *testing.T, userID int64) (*auth.Manager, auth.Grant) {
	t.Helper()
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken,
		Now:      func() time.Time { return ownerLimitsTestNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.IssueManagerSession(apiSignedInitData(ownerLimitsTestNow, userID))
	if err != nil {
		t.Fatal(err)
	}
	return manager, grant
}

func ownerLimitsServer(t *testing.T, manager *auth.Manager, grant auth.Grant, store *settings.Store) (*Server, []*http.Cookie) {
	t.Helper()
	cookies := httptest.NewRecorder()
	manager.SetCookies(cookies, grant)
	return New(Config{
		Authenticator: manager,
		Settings:      store,
	}), cookies.Result().Cookies()
}

func ownerLimitsRequest(server *Server, method, path string, cookies []*http.Cookie, csrf, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
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
