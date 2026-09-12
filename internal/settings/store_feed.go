package settings

import "fmt"

// FeedBaseline contains immutable baseline values for one chat's subscriptions.
type FeedBaseline struct {
	Lang            BaselineValue[string]
	IntervalSeconds BaselineValue[int]
	Bugs            BaselineValue[bool]
	News            BaselineValue[bool]
	BugProduct      BaselineValue[string]
	BugComponent    BaselineValue[string]
	SilentBugs      BaselineValue[bool]
	GitHubRepos     BaselineValue[[]GitHubRepo]
}

// FeedOverride is the sparse per-chat subscription override.
type FeedOverride struct {
	Lang            *string       `json:"lang,omitempty"`
	IntervalSeconds *int          `json:"interval_seconds,omitempty"`
	Bugs            *bool         `json:"bugs,omitempty"`
	News            *bool         `json:"news,omitempty"`
	BugProduct      *string       `json:"bug_product,omitempty"`
	BugComponent    *string       `json:"bug_component,omitempty"`
	SilentBugs      *bool         `json:"silent_bugs,omitempty"`
	GitHubRepos     *[]GitHubRepo `json:"github_repos,omitempty"`
}

// FeedView is the effective subscription configuration with provenance.
type FeedView struct {
	Lang            Setting[string]
	IntervalSeconds Setting[int]
	Bugs            Setting[bool]
	News            Setting[bool]
	BugProduct      Setting[string]
	BugComponent    Setting[string]
	SilentBugs      Setting[bool]
	GitHubRepos     Setting[[]GitHubRepo]
}

// FeedValidationError identifies one invalid subscription field.
type FeedValidationError struct {
	Field   string
	Code    string
	Message string
}

func (e *FeedValidationError) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// CurrentFeeds returns detached effective destinations that have at least one enabled source.
func (s *Store) CurrentFeeds() []FeedConfig {
	snapshot := s.snapshot.Load()
	feeds := make([]FeedConfig, 0, len(snapshot.groupIDs))
	for _, chatID := range snapshot.groupIDs {
		group := snapshot.groups[chatID]
		view := group.feed
		repos := cloneGitHubRepos(view.GitHubRepos.Value)
		if !view.Bugs.Value && !view.News.Value && len(repos) == 0 {
			continue
		}
		lang := view.Lang.Value
		if lang == "" {
			lang = group.lang.Value
		}
		bugs, news, silent := view.Bugs.Value, view.News.Value, view.SilentBugs.Value
		feeds = append(feeds, FeedConfig{
			ChatID: chatID, Lang: lang, IntervalSeconds: view.IntervalSeconds.Value,
			Bugs: &bugs, News: &news, BugProduct: view.BugProduct.Value,
			BugComponent: view.BugComponent.Value, SilentBugs: &silent, GitHubRepos: repos,
		})
	}
	return feeds
}

// Feed returns a detached effective subscription view.
func (v GroupView) Feed() FeedView {
	feed := v.group.feed
	feed.GitHubRepos = Setting[[]GitHubRepo]{
		Value:  cloneGitHubRepos(feed.GitHubRepos.Value),
		Source: feed.GitHubRepos.Source,
	}
	return feed
}
