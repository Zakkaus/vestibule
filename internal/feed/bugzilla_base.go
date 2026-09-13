package feed

import (
	"context"
	"log"
	"sort"
)

// syncBugzillaState keeps a destination's Bugzilla cursor bound to the base it came from.
func syncBugzillaState(st *feedState, base string) {
	if st.BugzillaBase == nil {
		st.BugzillaBase = &base
		return
	}
	if *st.BugzillaBase == base {
		return
	}
	log.Printf("feed: reset Bugzilla cursor for changed base %q -> %q", *st.BugzillaBase, base)
	st.LastBugID = 0
	st.Tracked = nil
	st.BugzillaBase = &base
}

// bugCursorKey identifies one Bugzilla fetch: destinations sharing a base and cursor share the
// result, and two bases never share one because their bug numbers are unrelated.
type bugCursorKey struct {
	base   string
	cursor int
}

// fetchRecentBugsByCursor fetches each distinct (base, cursor) once, in a stable order so a
// transient failure lands on the same destination between runs.
func fetchRecentBugsByCursor(ctx context.Context, set map[bugCursorKey]bool, sources feedSources) map[bugCursorKey][]recentBug {
	keys := make([]bugCursorKey, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].base == keys[j].base {
			return keys[i].cursor < keys[j].cursor
		}
		return keys[i].base < keys[j].base
	})
	out := make(map[bugCursorKey][]recentBug, len(keys))
	for _, key := range keys {
		fctx, cancel := context.WithTimeout(ctx, feedFetchTimeout)
		bugs, ok := sources.fetchRecent(fctx, key.base, key.cursor)
		cancel()
		if ok {
			out[key] = bugs
		}
	}
	return out
}
