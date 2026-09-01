package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const apiTestToken = "123:api-test-token"

type apiTestAdminChecker struct {
	mu              sync.Mutex
	allowed         bool
	err             error
	cache           map[[2]int64]struct{}
	cachedCalls     int
	freshCalls      int
	telegramQueries int
}

type apiTestAdminCounts struct {
	cachedCalls     int
	freshCalls      int
	telegramQueries int
}

func (c *apiTestAdminChecker) CachedAdmin(_ context.Context, chatID, userID int64) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cachedCalls++
	key := [2]int64{chatID, userID}
	if _, ok := c.cache[key]; ok {
		return true, nil
	}
	return c.queryLocked(key)
}

func (c *apiTestAdminChecker) FreshAdmin(_ context.Context, chatID, userID int64) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.freshCalls++
	return c.queryLocked([2]int64{chatID, userID})
}

func (c *apiTestAdminChecker) queryLocked(key [2]int64) (bool, error) {
	c.telegramQueries++
	if c.err != nil {
		return false, c.err
	}
	if c.allowed {
		if c.cache == nil {
			c.cache = make(map[[2]int64]struct{})
		}
		c.cache[key] = struct{}{}
	} else {
		delete(c.cache, key)
	}
	return c.allowed, nil
}

func (c *apiTestAdminChecker) setAllowed(allowed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allowed = allowed
}

func (c *apiTestAdminChecker) counts() apiTestAdminCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	return apiTestAdminCounts{
		cachedCalls: c.cachedCalls, freshCalls: c.freshCalls, telegramQueries: c.telegramQueries,
	}
}

type apiTestQueueService struct {
	groups          []int64
	entries         []verification.ConsoleQueueEntry
	settledEntry    verification.ConsoleQueueEntry
	settleErr       error
	settlementCalls int
	telegramActions int
	auditEntries    []verification.ConsoleAuditEntry
	auditEntry      verification.ConsoleAuditEntry
	auditErr        error
	undoErr         error
	auditCalls      int
	undoCalls       int
	lastUndo        verification.ConsoleAuditUndo
}

func (s *apiTestQueueService) ConsoleGroups() []int64 {
	return append([]int64(nil), s.groups...)
}

func (s *apiTestQueueService) ConsoleQueue(context.Context, int64) ([]verification.ConsoleQueueEntry, error) {
	return append([]verification.ConsoleQueueEntry(nil), s.entries...), nil
}

func (s *apiTestQueueService) SettleConsole(context.Context, verification.ConsoleSettlement) (verification.ConsoleQueueEntry, error) {
	s.settlementCalls++
	return s.settledEntry, s.settleErr
}

func (s *apiTestQueueService) ConsoleAudit(
	context.Context,
	int64,
	int64,
) ([]verification.ConsoleAuditEntry, error) {
	s.auditCalls++
	return append([]verification.ConsoleAuditEntry(nil), s.auditEntries...), s.auditErr
}

func (s *apiTestQueueService) UndoConsoleAudit(
	_ context.Context,
	undo verification.ConsoleAuditUndo,
) (verification.ConsoleAuditEntry, error) {
	s.undoCalls++
	s.lastUndo = undo
	return s.auditEntry, s.undoErr
}

func TestPostSessionRejectsReplayedInitData(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{BotToken: apiTestToken, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Authenticator: manager})
	initData := apiSignedInitData(now, 9)
	if response := postInitData(server, initData); response.Code != http.StatusCreated {
		t.Fatalf("first session status = %d, want 201", response.Code)
	}
	response := postInitData(server, initData)
	if response.Code != http.StatusConflict || decodeError(response) != "init_data_replayed" {
		t.Fatalf("replay status=%d code=%s, want 409 and init_data_replayed", response.Code, decodeError(response))
	}
	t.Logf("same initData replay -> %d", response.Code)
}

func TestEnterRedeemsOperatorLinkOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken, Now: func() time.Time { return now }, OperatorAllowed: func(id int64) bool { return id == 9 },
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.IssueOperatorLink(9)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Authenticator: manager})
	first := enterLink(server, token)
	cookies := first.Result().Cookies()
	if first.Code != http.StatusSeeOther || first.Header().Get("Location") != "/" || len(cookies) != 1 {
		t.Fatalf("first enter response = %d %q cookies=%d", first.Code, first.Header().Get("Location"), len(cookies))
	}
	if !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie lacks required protections: %#v", cookies[0])
	}
	second := enterLink(server, token)
	if second.Code != http.StatusSeeOther || second.Header().Get("Location") != "/?state=redeemed" {
		t.Fatalf("second enter response = %d %q", second.Code, second.Header().Get("Location"))
	}
}

