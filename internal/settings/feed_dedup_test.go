package settings

import "testing"

// Feed state is stored per chat: one file, one cursor pair, keyed by the chat the feed posts
// to. Two feeds naming the same chat would share it, and each would advance the other's
// cursor past items it never posted. Configuration keeps the first and drops the rest, and
// nothing tested that -- removing the check left every test in the repository passing.
func TestConfigKeepsOneFeedPerChat(t *testing.T) {
	first, second := int64(-1009000001001), int64(-1009000001002)
	cfg := &Config{Feeds: []FeedConfig{
		{ChatID: first, Lang: "zh", IntervalSeconds: 300},
		{ChatID: second, Lang: "en", IntervalSeconds: 300},
		{ChatID: first, Lang: "en", IntervalSeconds: 600},
	}}
	if err := normalizeConfigFeeds(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Feeds) != 2 {
		t.Fatalf("feeds after normalisation = %d, want 2; two feeds for one chat share a "+
			"state file and each moves the other's cursor", len(cfg.Feeds))
	}
	if cfg.Feeds[0].ChatID != first || cfg.Feeds[1].ChatID != second {
		t.Fatalf("feeds = %d and %d, want %d and %d in that order; the first entry for a chat "+
			"is the one that survives", cfg.Feeds[0].ChatID, cfg.Feeds[1].ChatID, first, second)
	}
	if cfg.Feeds[0].Lang != "zh" || cfg.Feeds[0].IntervalSeconds != 300 {
		t.Errorf("the surviving feed is %+v, want the first entry's values", cfg.Feeds[0])
	}
}

// A feed with no chat id escapes that check, so two of them both survive and both key their
// state at chat zero. This is the previous generation's behaviour, ported as it stood
// (~/code/refs/gentoo-zh-verify-bot/internal/config/config.go:552 carries the same `!= 0`),
// and it is recorded here rather than quietly changed: such a feed can post nowhere, so the
// cost is a misconfiguration that runs instead of being refused. Changing it would refuse a
// configuration that loads today, which is the maintainer's call.
func TestConfigDoesNotDeduplicateFeedsWithoutAChat(t *testing.T) {
	cfg := &Config{Feeds: []FeedConfig{
		{Lang: "zh", IntervalSeconds: 300},
		{Lang: "en", IntervalSeconds: 300},
	}}
	if err := normalizeConfigFeeds(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Feeds) != 2 {
		t.Fatalf("feeds without a chat id after normalisation = %d, want 2; if this now "+
			"deduplicates, the behaviour changed and the comment above is stale", len(cfg.Feeds))
	}
}
