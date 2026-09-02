package auth

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// The CSRF token is what stops another site driving a logged-in operator's browser through
// the console: the session cookie travels on a cross-site request, this header does not.
// Nothing tested the comparison. Rewriting it to compare the supplied token against itself,
// which accepts any non-empty value, left every test in the repository passing.
func TestValidateCSRF(t *testing.T) {
	manager, grant := operatorGrant(t)
	for _, tc := range []struct {
		name   string
		header string
		want   error
	}{
		{"the token this session was issued", grant.CSRFToken, nil},
		{"a token from somewhere else", "a-token-that-is-not-this-one", ErrCSRFInvalid},
		{"the right length, wrong bytes", flipFirst(grant.CSRFToken), ErrCSRFInvalid},
		{"no header at all", "", ErrCSRFInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, "/api/chats/1/queue", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.header != "" {
				request.Header.Set("X-CSRF-Token", tc.header)
			}
			err = manager.ValidateCSRF(request, grant.Session)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("the session's own token was refused: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("header %q = %v, want %v; another site can send the cookie but not "+
					"this header, which is the whole defence", tc.header, err, tc.want)
			}
		})
	}
}

// A session carrying no token accepts nothing, whatever the caller sends. Two mechanisms
// hold this -- the empty-session clause and the comparison, which cannot match against an
// empty token either -- so no single edit makes it fail. The property is what matters.
func TestValidateCSRFRefusesASessionWithoutAToken(t *testing.T) {
	manager, grant := operatorGrant(t)
	session := grant.Session
	session.csrf = ""
	for _, header := range []string{"", "any-token-at-all", grant.CSRFToken} {
		request, err := http.NewRequest(http.MethodPost, "/api/chats/1/queue", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-CSRF-Token", header)
		if err := manager.ValidateCSRF(request, session); !errors.Is(err, ErrCSRFInvalid) {
			t.Errorf("a session with no token accepted header %q: %v", header, err)
		}
	}
}

func operatorGrant(t *testing.T) (*Manager, Grant) {
	t.Helper()
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:        "123:token",
		Now:             func() time.Time { return now },
		OperatorAllowed: func(id int64) bool { return id == 7 },
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(7)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.RedeemOperatorLink(link)
	if err != nil {
		t.Fatal(err)
	}
	return manager, grant
}

func flipFirst(token string) string {
	if token == "" {
		return "x"
	}
	first := byte('a')
	if token[0] == 'a' {
		first = 'b'
	}
	return string(first) + token[1:]
}
