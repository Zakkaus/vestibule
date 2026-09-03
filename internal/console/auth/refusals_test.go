package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const refusalTestChatID = int64(-1009000000999)

func TestNewRefusesAnEmptyBotToken(t *testing.T) {
	if _, err := New(Config{BotToken: "123:token"}); err != nil {
		t.Fatalf("a configured bot token was refused: %v", err)
	}
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidInitData) {
		t.Fatalf("an empty bot token returned %v, want %v; accepting it leaves Mini App signatures unverifiable", err, ErrInvalidInitData)
	}
}

func TestManagerRefusesMiniAppDataOutsideItsLifetime(t *testing.T) {
	const ttl = time.Minute
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name     string
		authDate time.Time
		want     error
	}{
		{"issued inside its lifetime", now.Add(-ttl + time.Second), nil},
		{"issued at its expiry boundary", now.Add(-ttl), ErrInitDataExpired},
		{"issued in the future", now.Add(time.Second), ErrInitDataExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(Config{
				BotToken: "123:token", InitDataTTL: ttl, Now: func() time.Time { return now },
			})
			if err != nil {
				t.Fatal(err)
			}
			grant, err := manager.IssueManagerSession(signedInitData(t, "123:token", tc.authDate, 42))
			if tc.want == nil {
				if err != nil || grant.Session.Principal.TelegramID != 42 {
					t.Fatalf("recent initData grant=%+v error=%v, want a session for its signed subject", grant, err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("initData issued at %s returned %v, want %v; a copied or future-dated credential must not create a browser session", tc.authDate, err, tc.want)
			}
		})
	}
}

func TestManagerRefusesMalformedSignedMiniAppIdentities(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name string
		raw  func(*testing.T) string
	}{
		{
			name: "a zero auth date",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Set("auth_date", "0")
				return signedInitDataValues(t, "123:token", values)
			},
		},
		{
			name: "an unparseable user",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Set("user", "{")
				return signedInitDataValues(t, "123:token", values)
			},
		},
		{
			name: "a zero user identifier",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Set("user", `{"id":0}`)
				return signedInitDataValues(t, "123:token", values)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(Config{BotToken: "123:token", Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.IssueManagerSession(signedInitData(t, "123:token", now, 43)); err != nil {
				t.Fatalf("valid signed identity was refused: %v", err)
			}
			if _, err = manager.IssueManagerSession(tc.raw(t)); !errors.Is(err, ErrInvalidInitData) {
				t.Fatalf("malformed signed identity returned %v, want %v; malformed identity data must not become a browser principal", err, ErrInvalidInitData)
			}
		})
	}
}

func TestManagerRefusesOversizedOrAmbiguousSignedMiniAppData(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, tc := range []struct {
		name string
		raw  func(*testing.T) string
	}{
		{
			name: "an oversized signed payload",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Set("query_id", strings.Repeat("a", 8193))
				return signedInitDataValues(t, "123:token", values)
			},
		},
		{
			name: "a malformed query beside a valid signature",
			raw: func(t *testing.T) string {
				return signedInitData(t, "123:token", now, 42) + "&%zz=ambiguous"
			},
		},
		{
			name: "two hash fields",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Add("hash", "another-hash")
				return values.Encode()
			},
		},
		{
			name: "a repeated signed field",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Add("query_id", "another-query")
				return values.Encode()
			},
		},
		{
			name: "an unnamed signed field",
			raw: func(t *testing.T) string {
				values := parseSignedInitData(t, signedInitData(t, "123:token", now, 42))
				values.Set("", "ambiguous")
				return signedInitDataValues(t, "123:token", values)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(Config{BotToken: "123:token", Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.IssueManagerSession(signedInitData(t, "123:token", now, 43)); err != nil {
				t.Fatalf("valid signed initData was refused: %v", err)
			}
			if _, err = manager.IssueManagerSession(tc.raw(t)); !errors.Is(err, ErrInvalidInitData) {
				t.Fatalf("invalid signed initData returned %v, want %v; malformed or oversized input must not create a browser session", err, ErrInvalidInitData)
			}
		})
	}
}

func TestManagerLimitsIssuedSessionsToCredentialCapacity(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken: "123:token", MaxEntries: 1, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.IssueManagerSession(signedInitData(t, "123:token", now, 42)); err != nil {
		t.Fatalf("first credential was refused: %v", err)
	}
	if _, err = manager.IssueManagerSession(signedInitData(t, "123:token", now, 43)); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("second credential returned %v, want %v; the in-memory credential cache must stay bounded", err, ErrSessionCapacity)
	}
}

