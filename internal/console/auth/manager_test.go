package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestManagerRejectsReplayedInitData(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{BotToken: "123:token", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	initData := signedInitData(t, "123:token", now, 42)
	if _, err := manager.IssueManagerSession(initData); err != nil {
		t.Fatalf("first initData exchange: %v", err)
	}
	if _, err := manager.IssueManagerSession(initData); !errors.Is(err, ErrInitDataReplayed) {
		t.Fatalf("replay error = %v, want %v", err, ErrInitDataReplayed)
	} else {
		t.Logf("same initData replay -> %v", err)
	}
}

func TestManagerRedeemsOperatorLinkOnlyOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken: "123:token", Now: func() time.Time { return now }, OperatorAllowed: func(id int64) bool { return id == 7 },
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RedeemOperatorLink(link); err != nil {
		t.Fatalf("first redemption: %v", err)
	}
	if _, err := manager.RedeemOperatorLink(link); !errors.Is(err, ErrOperatorLinkRedeemed) {
		t.Fatalf("second redemption error = %v, want %v", err, ErrOperatorLinkRedeemed)
	}
}

func TestManagerCapsAllCredentialLifetimesAtEightHours(t *testing.T) {
	manager, err := New(Config{
		BotToken:    "123:token",
		InitDataTTL: 24 * time.Hour, SessionTTL: 24 * time.Hour, OperatorLinkTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string]time.Duration{
		"init data": manager.initDataTTL, "session": manager.sessionTTL, "operator link": manager.operatorLinkTTL,
	} {
		if got != defaultSessionTTL {
			t.Fatalf("%s lifetime = %s, want %s", name, got, defaultSessionTTL)
		}
	}
}

func signedInitData(t *testing.T, botToken string, now time.Time, userID int64) string {
	t.Helper()
	values := url.Values{
		"auth_date": {strconv.FormatInt(now.Unix(), 10)},
		"query_id":  {"query"},
		"user":      {`{"id":` + strconv.FormatInt(userID, 10) + `}`},
	}
	keys := []string{"auth_date", "query_id", "user"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(botToken))
	check := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = check.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(check.Sum(nil)))
	return values.Encode()
}
