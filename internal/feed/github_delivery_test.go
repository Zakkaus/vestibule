package feed

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/lookup"
)

func TestGitHubSuccessfulSendsArePaced(t *testing.T) {
	const pause = 20 * time.Millisecond
	setFeedPauseForTest(t, pause)
	feed := githubTestFeed()
	key := githubRepoKey("o/r", "")
	state := &feedState{GitHub: &githubState{Repos: map[string]githubRepoState{key: {LastID: githubTestCommit(0).ID}}}}
	caller := &primaryCaller{}
	bot := newAPITestBot(t, caller)
	fetch := func(context.Context, string, string) ([]lookup.Commit, error) {
		return []lookup.Commit{githubTestCommit(2), githubTestCommit(1), githubTestCommit(0)}, nil
	}
	pollGitHubWithFetcher(context.Background(), bot, feed, state, fetch)
	if len(caller.sentAt) != 2 {
		t.Fatalf("serialized sends = %d, want 2", len(caller.sentAt))
	}
	if elapsed := caller.sentAt[1].Sub(caller.sentAt[0]); elapsed < pause {
		t.Fatalf("inter-send interval = %s, want at least %s", elapsed, pause)
	}
}
