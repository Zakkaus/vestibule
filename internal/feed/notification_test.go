package feed

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func withoutFeedPacing(t *testing.T) {
	t.Helper()
	oldPause := feedSendPause
	feedSendPause = 0
	t.Cleanup(func() { feedSendPause = oldPause })
}

func TestRefreshTrackedDoesNotConfirmPingAnUnconfirmedBugThatResolves(t *testing.T) {
	withoutFeedPacing(t)
	f := &settings.FeedConfig{ChatID: -100900000711, Lang: "en"}

	st := &feedState{Tracked: map[string]*trackedBug{"711": {MsgID: 11, State: "UNCONFIRMED|"}}}
	fb := &fakeFeedBot{}
	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		711: {ID: 711, Status: "RESOLVED", Resolution: "INVALID", Summary: "not actionable"},
	}, true)

	if fb.edits != 1 {
		t.Fatalf("a resolved report must update its original message, got %d edits", fb.edits)
	}
	if fb.sends != 0 {
		t.Fatalf("an UNCONFIRMED bug resolved before confirmation must not ring the feed, got %d confirm pings", fb.sends)
	}
	if got := st.Tracked["711"]; got == nil || got.State != "RESOLVED|INVALID" {
		t.Fatalf("the resolution edit must advance tracked state, got %+v", got)
	}

	st = &feedState{Tracked: map[string]*trackedBug{"712": {MsgID: 12, State: "UNCONFIRMED|"}}}
	fb = &fakeFeedBot{}
	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		712: {ID: 712, Status: "CONFIRMED", Summary: "actionable"},
	}, true)
	if fb.sends != 1 {
		t.Fatalf("a non-resolved transition out of UNCONFIRMED must ring the feed once, got %d confirm pings", fb.sends)
	}
}

func TestRefreshTrackedDoesNotConfirmPingAReopenedBugStillUnconfirmed(t *testing.T) {
	withoutFeedPacing(t)
	f := &settings.FeedConfig{ChatID: -100900000721, Lang: "en"}

	st := &feedState{Tracked: map[string]*trackedBug{"721": {MsgID: 21, State: "UNCONFIRMED|INVALID"}}}
	fb := &fakeFeedBot{}
	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		721: {ID: 721, Status: "UNCONFIRMED", Summary: "reopened report"},
	}, true)

	if fb.edits != 1 {
		t.Fatalf("a reopened report must update its original message, got %d edits", fb.edits)
	}
	if fb.sends != 0 {
		t.Fatalf("a reopened bug still UNCONFIRMED must not ring the feed, got %d confirm pings", fb.sends)
	}
	if got := st.Tracked["721"]; got == nil || got.State != "UNCONFIRMED|" {
		t.Fatalf("the reopened unconfirmed state must be retained, got %+v", got)
	}

	st = &feedState{Tracked: map[string]*trackedBug{"722": {MsgID: 22, State: "UNCONFIRMED|"}}}
	fb = &fakeFeedBot{}
	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		722: {ID: 722, Status: "CONFIRMED", Summary: "confirmed report"},
	}, true)
	if fb.sends != 1 {
		t.Fatalf("a transition from UNCONFIRMED to CONFIRMED must ring the feed once, got %d confirm pings", fb.sends)
	}
}

func TestRefreshTrackedResetsConfirmRetriesAfterASuccessfulPing(t *testing.T) {
	withoutFeedPacing(t)
	f := &settings.FeedConfig{ChatID: -100900000731, Lang: "en"}
	st := &feedState{Tracked: map[string]*trackedBug{
		"731": {MsgID: 31, State: "UNCONFIRMED|", ConfirmTries: maxConfirmTries - 1},
	}}
	fb := &fakeFeedBot{}

	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		731: {ID: 731, Status: "CONFIRMED", Summary: "first confirmation"},
	}, true)
	if fb.sends != 1 {
		t.Fatalf("the first confirmation must be delivered, got %d confirm pings", fb.sends)
	}

	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		731: {ID: 731, Status: "UNCONFIRMED", Summary: "reopened"},
	}, true)
	fb.sendErr = errors.New("Bad Gateway")
	refreshTracked(context.Background(), fb, f, feedLanguage(f.Lang), st, map[int]recentBug{
		731: {ID: 731, Status: "CONFIRMED", Summary: "second confirmation"},
	}, true)

	got := st.Tracked["731"]
	if got == nil || got.State != "UNCONFIRMED|" {
		t.Fatalf("a failed later confirmation must retain its full retry budget, got %+v", got)
	}
	if got.ConfirmTries != 1 {
		t.Fatalf("a successful earlier confirmation must reset retry attempts, got %d after one later failure", got.ConfirmTries)
	}
}

func TestPostFeedItemsDeliversComputedSilentBugsWithoutNotification(t *testing.T) {
	withoutFeedPacing(t)
	forcedSilent := true
	tests := []struct {
		name string
		feed settings.FeedConfig
		bug  recentBug
	}{
		{
			name: "an unconfirmed bug",
			feed: settings.FeedConfig{ChatID: -100900000741, Lang: "en"},
			bug:  recentBug{ID: 741, Status: "UNCONFIRMED", Summary: "new report"},
		},
		{
			name: "a feed configured silent",
			feed: settings.FeedConfig{ChatID: -100900000742, Lang: "en", SilentBugs: &forcedSilent},
			bug:  recentBug{ID: 742, Status: "CONFIRMED", Summary: "configured silent"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &feedState{LastBugID: tt.bug.ID - 1}
			fb := &fakeFeedBot{}
			postFeedItems(context.Background(), fb, &tt.feed, feedLanguage(tt.feed.Lang), st, []recentBug{tt.bug}, nil)
			if fb.sends != 1 {
				t.Fatalf("a new silent bug must be delivered once, got %d sends", fb.sends)
			}
			if !fb.sentSilent[0] {
				t.Fatal("a computed-silent bug must set DisableNotification so it does not ping the feed")
			}
		})
	}

	f := &settings.FeedConfig{ChatID: -100900000743, Lang: "en"}
	st := &feedState{LastBugID: 742}
	fb := &fakeFeedBot{}
	postFeedItems(context.Background(), fb, f, feedLanguage(f.Lang), st, []recentBug{{ID: 743, Status: "CONFIRMED", Summary: "ordinary report"}}, nil)
	if fb.sends != 1 || fb.sentSilent[0] {
		t.Fatalf("an ordinary confirmed bug must remain the audible control, sends=%d silent=%v", fb.sends, fb.sentSilent)
	}
}
