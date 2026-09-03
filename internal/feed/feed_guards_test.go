package feed

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
)

const guardedFeedChatID int64 = -100900000001

type observedFeedBot struct {
	fakeFeedBot
	sendStarted chan time.Time
	editStarted chan time.Time
}

func (b *observedFeedBot) SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	if b.sendStarted != nil {
		b.sendStarted <- time.Now()
	}
	return b.fakeFeedBot.SendMessage(ctx, params)
}

func (b *observedFeedBot) EditMessageText(ctx context.Context, params *telego.EditMessageTextParams) (*telego.Message, error) {
	if b.editStarted != nil {
		b.editStarted <- time.Now()
	}
	return b.fakeFeedBot.EditMessageText(ctx, params)
}

func setFeedPauseForTest(t *testing.T, pause time.Duration) {
	t.Helper()
	old := feedSendPause
	feedSendPause = pause
	t.Cleanup(func() { feedSendPause = old })
}

func TestSuccessfulFeedSendsArePaced(t *testing.T) {
	const pause = 150 * time.Millisecond
	setFeedPauseForTest(t, pause)

	bugsOn, newsOff := true, false
	feed := &settings.FeedConfig{ChatID: guardedFeedChatID, Lang: "en", Bugs: &bugsOn, News: &newsOff}
	state := &feedState{LastBugID: 1000}
	bot := &observedFeedBot{sendStarted: make(chan time.Time, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		postFeedItems(ctx, bot, feed, feedLanguage(feed.Lang), state, []recentBug{
			{ID: 1001, Summary: "first", Status: "CONFIRMED"},
			{ID: 1002, Summary: "second", Status: "CONFIRMED"},
		}, nil)
		close(done)
	}()

	var first, second time.Time
	select {
	case first = <-bot.sendStarted:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("valid feed item never reached the first send")
	}
	select {
	case second = <-bot.sendStarted:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("second valid feed item was not sent")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("feed posting did not stop after cancellation")
	}

	if gap := second.Sub(first); gap < pause {
		t.Fatalf("second feed send started after %s, before the %s pause; catch-up deliveries would burst", gap, pause)
	}
	if state.LastBugID != 1002 {
		t.Fatalf("cursor = %d, want both paced items delivered through 1002", state.LastBugID)
	}
}

func TestRefreshTrackedPacesEditsAndStopsWhenCanceled(t *testing.T) {
	const pause = 400 * time.Millisecond
	setFeedPauseForTest(t, pause)

	feed := &settings.FeedConfig{ChatID: guardedFeedChatID, Lang: "en"}
	state := &feedState{Tracked: map[string]*trackedBug{
		"2001": {MsgID: 11, State: "CONFIRMED|"},
		"2002": {MsgID: 12, State: "CONFIRMED|"},
	}}
	current := map[int]recentBug{
		2001: {ID: 2001, Status: "IN_PROGRESS"},
		2002: {ID: 2002, Status: "IN_PROGRESS"},
	}
	bot := &observedFeedBot{editStarted: make(chan time.Time, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		refreshTracked(ctx, bot, feed, feedLanguage(feed.Lang), state, current, true)
		close(done)
	}()

	select {
	case <-bot.editStarted:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("valid changed bug never reached the first edit")
	}

	timer := time.NewTimer(pause / 4)
	secondBeforePause := false
	select {
	case <-bot.editStarted:
		secondBeforePause = true
	case <-timer.C:
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tracked refresh did not stop after cancellation")
	}

	if secondBeforePause {
		t.Fatal("second tracked edit started before feedSendPause elapsed; refresh would burst edits")
	}
	if bot.edits != 1 {
		t.Fatalf("refresh made %d edits after cancellation, want only the successful edit already in flight; shutdown would keep walking tracked bugs", bot.edits)
	}
}

func TestRateLimitedConfirmationStopsTheRefreshCycle(t *testing.T) {
	setFeedPauseForTest(t, 0)
	feed := &settings.FeedConfig{ChatID: guardedFeedChatID, Lang: "en"}
	current := map[int]recentBug{
		3001: {ID: 3001, Summary: "first", Status: "CONFIRMED"},
		3002: {ID: 3002, Summary: "second", Status: "CONFIRMED"},
	}
	newState := func() *feedState {
		return &feedState{Tracked: map[string]*trackedBug{
			"3001": {MsgID: 21, State: "UNCONFIRMED|"},
			"3002": {MsgID: 22, State: "UNCONFIRMED|"},
		}}
	}

	t.Run("successful confirmations process every changed bug", func(t *testing.T) {
		state := newState()
		base := &fakeFeedBot{}
		bot := &scriptedFeedBot{fakeFeedBot: base}
		refreshTracked(context.Background(), bot, feed, feedLanguage(feed.Lang), state, current, true)
		if base.edits != 2 || base.sends != 2 {
			t.Fatalf("valid confirmations made %d edits and %d sends, want two of each", base.edits, base.sends)
		}
		for id, tracked := range state.Tracked {
			if tracked.State != "CONFIRMED|" {
				t.Fatalf("bug %s state = %q after successful confirmation, want CONFIRMED|", id, tracked.State)
			}
		}
	})

	t.Run("a 429 confirmation stops before the next bug", func(t *testing.T) {
		state := newState()
		base := &fakeFeedBot{}
		bot := &scriptedFeedBot{
			fakeFeedBot: base,
			sendErrs: []error{&telegoapi.Error{
				ErrorCode:   429,
				Description: "Too Many Requests: retry after 30",
			}},
		}
		refreshTracked(context.Background(), bot, feed, feedLanguage(feed.Lang), state, current, true)

		if base.edits != 1 || base.sends != 1 {
			t.Fatalf("429 confirmation made %d edits and %d sends, want one of each; refresh must stop instead of hammering the throttled chat", base.edits, base.sends)
		}
		tries := 0
		for id, tracked := range state.Tracked {
			if tracked.State != "UNCONFIRMED|" {
				t.Fatalf("bug %s advanced to %q after the throttled confirmation, want retryable UNCONFIRMED|", id, tracked.State)
			}
			tries += tracked.ConfirmTries
		}
		if tries != 1 {
			t.Fatalf("confirmation tries = %d, want only the rate-limited attempt recorded", tries)
		}
	})
}

func TestEmptyStateDirectoryKeepsFeedStateInMemory(t *testing.T) {
	setFeedPauseForTest(t, 0)
	if path := feedStatePath("", guardedFeedChatID); path != "" {
		t.Fatalf("empty state directory resolved to %q; the feed would touch the filesystem root", path)
	}

	var writePaths []string
	setFeedStateWriter(t, func(path string, _ any) error {
		writePaths = append(writePaths, path)
		return nil
	})
	saveFeedState("", feedState{LastBugID: 4000})
	if len(writePaths) != 0 {
		t.Fatalf("saving an in-memory feed called the state writer with %q", writePaths)
	}

	bugsOn, newsOff := true, false
	feed := &settings.FeedConfig{ChatID: guardedFeedChatID, Lang: "en", Bugs: &bugsOn, News: &newsOff}
	state := &feedState{LastBugID: 4000}
	now := time.Date(2026, time.September, 3, 0, 0, 0, 0, time.UTC)
	pollAllWithSources(context.Background(), &fakeFeedBot{}, []*settings.FeedConfig{feed}, map[int64]*feedState{
		feed.ChatID: state,
	}, "", now, map[int64]time.Time{feed.ChatID: now}, feedSources{
		recent: func(_ context.Context, after int) ([]recentBug, bool) {
			if after != 4000 {
				t.Fatalf("in-memory cursor = %d, want 4000", after)
			}
			return []recentBug{{ID: 4001, Summary: "memory only", Status: "CONFIRMED"}}, true
		},
		news: func(context.Context) ([]lookup.NewsItem, error) {
			return nil, nil
		},
		tracked: func(context.Context, []int) ([]recentBug, bool) {
			return nil, true
		},
	})

	if state.LastBugID != 4001 {
		t.Fatalf("in-memory cursor = %d after poll, want 4001", state.LastBugID)
	}
	if len(writePaths) != 0 {
		t.Fatalf("polling without a state directory called the state writer with %q", writePaths)
	}
}
