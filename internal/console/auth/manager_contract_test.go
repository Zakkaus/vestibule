package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const authContractBotToken = "123:auth-contract-token"

type authContractCookieState struct {
	name        string
	value       string
	path        string
	expiresNano int64
	maxAge      int
	secure      bool
	httpOnly    bool
	sameSite    http.SameSite
}

func TestMiniAppIdentityRequiresSingularBoundedFields(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	const ttl = 10 * time.Minute
	valid := authContractInitData(now, 42, "valid")
	identity, err := VerifyInitData(valid, authContractBotToken, now, ttl)
	if err != nil || identity.ID != 42 {
		t.Fatalf("valid Mini App identity was refused: identity=%+v error=%v", identity, err)
	}

	tests := []struct {
		name string
		raw  func() string
	}{
		{
			name: "payload larger than the admission bound",
			raw: func() string {
				values := authContractValues(now, 42, "oversized")
				values.Set("padding", strings.Repeat("x", 8192))
				return signAuthContractValues(values)
			},
		},
		{
			name: "two hashes",
			raw: func() string {
				values, parseErr := url.ParseQuery(valid)
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				hash := values.Get("hash")
				values["hash"] = []string{hash, hash}
				return values.Encode()
			},
		},
		{
			name: "repeated signed field",
			raw: func() string {
				values := authContractValues(now, 42, "first")
				values["query_id"] = []string{"first", "second"}
				return signAuthContractValues(values)
			},
		},
		{
			name: "empty field name",
			raw: func() string {
				values := authContractValues(now, 42, "empty-key")
				values[""] = []string{"hidden"}
				return signAuthContractValues(values)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertAuthContractRejected(t, test.raw(), now, ttl, ErrInvalidInitData)
		})
	}
}

func TestMiniAppIdentityRequiresANonzeroDecodableUser(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	const ttl = 10 * time.Minute
	tests := []struct {
		name string
		user string
	}{
		{name: "zero user identifier", user: `{"id":0}`},
		{name: "malformed user", user: "not-json"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := authContractValues(now, 42, test.name)
			values.Set("user", test.user)
			assertAuthContractRejected(t, signAuthContractValues(values), now, ttl, ErrInvalidInitData)
		})
	}
}

func TestMiniAppIdentityEnforcesFreshnessBoundaries(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	const ttl = 10 * time.Minute
	tests := []struct {
		name     string
		authDate time.Time
	}{
		{name: "future authentication date", authDate: now.Add(time.Second)},
		{name: "authentication date at expiry boundary", authDate: now.Add(-ttl)},
		{name: "authentication date beyond expiry boundary", authDate: now.Add(-ttl - time.Second)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := authContractInitData(test.authDate, 42, test.name)
			assertAuthContractRejected(t, raw, now, ttl, ErrInitDataExpired)
		})
	}
	if _, err := VerifyInitData(authContractInitData(now.Add(-ttl+time.Second), 42, "inside"), authContractBotToken, now, ttl); err != nil {
		t.Fatalf("fresh Mini App identity inside the boundary was refused: %v", err)
	}
}

func assertAuthContractRejected(t *testing.T, raw string, now time.Time, ttl time.Duration, want error) {
	t.Helper()
	if got, err := VerifyInitData(raw, authContractBotToken, now, ttl); !errors.Is(err, want) {
		t.Fatalf("untrusted Mini App payload opened a manager identity: identity=%+v error=%v, want %v", got, err, want)
	}
}

func TestManagerKeepsOneLiveSessionPerUserAtCapacity(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:    authContractBotToken,
		Now:         func() time.Time { return now },
		InitDataTTL: time.Minute,
		SessionTTL:  10 * time.Minute,
		MaxEntries:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := issueAuthContractSession(t, manager, now, 41, "first")
	second := issueAuthContractSession(t, manager, now, 42, "second")
	now = now.Add(2 * time.Minute)
	replacement := issueAuthContractSession(t, manager, now, 41, "replacement")

	if _, err = manager.GrantFromRequest(authContractRequest(first)); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("a replaced browser session remained usable: %v", err)
	}
	for name, grant := range map[string]Grant{"replacement": replacement, "other user": second} {
		if _, err = manager.GrantFromRequest(authContractRequest(grant)); err != nil {
			t.Fatalf("%s session was lost while replacing one user: %v", name, err)
		}
	}
	if _, err = manager.IssueManagerSession(authContractInitData(now, 43, "over-capacity")); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("a third live user exceeded the two-session bound: %v", err)
	}
}