func TestGetSessionRejectsMissingCookieWithoutIssuingSession(t *testing.T) {
	manager, err := auth.New(auth.Config{BotToken: apiTestToken})
	if err != nil {
		t.Fatal(err)
	}
	response := getPath(New(Config{Authenticator: manager}), "/api/session")
	cookies := response.Result().Cookies()
	errorCode := decodeError(response)
	if response.Code != http.StatusUnauthorized || errorCode != "authentication_expired" ||
		len(cookies) != 1 || cookies[0].Value != "" || cookies[0].MaxAge != -1 {
		t.Fatalf("status=%d code=%s cookies=%#v, want 401, authentication_expired, and only a clearing cookie",
			response.Code, errorCode, cookies)
	}
	t.Logf("GET /api/session no_cookie -> status=%d body=%s set_cookie=%q",
		response.Code, strings.TrimSpace(response.Body.String()), response.Header().Get("Set-Cookie"))
}

func TestGetSessionRejectsExpiredCookie(t *testing.T) {
	clock := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken, Now: func() time.Time { return clock }, SessionTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.IssueManagerSession(apiSignedInitData(clock, 9))
	if err != nil {
		t.Fatal(err)
	}
	cookieWriter := httptest.NewRecorder()
	manager.SetCookies(cookieWriter, grant)
	clock = clock.Add(time.Minute)
	response := getAuthenticatedPath(New(Config{Authenticator: manager}), cookieWriter.Result().Cookies(), "/api/session")
	cookies := response.Result().Cookies()
	errorCode := decodeError(response)
	if response.Code != http.StatusUnauthorized || errorCode != "authentication_expired" ||
		len(cookies) != 1 || cookies[0].Value != "" || cookies[0].MaxAge != -1 {
		t.Fatalf("status=%d code=%s cookies=%#v, want 401, authentication_expired, and only a clearing cookie",
			response.Code, errorCode, cookies)
	}
	t.Logf("GET /api/session expired_cookie -> status=%d body=%s set_cookie=%q",
		response.Code, strings.TrimSpace(response.Body.String()), response.Header().Get("Set-Cookie"))
}

