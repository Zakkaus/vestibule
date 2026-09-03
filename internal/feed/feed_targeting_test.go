package feed

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

// Synthetic destinations for the targeting and scheduling properties below.
const (
	editFeedChat  = -1009000000201
	stateFeedChat = -1009000000202
	fastFeedChat  = -1009000000203
	slowFeedChat  = -1009000000204
)

// recordingFeedBot keeps what fakeFeedBot throws away: the parameters of every edit, and the chat
// each send was addressed to. Without them nothing in the package can observe what an in-place
// edit actually rewrote, or which destination a post reached.
type recordingFeedBot struct {
	*fakeFeedBot
	edited    []*telego.EditMessageTextParams
	sentChats []int64
}

func newRecordingFeedBot() *recordingFeedBot {
	return &recordingFeedBot{fakeFeedBot: &fakeFeedBot{}}
}

func (b *recordingFeedBot) EditMessageText(ctx context.Context, p *telego.EditMessageTextParams) (*telego.Message, error) {
	b.edited = append(b.edited, p)
	return b.fakeFeedBot.EditMessageText(ctx, p)
}

func (b *recordingFeedBot) SendMessage(ctx context.Context, p *telego.SendMessageParams) (*telego.Message, error) {
	b.sentChats = append(b.sentChats, p.ChatID.ID)
	return b.fakeFeedBot.SendMessage(ctx, p)
}

// TestAClosedBugsMessageIsRewrittenWithItsResolutionMarker asserts that the edit a closed bug
// triggers carries the resolved rendering. If it does not, every closed bug in every feed channel
// keeps its 🐞 open marker forever and readers see fixed and WONTFIX'd bugs presented as open —
// the mis-marking the previous generation had to repair across a hundred already-posted messages.
func TestAClosedBugsMessageIsRewrittenWithItsResolutionMarker(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	feed := &settings.FeedConfig{ChatID: editFeedChat, Lang: "en"}
	lang := feedLanguage(feed.Lang)

	for _, tt := range []struct {
		name    string
		bug     recentBug
		want    string
		unwant  string
		explain string
	}{
		{
			name:    "a fixed bug is marked fixed",
			bug:     recentBug{ID: 810, Summary: "fixed one", Status: "RESOLVED", Resolution: "FIXED"},
			want:    "✅",
			unwant:  "🐞",
			explain: "a bug closed as FIXED stays presented as open",
		},
		{
			name:    "a bug closed without a fix is marked so",
			bug:     recentBug{ID: 811, Summary: "wontfix one", Status: "RESOLVED", Resolution: "WONTFIX"},
			want:    "❌",
			unwant:  "🐞",
			explain: "a bug closed as WONTFIX stays presented as open",
		},
		{
			// Positive control: a bug that is still open keeps the open marker.
			name:    "a still-open bug keeps the open marker",
			bug:     recentBug{ID: 812, Summary: "open one", Status: "IN_PROGRESS"},
			want:    "🐞",
			unwant:  "✅",
			explain: "an open bug is rendered as closed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key := strconv.Itoa(tt.bug.ID)
			st := &feedState{Tracked: map[string]*trackedBug{key: {MsgID: 77, State: "CONFIRMED|"}}}
			bot := newRecordingFeedBot()
			refreshTracked(context.Background(), bot, feed, lang, st, map[int]recentBug{tt.bug.ID: tt.bug}, true)
			if len(bot.edited) != 1 {
				t.Fatalf("want exactly one edit, got %d", len(bot.edited))
			}
			text := bot.edited[0].Text
			if !strings.Contains(text, tt.want) || strings.Contains(text, tt.unwant) {
				t.Errorf("bug %d was rewritten as %q; want the %s marker and no %s, otherwise %s in chat %d",
					tt.bug.ID, firstLine(text), tt.want, tt.unwant, tt.explain, feed.ChatID)
			}
		})
	}
}

// TestATrackedEditIsAddressedToItsOwnChatAndMessage asserts the edit is aimed at the tracked bug's
// own message in this feed's own chat and carries that bug's freshly rendered text. All three
// fields were unobservable, so a mis-wired refactor could quietly overwrite an unrelated post in a
// neighbouring chat and every gate would still be green.
func TestATrackedEditIsAddressedToItsOwnChatAndMessage(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	feed := &settings.FeedConfig{ChatID: editFeedChat, Lang: "en"}
	const msgID = 4242
	bug := recentBug{ID: 820, Summary: "kernel oops on resume", Status: "IN_PROGRESS"}
	st := &feedState{Tracked: map[string]*trackedBug{"820": {MsgID: msgID, State: "CONFIRMED|"}}}

	bot := newRecordingFeedBot()
	refreshTracked(context.Background(), bot, feed, feedLanguage(feed.Lang), st, map[int]recentBug{bug.ID: bug}, true)

	if len(bot.edited) != 1 {
		t.Fatalf("want exactly one edit, got %d", len(bot.edited))
	}
	p := bot.edited[0]
	if p.ChatID.ID != feed.ChatID {
		t.Errorf("the edit was addressed to chat %d, not this feed's chat %d: an edit aimed at another chat overwrites somebody else's message",
			p.ChatID.ID, feed.ChatID)
	}
	if p.MessageID != msgID {
		t.Errorf("the edit was addressed to message %d, not the tracked message %d: an edit aimed at another message destroys an unrelated post",
			p.MessageID, msgID)
	}
	if !strings.Contains(p.Text, bug.Summary) || !strings.Contains(p.Text, "bugs.gentoo.org/820") {
		t.Errorf("the edit rewrote message %d as %q, which is not bug %d's rendering: the reader's post is replaced by text belonging to something else",
			msgID, firstLine(p.Text), bug.ID)
	}
}