func TestManagerBoundsReplayRecordsEvenWhenAUserReplacesTheirSession(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:   authContractBotToken,
		Now:        func() time.Time { return now },
		MaxEntries: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	issueAuthContractSession(t, manager, now, 41, "first")
	if _, err = manager.IssueManagerSession(authContractInitData(now, 41, "second")); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("one user grew the bounded replay cache past its limit: %v", err)
	}
}

func TestManagerRefusesConfigurationWithoutABotToken(t *testing.T) {
	if _, err := New(Config{BotToken: authContractBotToken}); err != nil {
		t.Fatalf("manager rejected a configured bot token: %v", err)
	}
	if manager, err := New(Config{}); !errors.Is(err, ErrInvalidInitData) || manager != nil {
		t.Fatalf("manager accepted configuration that cannot authenticate Mini App data: manager=%v error=%v", manager, err)
	}
}

func TestSessionCookiesShareTheSessionLifetimeAndDeletionScope(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:   authContractBotToken,
		Now:        func() time.Time { return now },
		SessionTTL: 90 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := issueAuthContractSession(t, manager, now, 41, "cookie")
	if want := now.Add(90 * time.Minute); !grant.Session.ExpiresAt.Equal(want) {
		t.Fatalf("issued session lost the configured lifetime: expiry=%v want=%v", grant.Session.ExpiresAt, want)
	}
	writer := httptest.NewRecorder()
	manager.SetCookies(writer, grant)
	cookie := authContractCookie(t, writer)
	wantCookie := authContractCookieState{
		name: sessionCookieName, value: grant.Session.token, path: "/",
		expiresNano: grant.Session.ExpiresAt.UnixNano(),
		secure:      true, httpOnly: true, sameSite: http.SameSiteLaxMode,
	}
	if got := authContractCookieStateOf(cookie); got != wantCookie {
		t.Fatalf("session cookie did not carry the session lifetime and browser protections: got=%+v want=%+v", got, wantCookie)
	}
	if _, err = manager.GrantFromRequest(authContractRequest(grant)); err != nil {
		t.Fatalf("the matching session cookie was refused: %v", err)
	}

	clearWriter := httptest.NewRecorder()
	manager.ClearCookies(clearWriter)
	cleared := authContractCookie(t, clearWriter)
	wantCleared := authContractCookieState{
		name: sessionCookieName, path: "/", expiresNano: time.Unix(1, 0).UnixNano(), maxAge: -1,
		secure: true, httpOnly: true, sameSite: http.SameSiteLaxMode,
	}
	if got := authContractCookieStateOf(cleared); got != wantCleared {
		t.Fatalf("cookie deletion did not cover the protected session cookie: got=%+v want=%+v", got, wantCleared)
	}
}

type authContractAdminChecker struct {
	allowed map[int64]bool
	errors  map[int64]error
	cached  atomic.Int64
	fresh   atomic.Int64
}

func (c *authContractAdminChecker) CachedAdmin(_ context.Context, chatID, _ int64) (bool, error) {
	c.cached.Add(1)
	return c.allowed[chatID], c.errors[chatID]
}

func (c *authContractAdminChecker) FreshAdmin(_ context.Context, chatID, _ int64) (bool, error) {
	c.fresh.Add(1)
	return c.allowed[chatID], c.errors[chatID]
}

