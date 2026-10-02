package lookup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureReleaseInfoEmptyDoesNotOverwrite(t *testing.T) {
	relInfo.Mu.Lock()
	relInfo.Debian = map[string]string{"13": "stable"}
	relInfo.DebianSer = map[string]bool{"trixie": true}
	relInfo.ubuntu = map[string]bool{"24.04": true}
	relInfo.Fetched, relInfo.Refreshing = time.Time{}, false // stale => ensureReleaseInfo will refetch
	relInfo.Mu.Unlock()
	t.Cleanup(func() {
		relInfo.Mu.Lock()
		relInfo.Debian, relInfo.DebianSer, relInfo.ubuntu, relInfo.ubuntuRel, relInfo.ubuntuEOL, relInfo.UbuntuSer = nil, nil, nil, nil, nil, nil
		relInfo.Fetched, relInfo.Refreshing = time.Time{}, false
		relInfo.Mu.Unlock()
	})

	od, ou := fetchDebianStatusFn, fetchUbuntuFn
	fetchDebianStatusFn = func(context.Context, time.Time) debianReleaseData { return debianReleaseData{} }
	fetchUbuntuFn = func(context.Context, time.Time) (map[string]bool, map[string]bool, map[string]bool, map[string]bool) {
		return map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	}
	t.Cleanup(func() { fetchDebianStatusFn, fetchUbuntuFn = od, ou })

	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	EnsureReleaseInfo(context.Background(), now)

	relInfo.Mu.Lock()
	defer relInfo.Mu.Unlock()
	if relInfo.Debian["13"] != "stable" || !relInfo.DebianSer["trixie"] || !relInfo.ubuntu["24.04"] {
		t.Error("an empty (malformed-200) fetch must NOT overwrite previously-good cached release data")
	}
	if relInfo.Fetched.Equal(now) {
		t.Error("an empty fetch must take the short retry window (fetched != now), not full-TTL freshness")
	}
}

func TestReleaseMetadataDoesNotRefreshBeforeItsDailyTTLExpires(t *testing.T) {
	resetReleaseInfoForTest(t)

	var debianFetches, ubuntuFetches int
	fetchDebianStatusFn = func(context.Context, time.Time) debianReleaseData {
		debianFetches++
		return debianReleaseData{
			Roles:  map[string]string{"13": "stable"},
			Series: map[string]bool{"trixie": true},
		}
	}
	fetchUbuntuFn = func(context.Context, time.Time) (map[string]bool, map[string]bool, map[string]bool, map[string]bool) {
		ubuntuFetches++
		return map[string]bool{"24.04": true}, map[string]bool{"24.04": true}, map[string]bool{"24.04": true}, map[string]bool{"noble": true}
	}

	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	EnsureReleaseInfo(context.Background(), now)
	EnsureReleaseInfo(context.Background(), now.Add(relInfoTTL-time.Second))
	if debianFetches != 1 || ubuntuFetches != 1 {
		t.Fatalf("release metadata refreshed before its daily TTL expired: Debian=%d Ubuntu=%d, want one fetch each", debianFetches, ubuntuFetches)
	}

	EnsureReleaseInfo(context.Background(), now.Add(relInfoTTL))
	if debianFetches != 2 || ubuntuFetches != 2 {
		t.Errorf("release metadata was not refetched once the daily TTL expired: Debian=%d Ubuntu=%d, want two fetches each", debianFetches, ubuntuFetches)
	}
}

