package settings

import (
	"strings"
	"testing"
)

func TestLegacyFeedsImportNilBugNewsEnabledAndIsOneTime(t *testing.T) {
	const chatID int64 = -1009000000701
	cfg := &Config{GroupIDs: []int64{chatID}}
	baseline, err := LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	legacy := FeedConfig{ChatID: chatID, IntervalSeconds: 60, GitHubRepos: []GitHubRepo{{Repo: "owner/repo", Issues: ptr(true)}}}
	path := t.TempDir() + "/settings.json"
	store, err := NewStore(path, baseline, nil, []FeedConfig{legacy})
	if err != nil {
		t.Fatal(err)
	}
	view, ok := store.Settings(chatID)
	if !ok {
		t.Fatal("imported chat is not available")
	}
	feed := view.Feed()
	if !feed.Bugs.Value || !feed.News.Value || feed.Bugs.Source != SourceChatOverride || feed.News.Source != SourceChatOverride {
		t.Fatalf("legacy nil booleans = bugs:%v/%v news:%v/%v", feed.Bugs.Value, feed.Bugs.Source, feed.News.Value, feed.News.Source)
	}
	detached := view.Overrides()
	*detached.Feed.News = false
	if current, _ := store.Settings(chatID); !current.Feed().News.Value || current.Revision() != 1 {
		t.Fatalf("detached scalar override mutated store: feed=%+v revision=%d", current.Feed(), current.Revision())
	}
	current := store.CurrentFeeds()
	if len(current) != 1 || current[0].Lang != "zh" || len(current[0].GitHubRepos) != 1 {
		t.Fatalf("current feeds = %+v", current)
	}
	*current[0].Bugs = false
	current[0].GitHubRepos[0].Repo = "mutated/repo"
	*current[0].GitHubRepos[0].Issues = false
	again := store.CurrentFeeds()
	if !*again[0].Bugs || !again[0].GitHubRepos[0].IssuesOn() || again[0].GitHubRepos[0].Repo != "owner/repo" {
		t.Fatalf("current feed provider aliases store state: %+v", again)
	}
	reloaded, err := NewStore(path, baseline, nil, []FeedConfig{legacy})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reloaded.Settings(chatID); got.Revision() != 1 {
		t.Fatalf("legacy feed imported more than once: revision=%d", got.Revision())
	}
}

type feedImportRepository struct {
	record   Record
	casCalls int
}

func (r *feedImportRepository) SeedSettings(records []Record) error {
	if r.record.ChatID == 0 && len(records) > 0 {
		r.record = records[0]
	}
	return nil
}

func (r *feedImportRepository) LoadSettings() ([]Record, error) {
	return []Record{r.record}, nil
}

func (r *feedImportRepository) CompareAndSwapSettings(
	chatID int64,
	expectedRevision uint64,
	next GroupOverrides,
) (uint64, bool, error) {
	r.casCalls++
	if chatID != r.record.ChatID || expectedRevision != r.record.Revision {
		return r.record.Revision, false, nil
	}
	r.record.Revision++
	r.record.Overrides = cloneGroupOverrides(next)
	return r.record.Revision, true, nil
}

func TestLegacyFeedsImportUsesRepositoryCASOnce(t *testing.T) {
	const chatID int64 = -1009000000703
	baseline, err := LoadBaseline("", &Config{GroupIDs: []int64{chatID}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &feedImportRepository{}
	legacy := []FeedConfig{{ChatID: chatID, News: ptr(true)}}
	if _, err := NewStore("", baseline, repository, legacy); err != nil {
		t.Fatal(err)
	}
	if repository.casCalls != 1 || repository.record.Revision != 1 || repository.record.Overrides.Feed == nil {
		t.Fatalf("repository import = calls %d record %+v", repository.casCalls, repository.record)
	}
	if _, err := NewStore("", baseline, repository, legacy); err != nil {
		t.Fatal(err)
	}
	if repository.casCalls != 1 || repository.record.Revision != 1 {
		t.Fatalf("repository feed imported again: calls %d revision %d", repository.casCalls, repository.record.Revision)
	}
}

func TestLegacyFeedsRejectUnmanagedChats(t *testing.T) {
	baseline, err := LoadBaseline("", &Config{GroupIDs: []int64{-1009000000704}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewStore("", baseline, nil, []FeedConfig{{ChatID: 0}, {ChatID: -1009000000799}})
	if err == nil || !strings.Contains(err.Error(), "0, -1009000000799") {
		t.Fatalf("unmanaged legacy feeds error = %v", err)
	}
}

func TestFeedOverrideKeepsNonFeedOverridesAndExplicitEmptyRepos(t *testing.T) {
	const chatID int64 = -1009000000702
	baseline, err := LoadBaseline("", &Config{GroupIDs: []int64{chatID}})
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/settings.json"
	store, err := NewStore(path, baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := store.Settings(chatID)
	enabled := false
	lang := ""
	interval := 300
	bugs := false
	next := view.Overrides()
	next.Enabled = &enabled
	next.Feed = &FeedOverride{
		Lang: &lang, IntervalSeconds: &interval, Bugs: &bugs,
		GitHubRepos: &[]GitHubRepo{},
	}
	if _, err := store.Update(chatID, view.Revision(), next); err != nil {
		t.Fatal(err)
	}
	view, _ = store.Settings(chatID)
	if view.Enabled().Value || view.Enabled().Source != SourceChatOverride {
		t.Fatalf("non-feed override was lost: %+v", view.Enabled())
	}
	feed := view.Feed()
	if feed.GitHubRepos.Source != SourceChatOverride || feed.GitHubRepos.Value == nil || len(feed.GitHubRepos.Value) != 0 {
		t.Fatalf("explicit empty GitHub list = %+v", feed.GitHubRepos)
	}
	compacted := view.Overrides().Feed
	if compacted == nil || compacted.Lang != nil || compacted.IntervalSeconds != nil || compacted.Bugs != nil ||
		compacted.GitHubRepos == nil {
		t.Fatalf("compacted feed override = %+v", compacted)
	}
	reloaded, err := NewStore(path, baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reloadedView, _ := reloaded.Settings(chatID)
	reloadedRepos := reloadedView.Feed().GitHubRepos
	if reloadedRepos.Source != SourceChatOverride || reloadedRepos.Value == nil || len(reloadedRepos.Value) != 0 {
		t.Fatalf("reloaded explicit empty GitHub list = %+v", reloadedRepos)
	}
}
