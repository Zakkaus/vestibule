package feed

import (
	"context"
	"sort"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func collectTrackedBugIDs(due []*settings.FeedConfig, states map[int64]*feedState) map[string]map[int]bool {
	byBase := map[string]map[int]bool{}
	for _, f := range due {
		trackedSet := byBase[f.BugzillaBase]
		if trackedSet == nil {
			trackedSet = map[int]bool{}
			byBase[f.BugzillaBase] = trackedSet
		}
		for key := range states[f.ChatID].Tracked {
			if id, err := strconv.Atoi(key); err == nil {
				trackedSet[id] = true
			}
		}
	}
	return byBase
}

func fetchTrackedBugs(
	ctx context.Context,
	trackedByBase map[string]map[int]bool,
	fetch func(context.Context, string, []int) ([]recentBug, bool),
) (map[string]map[int]recentBug, map[string]bool) {
	bugsByBase := map[string]map[int]recentBug{}
	fetchOKByBase := map[string]bool{}
	bases := make([]string, 0, len(trackedByBase))
	for base := range trackedByBase {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	for _, base := range bases {
		trackedSet := trackedByBase[base]
		if len(trackedSet) == 0 {
			continue
		}
		ids := make([]int, 0, len(trackedSet))
		for id := range trackedSet {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		fetched, ok := fetch(ctx, base, ids)
		byID := make(map[int]recentBug, len(fetched))
		for _, b := range fetched {
			byID[b.ID] = b
		}
		bugsByBase[base] = byID
		fetchOKByBase[base] = ok
	}
	return bugsByBase, fetchOKByBase
}