func TestConcurrentColdReleaseMetadataLookupsShareOneRefresh(t *testing.T) {
	resetReleaseInfoForTest(t)

	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	released := false
	releaseFirst := func() {
		if !released {
			close(release)
			released = true
		}
	}
	t.Cleanup(func() {
		releaseFirst()
		select {
		case <-firstDone:
		case <-time.After(time.Second):
			t.Error("the first release metadata refresh did not finish")
		}
	})

	var debianFetches, ubuntuFetches atomic.Int32
	fetchDebianStatusFn = func(context.Context, time.Time) debianReleaseData {
		if debianFetches.Add(1) == 1 {
			close(started)
			<-release
		}
		return debianReleaseData{
			Roles:  map[string]string{"13": "stable"},
			Series: map[string]bool{"trixie": true},
		}
	}
	fetchUbuntuFn = func(context.Context, time.Time) (map[string]bool, map[string]bool, map[string]bool, map[string]bool) {
		ubuntuFetches.Add(1)
		return map[string]bool{"24.04": true}, map[string]bool{"24.04": true}, map[string]bool{"24.04": true}, map[string]bool{"noble": true}
	}

	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	go func() {
		EnsureReleaseInfo(context.Background(), now)
		close(firstDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the first cold release metadata lookup did not begin its fetch")
	}

	secondDone := make(chan struct{})
	go func() {
		EnsureReleaseInfo(context.Background(), now)
		close(secondDone)
	}()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("a concurrent cold lookup blocked instead of returning the current release metadata")
	}
	if got := debianFetches.Load(); got != 1 {
		t.Fatalf("a concurrent cold lookup started a second release metadata refresh: Debian CSV fetched %d times, want 1", got)
	}

	releaseFirst()
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("the first release metadata refresh did not finish")
	}
	if debianFetches.Load() != 1 || ubuntuFetches.Load() != 1 {
		t.Errorf("cold lookup coalescing fetched release metadata more than once: Debian=%d Ubuntu=%d", debianFetches.Load(), ubuntuFetches.Load())
	}
}

func TestFailedColdReleaseMetadataRefreshIsMarkedAttempted(t *testing.T) {
	resetReleaseInfoForTest(t)

	var debianFetches, ubuntuFetches int
	fetchDebianStatusFn = func(context.Context, time.Time) debianReleaseData {
		debianFetches++
		return debianReleaseData{}
	}
	fetchUbuntuFn = func(context.Context, time.Time) (map[string]bool, map[string]bool, map[string]bool, map[string]bool) {
		ubuntuFetches++
		return map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	}

	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	EnsureReleaseInfo(context.Background(), now)
	relInfo.Mu.Lock()
	attempted := relInfo.Debian != nil
	relInfo.Mu.Unlock()
	if !attempted {
		t.Fatal("a failed cold refresh left release metadata unmarked, so every lookup retries upstream")
	}

	EnsureReleaseInfo(context.Background(), now.Add(relInfoRetryTTL-time.Second))
	if debianFetches != 1 || ubuntuFetches != 1 {
		t.Errorf("a failed refresh retried before its short retry window expired: Debian=%d Ubuntu=%d, want one fetch each", debianFetches, ubuntuFetches)
	}
}

func resetReleaseInfoForTest(t *testing.T) {
	t.Helper()
	relInfo.Mu.Lock()
	oldDebian, oldDebianSer := relInfo.Debian, relInfo.DebianSer
	oldUbuntu, oldUbuntuRel, oldUbuntuEOL, oldUbuntuSer := relInfo.ubuntu, relInfo.ubuntuRel, relInfo.ubuntuEOL, relInfo.UbuntuSer
	oldFetched, oldRefreshing := relInfo.Fetched, relInfo.Refreshing
	relInfo.Debian, relInfo.DebianSer, relInfo.ubuntu, relInfo.ubuntuRel, relInfo.ubuntuEOL, relInfo.UbuntuSer = nil, nil, nil, nil, nil, nil
	relInfo.Fetched, relInfo.Refreshing = time.Time{}, false
	relInfo.Mu.Unlock()

	oldDebianFetch, oldUbuntuFetch := fetchDebianStatusFn, fetchUbuntuFn
	t.Cleanup(func() {
		relInfo.Mu.Lock()
		relInfo.Debian, relInfo.DebianSer = oldDebian, oldDebianSer
		relInfo.ubuntu, relInfo.ubuntuRel, relInfo.ubuntuEOL, relInfo.UbuntuSer = oldUbuntu, oldUbuntuRel, oldUbuntuEOL, oldUbuntuSer
		relInfo.Fetched, relInfo.Refreshing = oldFetched, oldRefreshing
		relInfo.Mu.Unlock()
		fetchDebianStatusFn, fetchUbuntuFn = oldDebianFetch, oldUbuntuFetch
	})
}

