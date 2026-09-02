package auth

import (
	"errors"
	"testing"
	"time"
)

// A one-time console link is posted into a chat, where it stays readable long after the
// operator has used it or forgotten it. Its lifetime is the whole reason that is safe.
// Removing both expiry paths -- the explicit check in RedeemOperatorLink and the sweep in
// pruneLocked -- left every console test passing, so nothing held this.
func TestOperatorLinkExpires(t *testing.T) {
	const ttl = 10 * time.Minute
	for _, tc := range []struct {
		name    string
		elapsed time.Duration
		want    error
	}{
		{"used well inside its lifetime", ttl / 2, nil},
		{"used at the moment it expires", ttl, ErrOperatorLinkExpired},
		{"used after it expired", ttl + time.Second, ErrOperatorLinkExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Unix(1_800_000_000, 0)
			manager, err := New(Config{
				BotToken:        "123:token",
				Now:             func() time.Time { return now },
				OperatorAllowed: func(id int64) bool { return id == 7 },
				OperatorLinkTTL: ttl,
			})
			if err != nil {
				t.Fatal(err)
			}
			link, _, err := manager.IssueOperatorLink(7)
			if err != nil {
				t.Fatal(err)
			}

			now = now.Add(tc.elapsed)
			_, err = manager.RedeemOperatorLink(link)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("redeeming after %s: %v; the link was still inside its %s lifetime",
						tc.elapsed, err, ttl)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("redeeming after %s = %v, want %v; a link posted in a chat outlives "+
					"the reason it was safe to post", tc.elapsed, err, tc.want)
			}
		})
	}
}

// An expired link is spent by the attempt that finds it expired, so a later attempt is told
// it expired rather than being handed a session by a path that no longer prunes.
func TestExpiredOperatorLinkStaysSpent(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	manager, err := New(Config{
		BotToken:        "123:token",
		Now:             func() time.Time { return now },
		OperatorAllowed: func(id int64) bool { return id == 7 },
		OperatorLinkTTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(7)
	if err != nil {
		t.Fatal(err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := manager.RedeemOperatorLink(link); !errors.Is(err, ErrOperatorLinkExpired) {
		t.Fatalf("first attempt after expiry = %v, want %v", err, ErrOperatorLinkExpired)
	}
	if _, err := manager.RedeemOperatorLink(link); !errors.Is(err, ErrOperatorLinkExpired) {
		t.Fatalf("second attempt = %v, want %v: an expired link must not become an unknown "+
			"one, which would say nothing about why it failed", err, ErrOperatorLinkExpired)
	}
}
