package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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

type blockingAdminChecker struct {
	mu          sync.Mutex
	started     chan struct{}
	release     <-chan struct{}
	inFlight    int
	maxInFlight int
	cachedCalls int
	freshCalls  int
}

func (c *blockingAdminChecker) CachedAdmin(context.Context, int64, int64) (bool, error) {
	return c.check(true)
}

func (c *blockingAdminChecker) FreshAdmin(context.Context, int64, int64) (bool, error) {
	return c.check(false)
}

func (c *blockingAdminChecker) check(cached bool) (bool, error) {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.maxInFlight {
		c.maxInFlight = c.inFlight
	}
	if cached {
		c.cachedCalls++
	} else {
		c.freshCalls++
	}
	c.mu.Unlock()
	c.started <- struct{}{}
	<-c.release
	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
	return true, nil
}

func (c *blockingAdminChecker) counts() (maxInFlight, cachedCalls, freshCalls int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxInFlight, c.cachedCalls, c.freshCalls
}

func TestAccessibleChatsBoundsConcurrentChecks(t *testing.T) {
	const (
		checkLimit     = 8
		candidateCount = 64
	)
	release := make(chan struct{})
	checker := &blockingAdminChecker{
		started: make(chan struct{}, candidateCount),
		release: release,
	}
	manager, err := New(Config{BotToken: "123:token", AdminChecker: checker})
	if err != nil {
		t.Fatal(err)
	}
	candidates := make([]int64, candidateCount)
	for index := range candidates {
		candidates[index] = -int64(index + 1)
	}
	done := make(chan []int64, 1)
	go func() {
		done <- manager.AccessibleChats(context.Background(), Session{
			Principal: Principal{TelegramID: 42},
		}, candidates)
	}()
	started := 0
	for started < checkLimit {
		select {
		case <-checker.started:
			started++
		case <-time.After(250 * time.Millisecond):
			close(release)
			<-done
			t.Fatalf("concurrent checks started = %d, want %d before release", started, checkLimit)
		}
	}
	exceeded := false
	select {
	case <-checker.started:
		exceeded = true
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	allowed := <-done
	maxInFlight, cachedCalls, freshCalls := checker.counts()
	if exceeded || maxInFlight > checkLimit || len(allowed) != candidateCount ||
		cachedCalls != candidateCount || freshCalls != 0 {
		t.Fatalf("max_in_flight=%d limit=%d allowed=%d cached_calls=%d fresh_calls=%d",
			maxInFlight, checkLimit, len(allowed), cachedCalls, freshCalls)
	}
	t.Logf("candidates=%d max_in_flight=%d limit=%d", candidateCount, maxInFlight, checkLimit)
}

func signedInitData(t *testing.T, botToken string, now time.Time, userID int64) string {
	t.Helper()
	return signedInitDataValues(t, botToken, url.Values{
		"auth_date": {strconv.FormatInt(now.Unix(), 10)},
		"query_id":  {"query"},
		"user":      {`{"id":` + strconv.FormatInt(userID, 10) + `}`},
	})
}

func signedInitDataValues(t *testing.T, botToken string, values url.Values) string {
	t.Helper()
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
	_, _ = secret.Write([]byte(botToken))
	check := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = check.Write([]byte(strings.Join(parts, "\n")))
	values.Set("hash", hex.EncodeToString(check.Sum(nil)))
	return values.Encode()
}
