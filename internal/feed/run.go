package feed

import (
	"context"
	"log"
	"reflect"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// runState is what one Run holds between ticks: the destinations seen so far, their
// cursors, when each is next due, and which were active on the previous tick.
type runState struct {
	states  map[int64]*feedState
	nextDue map[int64]time.Time
	active  map[int64]*settings.FeedConfig
	ordered []*settings.FeedConfig
}

// Run polls current destinations on a fixed 60-second tick until ctx is canceled.
func (s *Service) Run(ctx context.Context) {
	rs := &runState{states: map[int64]*feedState{}, nextDue: map[int64]time.Time{}, active: map[int64]*settings.FeedConfig{}}
	doPoll := pollAll
	if s.poll != nil {
		doPoll = s.poll
	}
	doProbe := probeFeedPerms
	if s.probe != nil {
		doProbe = s.probe
	}
	tick := func() {
		if newFeeds := s.reconcile(rs); len(newFeeds) > 0 {
			doProbe(ctx, s.bot, newFeeds)
		}
		s.safePoll(ctx, rs, doPoll)
	}
	tick()
	log.Printf("feed: fixed 60-second tick, %d active destination(s)", len(rs.active))
	t := time.NewTicker(feedPollTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			for chatID, st := range rs.states {
				saveFeedState(feedStatePath(s.stateDir, chatID), *st)
			}
			return
		case <-t.C:
			tick()
		}
	}
}

// reconcile reads the provider's current destinations into rs and returns the ones seen
// for the first time or changed since the last tick, which are due at once.
func (s *Service) reconcile(rs *runState) []*settings.FeedConfig {
	current := map[int64]*settings.FeedConfig{}
	ordered := make([]*settings.FeedConfig, 0)
	if s.provider != nil {
		for _, value := range s.provider() {
			if value.ChatID == 0 {
				continue
			}
			f := value
			current[f.ChatID] = &f
			ordered = append(ordered, &f)
		}
	}
	newFeeds := make([]*settings.FeedConfig, 0)
	for chatID, f := range current {
		if _, ok := rs.states[chatID]; !ok {
			st := loadFeedState(feedStatePath(s.stateDir, chatID))
			rs.states[chatID] = &st
		}
		previous, wasActive := rs.active[chatID]
		switch {
		case !wasActive:
			newFeeds = append(newFeeds, f)
			rs.nextDue[chatID] = time.Time{}
		case !reflect.DeepEqual(*previous, *f):
			rs.nextDue[chatID] = time.Time{}
		}
	}
	for chatID := range rs.active {
		if _, ok := current[chatID]; !ok {
			saveFeedState(feedStatePath(s.stateDir, chatID), *rs.states[chatID])
			delete(rs.nextDue, chatID)
		}
	}
	rs.ordered = ordered
	rs.active = current
	return newFeeds
}

func (s *Service) safePoll(ctx context.Context, rs *runState, doPoll pollFunc) {
	if len(rs.ordered) == 0 {
		return
	}
	feeds := append([]*settings.FeedConfig(nil), rs.ordered...)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("feed: poll panicked (recovered, feeds continue): %v", r)
		}
	}()
	doPoll(ctx, s.bot, feeds, rs.states, s.stateDir, time.Now(), rs.nextDue)
}