func TestOperatorCanSettleAfterReadingCurrentSession(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{
		groups: []int64{-100},
		settledEntry: verification.ConsoleQueueEntry{
			ID: "-100:42:nonce", GroupID: -100, UserID: 42, Name: "Applicant",
			State: verification.ChallengeApproved, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
		},
	}
	manager, err := auth.New(auth.Config{
		BotToken: apiTestToken, Now: func() time.Time { return now }, AdminChecker: checker,
		OperatorAllowed: func(id int64) bool { return id == 9 },
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.IssueOperatorLink(9)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Authenticator: manager, Verification: queue})
	entered := enterLink(server, token)
	cookies := entered.Result().Cookies()
	if entered.Code != http.StatusSeeOther || entered.Header().Get("Location") != "/" || len(cookies) != 1 {
		t.Fatalf("enter status=%d location=%q cookies=%d, want 303, /, and one cookie",
			entered.Code, entered.Header().Get("Location"), len(cookies))
	}
	current := getAuthenticatedPath(server, cookies, "/api/session")
	var session sessionResponse
	if err := json.Unmarshal(current.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	if current.Code != http.StatusOK || session.Subject.TelegramID != "9" ||
		session.Subject.Role != auth.RoleOperator || session.CSRFToken == "" {
		t.Fatalf("GET session status=%d payload=%#v", current.Code, session)
	}
	requireNoSetCookie(t, current)
	withCSRF := postSettlement(server, cookies, session.CSRFToken, -100, "-100:42:nonce")
	withoutCSRF := postSettlement(server, cookies, "", -100, "-100:42:nonce")
	withoutCSRFCode := decodeError(withoutCSRF)
	if withCSRF.Code != http.StatusOK || withoutCSRF.Code != http.StatusForbidden ||
		withoutCSRFCode != "csrf_invalid" || queue.settlementCalls != 1 {
		t.Fatalf("with_csrf=%d without_csrf=%d code=%s settlement_calls=%d",
			withCSRF.Code, withoutCSRF.Code, withoutCSRFCode, queue.settlementCalls)
	}
	t.Logf("GET /enter/{token} -> status=%d location=%q cookies=%d",
		entered.Code, entered.Header().Get("Location"), len(cookies))
	t.Logf("GET /api/session -> status=%d body=%s", current.Code, strings.TrimSpace(current.Body.String()))
	t.Logf("POST settlement with X-CSRF-Token -> status=%d body=%s",
		withCSRF.Code, strings.TrimSpace(withCSRF.Body.String()))
	t.Logf("POST settlement without X-CSRF-Token -> status=%d body=%s",
		withoutCSRF.Code, strings.TrimSpace(withoutCSRF.Body.String()))
}

func TestPostSettlementRejectsMembershipLookupFailure(t *testing.T) {
	checker := &apiTestAdminChecker{err: errors.New("getChatMember unavailable")}
	queue := &apiTestQueueService{groups: []int64{-100}}
	server, cookies, csrf := apiTestServer(t, checker, queue, nil)
	response := postSettlement(server, cookies, csrf, -100, "-100:42:nonce")
	if response.Code != http.StatusServiceUnavailable || decodeError(response) != "chat_access_unavailable" ||
		queue.settlementCalls != 0 {
		t.Fatalf("status=%d code=%s settlement_calls=%d, want 503, chat_access_unavailable, and 0",
			response.Code, decodeError(response), queue.settlementCalls)
	}
	t.Logf("getChatMember query failure -> %d; settlement calls=%d", response.Code, queue.settlementCalls)
}

func TestPostSettlementUsesFreshAdminAfterCachedRead(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{groups: []int64{-100}}
	server, cookies, csrf := apiTestServer(t, checker, queue, nil)
	read := getAuthenticatedPath(server, cookies, "/api/chats/-100/queue")
	if read.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200", read.Code)
	}
	checker.setAllowed(false)
	write := postSettlement(server, cookies, csrf, -100, "-100:42:nonce")
	counts := checker.counts()
	if write.Code != http.StatusForbidden || decodeError(write) != "chat_access_denied" ||
		counts.cachedCalls != 1 || counts.freshCalls != 1 || counts.telegramQueries != 2 ||
		queue.settlementCalls != 0 {
		t.Fatalf("write status=%d code=%s cached_reads=%d fresh_writes=%d Telegram_queries=%d settlement_calls=%d",
			write.Code, decodeError(write), counts.cachedCalls, counts.freshCalls, counts.telegramQueries,
			queue.settlementCalls)
	}
	t.Logf("write_status=%d code=%s cached_reads=%d fresh_writes=%d Telegram_queries=%d settlement_calls=%d",
		write.Code, decodeError(write), counts.cachedCalls, counts.freshCalls, counts.telegramQueries,
		queue.settlementCalls)
}

func TestChatsReusesPositiveAdminCache(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{groups: []int64{-100}}
	server, cookies, _ := apiTestServer(t, checker, queue, nil)
	first := getAuthenticatedPath(server, cookies, "/api/chats")
	firstCounts := checker.counts()
	second := getAuthenticatedPath(server, cookies, "/api/chats")
	secondCounts := checker.counts()
	if first.Code != http.StatusOK || second.Code != http.StatusOK || firstCounts.telegramQueries != 1 ||
		secondCounts.telegramQueries != firstCounts.telegramQueries || secondCounts.cachedCalls != 2 ||
		secondCounts.freshCalls != 0 {
		t.Fatalf("statuses=%d,%d first_queries=%d second_query_delta=%d cached_calls=%d fresh_calls=%d",
			first.Code, second.Code, firstCounts.telegramQueries,
			secondCounts.telegramQueries-firstCounts.telegramQueries,
			secondCounts.cachedCalls, secondCounts.freshCalls)
	}
	t.Logf("first_queries=%d second_query_delta=%d total_queries=%d",
		firstCounts.telegramQueries, secondCounts.telegramQueries-firstCounts.telegramQueries,
		secondCounts.telegramQueries)
}

func TestPostSettlementReturnsConflictWithoutTelegramAction(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{groups: []int64{-100}, settleErr: verification.ErrConsoleChallengeConflict}
	server, cookies, csrf := apiTestServer(t, checker, queue, nil)
	response := postSettlement(server, cookies, csrf, -100, "-100:42:expired")
	if response.Code != http.StatusConflict || decodeError(response) != "challenge_conflict" || queue.telegramActions != 0 {
		t.Fatalf("status=%d code=%s telegram_actions=%d, want 409, challenge_conflict, and 0",
			response.Code, decodeError(response), queue.telegramActions)
	}
	t.Logf("POST stale challenge -> %d; Telegram actions=%d", response.Code, queue.telegramActions)
}

