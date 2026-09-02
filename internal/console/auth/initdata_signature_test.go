package auth

import (
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestManagerRejectsForgedInitDataSignatures(t *testing.T) {
	const botToken = "123:token"
	now := time.Unix(1_800_000_000, 0)
	cases := []struct {
		name string
		raw  func(*testing.T) string
	}{
		{
			name: "forged signature",
			raw: func(t *testing.T) string {
				t.Helper()
				values := parseSignedInitData(t, signedInitData(t, botToken, now, 42))
				hash := values.Get("hash")
				first := byte('0')
				if hash[0] == first {
					first = '1'
				}
				values.Set("hash", string(first)+hash[1:])
				return values.Encode()
			},
		},
		{
			name: "signature from another bot token",
			raw: func(t *testing.T) string {
				t.Helper()
				return signedInitData(t, "456:wrong-token", now, 42)
			},
		},
		{
			name: "signed payload with tampered user",
			raw: func(t *testing.T) string {
				t.Helper()
				values := parseSignedInitData(t, signedInitData(t, botToken, now, 42))
				values.Set("user", `{"id":43}`)
				return values.Encode()
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := New(Config{BotToken: botToken, Now: func() time.Time { return now }})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = manager.IssueManagerSession(tc.raw(t)); !errors.Is(err, ErrInvalidInitData) {
				t.Fatalf("IssueManagerSession error = %v, want %v", err, ErrInvalidInitData)
			}
		})
	}
}

func parseSignedInitData(t *testing.T, raw string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(values.Get("hash")) != 64 {
		t.Fatalf("signed initData hash length = %d, want 64", len(values.Get("hash")))
	}
	return values
}
