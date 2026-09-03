package feed

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// Synthetic destinations for the delivery properties below. Reserved -1009 block only.
const (
	silentFeedChat = -1009000000101
	pacedFeedChat  = -1009000000102
	pingFeedChat   = -1009000000103
)

// TestSilentItemsReachTelegramWithNotificationsDisabled asserts that an item the feed decided to
// post silently is actually delivered with DisableNotification set. bugSilent is separately
// tested, but the decision only matters if it survives the send: without this the whole
// noise-suppression design (fresh UNCONFIRMED reports stay quiet, silent_bugs=true silences a
// destination) is a no-op and every filed bug notifies the channel.
func TestSilentItemsReachTelegramWithNotificationsDisabled(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	bugsOn, newsOff, forced := true, false, true
	plain := func() *settings.FeedConfig {
		return &settings.FeedConfig{ChatID: silentFeedChat, Lang: "en", Bugs: &bugsOn, News: &newsOff}
	}
	silenced := plain()
	silenced.SilentBugs = &forced

	for _, tt := range []struct {
		name       string
		feed       *settings.FeedConfig
		bug        recentBug
		wantSilent bool
	}{
		{
			name:       "a fresh UNCONFIRMED report arrives without a notification",
			feed:       plain(),
			bug:        recentBug{ID: 105, Summary: "maybe a bug", Status: "UNCONFIRMED"},
			wantSilent: true,
		},
		{
			name:       "silent_bugs silences a bug that would otherwise notify",
			feed:       silenced,
			bug:        recentBug{ID: 105, Summary: "confirmed one", Status: "CONFIRMED"},
			wantSilent: true,
		},
		{
			// Positive control: the flag is not simply always set.
			name:       "a CONFIRMED bug on a plain feed still notifies",
			feed:       plain(),
			bug:        recentBug{ID: 105, Summary: "confirmed one", Status: "CONFIRMED"},
			wantSilent: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fb := &fakeFeedBot{}
			st := &feedState{LastBugID: 100}
			postFeedItems(context.Background(), newAPITestBot(t, fb), tt.feed,
				feedLanguage(tt.feed.Lang), st, []recentBug{tt.bug}, nil)
			if len(fb.sentSilent) != 1 {
				t.Fatalf("want exactly one delivery, got %d", len(fb.sentSilent))
			}
			if fb.sentSilent[0] != tt.wantSilent {
				t.Errorf("bug %d reached chat %d with DisableNotification=%v, want %v: the feed's silence decision is not carried into the send, so subscribers are notified for posts the operator asked to keep quiet",
					tt.bug.ID, tt.feed.ChatID, fb.sentSilent[0], tt.wantSilent)
			}
		})
	}
}

// TestFeedSendsArePacedSoACatchUpBacklogDoesNotBurst asserts that the loop actually waits
// feedSendPause after each delivered item. A catch-up cycle after downtime can carry up to
// recentBugsLimit bugs; fired back to back they are rate-limited and the tail of the backlog is
// dropped for that cycle, exactly when the bot is recovering.
func TestFeedSendsArePacedSoACatchUpBacklogDoesNotBurst(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second) // restores feedSendPause on cleanup
	const pause = 25 * time.Millisecond
	feedSendPause = pause

	bugsOn, newsOff := true, false
	feed := &settings.FeedConfig{ChatID: pacedFeedChat, Lang: "en", Bugs: &bugsOn, News: &newsOff}
	bugs := []recentBug{
		{ID: 101, Summary: "one", Status: "CONFIRMED"},
		{ID: 102, Summary: "two", Status: "CONFIRMED"},
		{ID: 103, Summary: "three", Status: "CONFIRMED"},
	}
	fb := &fakeFeedBot{}
	st := &feedState{LastBugID: 100}

	started := time.Now()
	postFeedItems(context.Background(), newAPITestBot(t, fb), feed, feedLanguage(feed.Lang), st, bugs, nil)
	elapsed := time.Since(started)

	if fb.sends != len(bugs) {
		t.Fatalf("want %d deliveries, got %d", len(bugs), fb.sends)
	}
	if floor := time.Duration(len(bugs)-1) * pause; elapsed < floor {
		t.Errorf("%d feed sends finished in %s; each delivery must be followed by a %s pause (at least %s for this backlog), otherwise a catch-up burst is fired at Telegram with no gap and the tail of the backlog is rejected",
			len(bugs), elapsed, pause, floor)
	}
}