func TestPostSettlementRequiresCSRF(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{groups: []int64{-100}}
	server, cookies, _ := apiTestServer(t, checker, queue, nil)
	response := postSettlement(server, cookies, "", -100, "-100:42:nonce")
	if response.Code != http.StatusForbidden || decodeError(response) != "csrf_invalid" || queue.settlementCalls != 0 {
		t.Fatalf("status=%d code=%s settlement_calls=%d, want 403, csrf_invalid, and 0",
			response.Code, decodeError(response), queue.settlementCalls)
	}
}

func TestQueueResponseUsesChallengeVocabulary(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	checker := &apiTestAdminChecker{allowed: true}
	queue := &apiTestQueueService{groups: []int64{-100}, entries: []verification.ConsoleQueueEntry{{
		ID: "-100:42:nonce", GroupID: -100, UserID: 42, Name: "Applicant", State: verification.ChallengePending,
		CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}}}
	server, cookies, _ := apiTestServer(t, checker, queue, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/chats/-100/queue", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	var payload struct {
		Items []queueResponse `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(payload.Items) != 1 || payload.Items[0].Result.State != verification.ChallengePending ||
		payload.Items[0].Result.Reason != nil || payload.Items[0].OccurredAt == nil || payload.Items[0].RemainingSeconds == nil {
		t.Fatalf("queue response = %#v", payload)
	}
}

func TestHealthKeepsLivenessWhenDatabaseFails(t *testing.T) {
	databaseHandle, err := database.Open(context.Background(), database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := databaseHandle.Close(); err != nil {
		t.Fatal(err)
	}
	health := status.NewHealth(databaseHandle.RawDB.PingContext)
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	server := New(Config{Health: health})
	live := getPath(server, "/livez")
	ready := getPath(server, "/readyz")
	if live.Code != http.StatusOK || ready.Code == http.StatusOK {
		t.Fatalf("livez=%d readyz=%d, want 200 and non-200", live.Code, ready.Code)
	}
	t.Logf("database unavailable -> /livez=%d /readyz=%d", live.Code, ready.Code)
}

func apiTestServer(
	t *testing.T,
	checker auth.AdminChecker,
	queue ConsoleService,
	health *status.Health,
	settingServices ...SettingsService,
) (*Server, []*http.Cookie, string) {
	t.Helper()
	if len(settingServices) > 1 {
		t.Fatal("apiTestServer accepts at most one settings service")
	}
	now := time.Unix(1_800_000_000, 0)
	manager, err := auth.New(auth.Config{BotToken: apiTestToken, Now: func() time.Time { return now }, AdminChecker: checker})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.IssueManagerSession(apiSignedInitData(now, 9))
	if err != nil {
		t.Fatal(err)
	}
	var settingsService SettingsService
	if len(settingServices) == 1 {
		settingsService = settingServices[0]
	}
	cookies := httptest.NewRecorder()
	manager.SetCookies(cookies, grant)
	config := Config{Authenticator: manager, Verification: queue, Settings: settingsService, Health: health}
	return New(config), cookies.Result().Cookies(), grant.CSRFToken
}

func getAuthenticatedPath(server *Server, cookies []*http.Cookie, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func requireNoSetCookie(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if header := response.Header().Get("Set-Cookie"); header != "" {
		t.Fatalf("unexpected Set-Cookie header: %q", header)
	}
}

func postSettlement(server *Server, cookies []*http.Cookie, csrf string, chatID int64, challengeID string) *httptest.ResponseRecorder {
	body := `{"expected":{"state":"pending","reason":null},"result":{"state":"approved","reason":null}}`
	request := httptest.NewRequest(http.MethodPost, "/api/chats/"+strconv.FormatInt(chatID, 10)+"/queue/"+challengeID, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func postInitData(server *Server, initData string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"init_data": initData})
	request := httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func enterLink(server *Server, token string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/enter/"+token, nil)
	server.Handler().ServeHTTP(response, request)
	return response
}

func getPath(server *Server, path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func apiSignedInitData(now time.Time, userID int64) string {
	values := url.Values{
		"auth_date": {strconv.FormatInt(now.Unix(), 10)},
		"user":      {`{"id":` + strconv.FormatInt(userID, 10) + `}`},
	}
	parts := []string{"auth_date=" + values.Get("auth_date"), "user=" + values.Get("user")}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(apiTestToken))
	check := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = check.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(check.Sum(nil)))
	return values.Encode()
}

func decodeError(response *httptest.ResponseRecorder) string {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &payload)
	return payload.Error.Code
}
