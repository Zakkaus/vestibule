package feed

import (
	"context"
	"sort"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func collectTrackedBugIDs(due []*settings.FeedConfig, states map[int64]*feedState) map[int]bool {
	trackedSet := map[int]bool{}
	for _, f := range due {
		for k := range states[f.ChatID].Tracked {
			if id, err := strconv.Atoi(k); err == nil {
				trackedSet[id] = true
			}
		}
	}
	return trackedSet
}

func fetchTrackedBugs(ctx context.Context, trackedSet map[int]bool, fetch func(context.Context, []int) ([]recentBug, bool)) (map[int]recentBug, bool) {
	var byID map[int]recentBug
	fetchOK := false
	if len(trackedSet) > 0 {
		ids := make([]int, 0, len(trackedSet))
		for id := range trackedSet {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		byID = map[int]recentBug{}
		fetched, ok := fetch(ctx, ids)
		for _, b := range fetched {
			byID[b.ID] = b
		}
		fetchOK = ok
	}
	return byID, fetchOK
}
