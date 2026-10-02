package lookup

import (
	"testing"
	"time"
)

// withDebianReleaseRoles installs live-looking Debian release roles and marks them fresh, so the
// handler under test uses them instead of reaching for distro-info-data.
func withDebianReleaseRoles(t *testing.T, roles map[string]string) {
	t.Helper()
	relInfo.Mu.Lock()
	oldDebian, oldSeries := relInfo.Debian, relInfo.DebianSer
	oldFetched, oldRefreshing := relInfo.Fetched, relInfo.Refreshing
	relInfo.Debian = roles
	relInfo.DebianSer = map[string]bool{}
	relInfo.Fetched = time.Now()
	relInfo.Refreshing = false
	relInfo.Mu.Unlock()
	t.Cleanup(func() {
		relInfo.Mu.Lock()
		relInfo.Debian, relInfo.DebianSer = oldDebian, oldSeries
		relInfo.Fetched, relInfo.Refreshing = oldFetched, oldRefreshing
		relInfo.Mu.Unlock()
	})
}