func TestDeriveDebianStatus(t *testing.T) {
	csv := `version,codename,series,created,release,eol
11,Bullseye,bullseye,2019-07-06,2021-08-14,2024-08-14
12,Bookworm,bookworm,2021-08-14,2023-06-10,2026-07-11
13,Trixie,trixie,2023-06-10,2025-08-09,2028-08-09
14,Forky,forky,2025-08-09
,Sid,sid,1993-08-16`

	now := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC) // Trixie released, Forky not
	got := deriveDebianStatus(csv, now)
	for ver, want := range map[string]string{"13": "stable", "12": "oldstable", "11": "oldoldstable", "14": "testing"} {
		if got[ver] != want {
			t.Errorf("now=2026: status[%s] = %q, want %q", ver, got[ver], want)
		}
	}
	series := deriveDebianReleaseData(csv, now).Series
	if !series["trixie"] || series["forky"] || series["sid"] {
		t.Errorf("now=2026: Debian series release state = %v", series)
	}

	// After Forky releases (its row now carries a release date), stable becomes 14 with no
	// code change — the mapping is purely date-driven.
	csv2 := `version,codename,series,created,release,eol
12,Bookworm,bookworm,2021-08-14,2023-06-10,2026-07-11
13,Trixie,trixie,2023-06-10,2025-08-09,2028-08-09
14,Forky,forky,2025-08-09,2026-08-01,2029-08-01
15,Duke,duke,2027-08-01
,Sid,sid,1993-08-16`
	later := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if g := deriveDebianStatus(csv2, later); g["14"] != "stable" || g["13"] != "oldstable" {
		t.Errorf("after Forky release: 14=%q 13=%q, want stable/oldstable", g["14"], g["13"])
	}
}

func TestUbuntuExcluded(t *testing.T) {
	relInfo.Mu.Lock()
	relInfo.ubuntuRel = map[string]bool{"18.04": true, "20.04": true, "24.04": true, "26.04": true, "26.10": false}
	relInfo.ubuntuEOL = map[string]bool{"18.04": true, "20.04": true}
	relInfo.Mu.Unlock()
	defer func() {
		relInfo.Mu.Lock()
		relInfo.ubuntuRel, relInfo.ubuntuEOL = nil, nil
		relInfo.Mu.Unlock()
	}()

	for label, want := range map[string]bool{
		"26.10": true, "24.04": false, "26.04": false,
		"26.10.proposed": true, "24.04.backports": true, "99.99": false,
		"18.04": true, "20.04": true, // past standard end-of-life
	} {
		if got := UbuntuExcluded(label); got != want {
			t.Errorf("ubuntuExcluded(%q) = %v, want %v", label, got, want)
		}
	}
}

func TestDeriveDebianStatusEmpty(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	for _, body := range []string{
		"", // empty body
		"version,codename,series,created,release,eol", // header only, no data rows
		"<html>503 Service Unavailable</html>",        // an error page, not CSV
		"garbage,no,real,release,dates,here",          // a row with no past release date
	} {
		if got := deriveDebianStatus(body, now); len(got) != 0 {
			t.Errorf("deriveDebianStatus(%q) = %v, want empty (so ensureReleaseInfo treats it as a failed fetch)", body, got)
		}
	}
}

func TestRelInfoNextFetched(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if got := relInfoNextFetched(now, true); !got.Equal(now) {
		t.Errorf("bothOK should mark fresh at now, got %v", got)
	}
	marker := relInfoNextFetched(now, false)
	// the freshness gate is now.Sub(fetched) < relInfoTTL; the failure window must equal retryTTL.
	if window := relInfoTTL - now.Sub(marker); window != relInfoRetryTTL {
		t.Errorf("failure freshness window = %v, want %v", window, relInfoRetryTTL)
	}
	if now.Add(relInfoRetryTTL-time.Minute).Sub(marker) >= relInfoTTL {
		t.Error("should still be fresh just before the retry TTL elapses")
	}
	if now.Add(relInfoRetryTTL+time.Minute).Sub(marker) < relInfoTTL {
		t.Error("should be stale just after the retry TTL elapses (triggering a refetch)")
	}
}

func withUbuntuSupportEnd(t *testing.T) {
	relInfo.Mu.Lock()
	oldLTS, oldEOL := relInfo.ubuntu, relInfo.ubuntuEOL
	relInfo.ubuntu = map[string]bool{"18.04": true, "24.04": true}
	relInfo.ubuntuEOL = map[string]bool{"18.04": true, "22.10": true}
	relInfo.Mu.Unlock()
	t.Cleanup(func() {
		relInfo.Mu.Lock()
		relInfo.ubuntu, relInfo.ubuntuEOL = oldLTS, oldEOL
		relInfo.Mu.Unlock()
	})

}