func TestAuthorizeChatRejectsInvalidSubjectsBeforeCallingTelegram(t *testing.T) {
	const chatID int64 = -1009000000901
	withoutChecker, err := New(Config{BotToken: authContractBotToken})
	if err != nil {
		t.Fatal(err)
	}
	validSession := Session{Principal: Principal{TelegramID: 41, Role: RoleManager}}
	if err = authorizeAuthContract(t, withoutChecker, validSession, chatID); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatalf("authorization without a Telegram checker failed open: %v", err)
	}

	checker := &authContractAdminChecker{allowed: map[int64]bool{chatID: true}, errors: map[int64]error{}}
	manager, err := New(Config{BotToken: authContractBotToken, AdminChecker: checker})
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		session Session
		chatID  int64
	}{
		"missing Telegram identity": {session: Session{}, chatID: chatID},
		"missing chat identity":     {session: validSession, chatID: 0},
	} {
		if authorizeErr := authorizeAuthContract(t, manager, test.session, test.chatID); !errors.Is(authorizeErr, ErrAccessUnavailable) {
			t.Fatalf("%s reached an authorization decision: %v", name, authorizeErr)
		}
	}
	cached, fresh := checker.cached.Load(), checker.fresh.Load()
	if cached != 0 || fresh != 0 {
		t.Fatalf("invalid authorization subjects reached Telegram: cached=%d fresh=%d", cached, fresh)
	}
	if err = manager.AuthorizeChat(context.Background(), validSession, chatID, ReadAccess); err != nil {
		t.Fatalf("valid cached authorization was refused: %v", err)
	}
}

func TestAccessibleChatsReturnsOnlyAllowedCandidatesInOriginalOrder(t *testing.T) {
	first := int64(-1009000000911)
	denied := int64(-1009000000912)
	unavailable := int64(-1009000000913)
	last := int64(-1009000000914)
	checker := &authContractAdminChecker{
		allowed: map[int64]bool{first: true, denied: false, last: true},
		errors:  map[int64]error{unavailable: errors.New("Telegram unavailable")},
	}
	manager, err := New(Config{BotToken: authContractBotToken, AdminChecker: checker})
	if err != nil {
		t.Fatal(err)
	}
	candidates := []int64{first, denied, first, unavailable, last}
	got := manager.AccessibleChats(context.Background(), Session{Principal: Principal{TelegramID: 41}}, candidates)
	want := []int64{first, first, last}
	if !slices.Equal(got, want) {
		t.Fatalf("accessible chats lost filtering, identifiers, or order: got=%v want=%v", got, want)
	}
	cached, fresh := checker.cached.Load(), checker.fresh.Load()
	if cached != int64(len(candidates)) || fresh != 0 {
		t.Fatalf("chat listing used the wrong authorization path: cached=%d fresh=%d", cached, fresh)
	}
}

func authorizeAuthContract(t *testing.T, manager *Manager, session Session, chatID int64) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("invalid authorization subject reached Telegram and panicked: %v", recovered)
		}
	}()
	return manager.AuthorizeChat(context.Background(), session, chatID, ReadAccess)
}

func authContractValues(at time.Time, userID int64, queryID string) url.Values {
	return url.Values{
		"auth_date": {strconv.FormatInt(at.Unix(), 10)},
		"query_id":  {queryID},
		"user":      {`{"id":` + strconv.FormatInt(userID, 10) + `}`},
	}
}

func authContractInitData(at time.Time, userID int64, queryID string) string {
	return signAuthContractValues(authContractValues(at, userID, queryID))
}

func signAuthContractValues(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(authContractBotToken))
	check := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = check.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(check.Sum(nil)))
	return values.Encode()
}

func issueAuthContractSession(t *testing.T, manager *Manager, at time.Time, userID int64, queryID string) Grant {
	t.Helper()
	grant, err := manager.IssueManagerSession(authContractInitData(at, userID, queryID))
	if err != nil {
		t.Fatalf("issue session for user %d: %v", userID, err)
	}
	return grant
}

func authContractRequest(grant Grant) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: grant.Session.token})
	return request
}

func authContractCookie(t *testing.T, writer *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	cookies := writer.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Set-Cookie count=%d, want 1", len(cookies))
	}
	return cookies[0]
}

func authContractCookieStateOf(cookie *http.Cookie) authContractCookieState {
	return authContractCookieState{
		name:        cookie.Name,
		value:       cookie.Value,
		path:        cookie.Path,
		expiresNano: cookie.Expires.UnixNano(),
		maxAge:      cookie.MaxAge,
		secure:      cookie.Secure,
		httpOnly:    cookie.HttpOnly,
		sameSite:    cookie.SameSite,
	}
}
