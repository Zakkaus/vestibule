package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestAuditListUsesWrapperAndUndoUsesBareResponse(t *testing.T) {
	settledAt := time.Date(2026, time.September, 1, 1, 2, 3, 0, time.UTC)
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{
		groups: []int64{-100},
		auditEntries: []verification.ConsoleAuditEntry{{
			ID: "-100:42:banned", GroupID: -100, UserID: 42, Name: "Applicant",
			State: verification.ChallengeBanned, SettledAt: settledAt, SettledBy: 9,
			UndoState: verification.ConsoleUndoAvailable,
		}},
		auditEntry: verification.ConsoleAuditEntry{
			ID: "-100:42:banned", GroupID: -100, UserID: 42, Name: "Applicant",
			State: verification.ChallengeBanned, SettledAt: settledAt, SettledBy: 9,
			UndoState: verification.ConsoleUndoCompleted,
		},
	}
	server, cookies, csrf := apiTestServer(t, checker, service, nil)

	list := getAuthenticatedPath(server, cookies, "/api/chats/-100/audit")
	var listed struct {
		Items []auditResponse `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	settledBy := "9"
	wantListed := []auditResponse{{
		ID: "-100:42:banned", Kind: auditKindChallenge, User: "Applicant", GroupKey: "-100",
		Result: resultResponse{State: verification.ChallengeBanned}, SettledAt: settledAt,
		SettledBy: &settledBy, UndoState: verification.ConsoleUndoAvailable,
	}}
	if list.Code != http.StatusOK {
		t.Fatalf("audit list status=%d, want 200", list.Code)
	}
	if !reflect.DeepEqual(listed.Items, wantListed) {
		t.Fatalf("audit list payload=%#v, want %#v", listed.Items, wantListed)
	}

	undone := postAuditUndo(server, cookies, csrf, -100, "-100:42:banned")
	var entry auditResponse
	if err := json.Unmarshal(undone.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	wantEntry := auditResponse{
		ID: "-100:42:banned", Kind: auditKindChallenge, User: "Applicant", GroupKey: "-100",
		Result: resultResponse{State: verification.ChallengeBanned}, SettledAt: settledAt,
		SettledBy: &settledBy, UndoState: verification.ConsoleUndoCompleted,
	}
	if undone.Code != http.StatusOK {
		t.Fatalf("audit undo status=%d, want 200", undone.Code)
	}
	if !reflect.DeepEqual(entry, wantEntry) {
		t.Fatalf("audit undo payload=%#v, want %#v", entry, wantEntry)
	}
	if strings.Contains(undone.Body.String(), `"items"`) {
		t.Fatalf("audit undo response was wrapped: %s", undone.Body.String())
	}
	if service.undoCalls != 1 {
		t.Fatalf("audit undo calls=%d, want 1", service.undoCalls)
	}
	wantUndo := verification.ConsoleAuditUndo{ID: "-100:42:banned", GroupID: -100, ActorID: 9}
	if service.lastUndo != wantUndo {
		t.Fatalf("audit undo request=%#v, want %#v", service.lastUndo, wantUndo)
	}
}

func TestAuditUndoRequiresCSRFFreshWriteAccessAndMapsConflicts(t *testing.T) {
	csrfChecker := &apiTestAdminChecker{allowed: true}
	csrfService := &apiTestQueueService{groups: []int64{-100}}
	csrfServer, csrfCookies, _ := apiTestServer(t, csrfChecker, csrfService, nil)
	withoutCSRF := postAuditUndo(csrfServer, csrfCookies, "", -100, "-100:42:banned")
	if withoutCSRF.Code != http.StatusForbidden || decodeError(withoutCSRF) != "csrf_invalid" ||
		csrfService.undoCalls != 0 {
		t.Fatalf("no_csrf=%d code=%s calls=%d",
			withoutCSRF.Code, decodeError(withoutCSRF), csrfService.undoCalls)
	}

	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{groups: []int64{-100}}
	server, cookies, csrf := apiTestServer(t, checker, service, nil)
	read := getAuthenticatedPath(server, cookies, "/api/chats/-100/audit")
	checker.setAllowed(false)
	denied := postAuditUndo(server, cookies, csrf, -100, "-100:42:banned")
	counts := checker.counts()
	if read.Code != http.StatusOK || denied.Code != http.StatusForbidden ||
		decodeError(denied) != "chat_access_denied" || service.undoCalls != 0 ||
		counts.cachedCalls != 1 || counts.freshCalls != 1 {
		t.Fatalf("read=%d denied=%d code=%s calls=%d cached=%d fresh=%d",
			read.Code, denied.Code, decodeError(denied), service.undoCalls,
			counts.cachedCalls, counts.freshCalls)
	}

	checker.setAllowed(true)
	service.undoErr = verification.ErrConsoleAuditNotUndoable
	conflict := postAuditUndo(server, cookies, csrf, -100, "-100:42:banned")
	if conflict.Code != http.StatusConflict || decodeError(conflict) != "audit_not_undoable" ||
		service.undoCalls != 1 {
		t.Fatalf("conflict=%d code=%s calls=%d", conflict.Code, decodeError(conflict), service.undoCalls)
	}
}

func postAuditUndo(
	server *Server,
	cookies []*http.Cookie,
	csrf string,
	chatID int64,
	auditID string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost,
		"/api/chats/"+strconv.FormatInt(chatID, 10)+"/audit/"+auditID+"/undo", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
