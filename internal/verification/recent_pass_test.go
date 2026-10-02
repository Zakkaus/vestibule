package verification

import (
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type recentPassStore struct {
	testVerificationStore
	record RecentPassRecord
}

func (s recentPassStore) LoadRecentPasses(string, int64, int64) ([]RecentPassRecord, error) {
	return []RecentPassRecord{s.record}, nil
}

func TestReviewRecentPassExpiresFromAdmissionNotRestart(t *testing.T) {
	passedAt := time.Unix(1800000000, 0)
	now := passedAt.Add(4*time.Minute + 30*time.Second)
	v := newTestService(&settings.Config{})
	v.timeNow = func() time.Time { return now }
	v.stateStore = recentPassStore{record: RecentPassRecord{GroupID: -1009000000911, UserID: 911, PassedAt: passedAt.Unix()}}
	if err := v.loadRecentPasses(); err != nil {
		t.Fatal(err)
	}
	if !v.recentlyPassed(-1009000000911, 911) {
		t.Fatal("restart discarded the remaining suppression window")
	}
	now = passedAt.Add(5*time.Minute + time.Second)
	if v.recentlyPassed(-1009000000911, 911) {
		t.Fatal("restart extended suppression beyond five minutes after admission")
	}
}