// TestARateLimitedConfirmPingStopsTheRefreshCycle asserts that when Telegram throttles a confirm
// ping the whole refresh stops rather than working through the remaining tracked bugs. Continuing
// deepens the rate limit on a chat that is already throttled, so ordinary bug posts to that
// channel keep being rejected for longer.
func TestARateLimitedConfirmPingStopsTheRefreshCycle(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	feed := &settings.FeedConfig{ChatID: pingFeedChat, Lang: "en"}
	lang := feedLanguage(feed.Lang)
	byID := map[int]recentBug{
		700: {ID: 700, Summary: "first", Status: "CONFIRMED"},
		701: {ID: 701, Summary: "second", Status: "CONFIRMED"},
	}
	tracked := func() *feedState {
		return &feedState{Tracked: map[string]*trackedBug{
			"700": {MsgID: 9, State: "UNCONFIRMED|"},
			"701": {MsgID: 10, State: "UNCONFIRMED|"},
		}}
	}

	st := tracked()
	throttled := &fakeFeedBot{sendErr: errors.New("Too Many Requests: retry after 30")}
	refreshTracked(context.Background(), throttled, feed, lang, st, byID, true)
	if throttled.sends != 1 || throttled.edits != 1 {
		t.Errorf("a 429 on a confirm ping left the cycle running: %d edits and %d pings went to chat %d, so the feed keeps hammering a chat Telegram is already throttling; want 1 and 1",
			throttled.edits, throttled.sends, feed.ChatID)
	}
	for _, id := range []string{"700", "701"} {
		if tb := st.Tracked[id]; tb == nil || tb.State != "UNCONFIRMED|" {
			t.Errorf("bug %s must keep its old state after a throttled cycle so the transition retries, got %+v", id, tb)
		}
	}

	// Positive control: only a 429 stops the cycle. A transient 502 is not a throttle signal, so
	// the refresh must keep working through the tracked map.
	transient := &fakeFeedBot{sendErr: errors.New("Bad Gateway")}
	refreshTracked(context.Background(), transient, feed, lang, tracked(), byID, true)
	if transient.sends != 2 || transient.edits != 2 {
		t.Errorf("a transient 502 must not stop the refresh, got %d edits and %d pings, want 2 and 2",
			transient.edits, transient.sends)
	}
}

// TestABugThatClosesOrStaysUnconfirmedDoesNotPingTheChannel asserts the two conjuncts that keep
// the confirm ping quiet. A bug filed and closed between two polls (the common
// UNCONFIRMED -> RESOLVED/INVALID path on Gentoo Bugzilla) is told by its ✅/❌ edit alone, and a
// bug reopened back into UNCONFIRMED was not confirmed at all. Pinging for either notifies the
// whole channel for exactly the reports the silent-UNCONFIRMED rule exists to keep quiet.
func TestABugThatClosesOrStaysUnconfirmedDoesNotPingTheChannel(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	feed := &settings.FeedConfig{ChatID: pingFeedChat, Lang: "en"}
	lang := feedLanguage(feed.Lang)

	for _, tt := range []struct {
		name      string
		state     string
		bug       recentBug
		wantSends int
	}{
		{
			name:      "filed and closed between two polls",
			state:     "UNCONFIRMED|",
			bug:       recentBug{ID: 710, Summary: "spam", Status: "RESOLVED", Resolution: "INVALID"},
			wantSends: 0,
		},
		{
			name:      "reopened back into UNCONFIRMED",
			state:     "UNCONFIRMED|INVALID",
			bug:       recentBug{ID: 711, Summary: "reopened", Status: "UNCONFIRMED"},
			wantSends: 0,
		},
		{
			// Positive control: a bug that really was confirmed still pings.
			name:      "actually confirmed",
			state:     "UNCONFIRMED|",
			bug:       recentBug{ID: 712, Summary: "real", Status: "CONFIRMED"},
			wantSends: 1,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := strconv.Itoa(tt.bug.ID)
			st := &feedState{Tracked: map[string]*trackedBug{key: {MsgID: 9, State: tt.state}}}
			fb := &fakeFeedBot{}
			refreshTracked(context.Background(), fb, feed, lang, st, map[int]recentBug{tt.bug.ID: tt.bug}, true)
			if fb.edits != 1 {
				t.Fatalf("the in-place edit must still happen, got %d edits", fb.edits)
			}
			if fb.sends != tt.wantSends {
				got := ""
				if len(fb.sentText) > 0 {
					got = strings.SplitN(fb.sentText[0], "\n", 2)[0]
				}
				t.Errorf("bug %d (%s -> %s) produced %d confirm pings into chat %d, want %d: %q",
					tt.bug.ID, tt.state, bugStateKey(tt.bug), fb.sends, feed.ChatID, tt.wantSends, got)
			}
			if tb := st.Tracked[key]; tb == nil || tb.State != bugStateKey(tt.bug) {
				t.Errorf("state must advance to the rendered one, got %+v", tb)
			}
		})
	}
}