func TestManagerLimitsOutstandingOperatorLinks(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:        "123:token",
		MaxEntries:      1,
		Now:             func() time.Time { return now },
		OperatorAllowed: func(int64) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = manager.IssueOperatorLink(7); err != nil {
		t.Fatalf("first operator link was refused: %v", err)
	}
	if _, _, err = manager.IssueOperatorLink(7); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("second operator link returned %v, want %v; outstanding links must stay bounded", err, ErrSessionCapacity)
	}
}

func TestASecondManagerSessionRevokesTheFirstCookie(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{BotToken: "123:token", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.IssueManagerSession(signedInitData(t, "123:token", now, 42))
	if err != nil {
		t.Fatalf("first session: %v", err)
	}
	now = now.Add(time.Second)
	second, err := manager.IssueManagerSession(signedInitData(t, "123:token", now, 42))
	if err != nil {
		t.Fatalf("second session: %v", err)
	}

	firstRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	firstRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: first.Session.token})
	if _, err = manager.GrantFromRequest(firstRequest); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("first cookie returned %v, want %v; an old browser must lose access when the person signs in again", err, ErrSessionExpired)
	}
	secondRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	secondRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: second.Session.token})
	grant, err := manager.GrantFromRequest(secondRequest)
	if err != nil || grant.Session.Principal.TelegramID != 42 {
		t.Fatalf("replacement cookie grant=%+v error=%v, want the new session to remain usable", grant, err)
	}
}

func TestAnUnissuedCookieCannotCreateAConsoleSession(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{BotToken: "123:token", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := manager.IssueManagerSession(signedInitData(t, "123:token", now, 42))
	if err != nil {
		t.Fatal(err)
	}
	issuedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	issuedRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: issued.Session.token})
	if _, err = manager.GrantFromRequest(issuedRequest); err != nil {
		t.Fatalf("issued cookie was refused: %v", err)
	}
	unissuedRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	unissuedRequest.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "unissued-session-token"})
	if _, err = manager.GrantFromRequest(unissuedRequest); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("unissued cookie returned %v, want %v; a caller must not manufacture a console session", err, ErrSessionExpired)
	}
}

type refusalAdminChecker struct {
	allowed bool
	err     error
}

func (c refusalAdminChecker) CachedAdmin(context.Context, int64, int64) (bool, error) {
	return c.allowed, c.err
}

func (c refusalAdminChecker) FreshAdmin(context.Context, int64, int64) (bool, error) {
	return c.allowed, c.err
}

func TestAuthorizeChatRefusesRequestsItCannotVerify(t *testing.T) {
	validSession := Session{Principal: Principal{TelegramID: 42, Role: RoleManager}}
	unavailable := errors.New("getChatMember unavailable")
	for _, tc := range []struct {
		name    string
		checker AdminChecker
		session Session
		chatID  int64
		intent  AccessIntent
		want    error
	}{
		{"a verified administrator", refusalAdminChecker{allowed: true}, validSession, refusalTestChatID, ReadAccess, nil},
		{"no access checker", nil, validSession, refusalTestChatID, ReadAccess, ErrAccessUnavailable},
		{"no authenticated subject", refusalAdminChecker{allowed: true}, Session{Principal: Principal{Role: RoleManager}}, refusalTestChatID, ReadAccess, ErrAccessUnavailable},
		{"no group", refusalAdminChecker{allowed: true}, validSession, 0, ReadAccess, ErrAccessUnavailable},
		{"an unsupported access intent", refusalAdminChecker{allowed: true}, validSession, refusalTestChatID, AccessIntent(255), ErrAccessUnavailable},
		{"a Telegram membership failure", refusalAdminChecker{err: unavailable}, validSession, refusalTestChatID, ReadAccess, ErrAccessUnavailable},
		{"a non-administrator", refusalAdminChecker{}, validSession, refusalTestChatID, ReadAccess, ErrAccessDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(Config{BotToken: "123:token", AdminChecker: tc.checker})
			if err != nil {
				t.Fatal(err)
			}
			err = manager.AuthorizeChat(context.Background(), tc.session, tc.chatID, tc.intent)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("verified request was refused: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("unverifiable request returned %v, want %v; a request without a current group authorization must not gain access", err, tc.want)
			}
		})
	}
}