// TestWithNoStateDirectoryTheFeedNeverWritesToDisk asserts the memory-only mode. Without a state
// directory the cursor path must stay empty and no write may be attempted: otherwise a bot started
// without STATE_DIRECTORY drops feed-<chat>.json at the filesystem root every cycle, which is
// root-owned junk in / when privileged and a failed write per cycle when not.
func TestWithNoStateDirectoryTheFeedNeverWritesToDisk(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	if got := feedStatePath("", stateFeedChat); got != "" {
		t.Errorf("with no state directory feedStatePath returned %q; the feed would write that path, at the filesystem root, every cycle", got)
	}

	var written []string
	original := feedStateWrite
	feedStateWrite = func(path string, _ any) error {
		written = append(written, path)
		return nil
	}
	t.Cleanup(func() { feedStateWrite = original })

	bugsOn, newsOff := true, false
	feed := &settings.FeedConfig{ChatID: stateFeedChat, Lang: "en", Bugs: &bugsOn, News: &newsOff}
	states := map[int64]*feedState{feed.ChatID: {LastBugID: 100}}
	sources := feedSources{
		recent: func(context.Context, int) ([]recentBug, bool) {
			return []recentBug{{ID: 101, Summary: "one", Status: "CONFIRMED"}}, true
		},
		news:    func(context.Context) ([]lookup.NewsItem, error) { return nil, nil },
		tracked: func(context.Context, []int) ([]recentBug, bool) { return nil, true },
	}
	now := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)

	pollAllWithSources(context.Background(), newRecordingFeedBot(), []*settings.FeedConfig{feed}, states,
		"", now, map[int64]time.Time{}, sources)
	if len(written) != 0 {
		t.Errorf("with no state directory the feed attempted %d state write(s), to %v; it must keep its cursors in memory only", len(written), written)
	}

	// Positive control: the recorder does see a write when a state directory IS configured, so the
	// silence above is the mode and not a blind assertion.
	dir := t.TempDir()
	pollAllWithSources(context.Background(), newRecordingFeedBot(), []*settings.FeedConfig{feed}, states,
		dir, now, map[int64]time.Time{}, sources)
	want := feedStatePath(dir, feed.ChatID)
	if len(written) != 1 || written[0] != want {
		t.Errorf("with a state directory the cursor must be persisted to %q, got %v", want, written)
	}
}

// TestASlowFeedSkipsTheFastFeedsCycles asserts that each destination is polled on its own
// configured interval even though one shared ticker runs at the fastest feed's rate. Without it a
// feed configured for an hour is posted to every minute, so its channel receives bug and news
// posts many times more often than the operator asked for.
func TestASlowFeedSkipsTheFastFeedsCycles(t *testing.T) {
	setFeedTestTiming(t, time.Second, time.Second)
	bugsOn, newsOff := true, false
	fast := &settings.FeedConfig{ChatID: fastFeedChat, Lang: "en", IntervalSeconds: 60, Bugs: &bugsOn, News: &newsOff}
	slow := &settings.FeedConfig{ChatID: slowFeedChat, Lang: "en", IntervalSeconds: 3600, Bugs: &bugsOn, News: &newsOff}
	feeds := []*settings.FeedConfig{fast, slow}
	states := map[int64]*feedState{fast.ChatID: {LastBugID: 100}, slow.ChatID: {LastBugID: 100}}

	calls := 0
	sources := feedSources{
		recent: func(context.Context, int) ([]recentBug, bool) {
			calls++
			bugs := []recentBug{{ID: 101, Summary: "one", Status: "CONFIRMED"}}
			if calls > 1 {
				bugs = append(bugs, recentBug{ID: 102, Summary: "two", Status: "CONFIRMED"})
			}
			return bugs, true
		},
		news:    func(context.Context) ([]lookup.NewsItem, error) { return nil, nil },
		tracked: func(context.Context, []int) ([]recentBug, bool) { return nil, true },
	}

	nextDue := map[int64]time.Time{}
	start := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	first := newRecordingFeedBot()
	pollAllWithSources(context.Background(), first, feeds, states, "", start, nextDue, sources)
	if len(first.sentChats) != 2 {
		t.Fatalf("both feeds are due on the first poll, want 2 posts, got %v", first.sentChats)
	}
	for _, f := range feeds {
		if want := start.Add(f.Interval()); !nextDue[f.ChatID].Equal(want) {
			t.Errorf("feed %d is next due at %s, want %s: its own interval_seconds is not what schedules it",
				f.ChatID, nextDue[f.ChatID], want)
		}
	}

	// Two minutes later the one-minute feed is due again and the one-hour feed is not.
	second := newRecordingFeedBot()
	pollAllWithSources(context.Background(), second, feeds, states, "", start.Add(2*time.Minute), nextDue, sources)
	for _, chat := range second.sentChats {
		if chat == slow.ChatID {
			t.Errorf("the one-hour feed %d was posted to two minutes into its interval; it is being polled at the fastest feed's rate, so its channel gets posts %d times more often than configured",
				slow.ChatID, int(slow.Interval()/fast.Interval()))
		}
	}
	if len(second.sentChats) != 1 || second.sentChats[0] != fast.ChatID {
		t.Errorf("the one-minute feed must still be polled on its own schedule, posts went to %v", second.sentChats)
	}
}

// firstLine keeps a failure message readable: feed renderings are multi-line.
func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }
