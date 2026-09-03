package verification

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestVerifyStrikes(t *testing.T) {
	c := &settings.Config{VerifyMaxFails: 3, VerifyRetrySeconds: 180}
	path := filepath.Join(t.TempDir(), "verify-fails.json")
	v := newTestService(c)
	v.vfailPath = path

	for i := 1; i <= 2; i++ {
		count, ban := v.recordVerifyFail(-100, 42, v.wallNow())
		if count != i || ban {
			t.Errorf("strike %d before restart = (%d, %v), want (%d, false)", i, count, ban, i)
		}
	}

	restored := newTestService(c)
	restored.vfailPath = path
	restored.loadVerifyFails()
	if remaining := restored.verifyCooldownRemaining(-100, 42); remaining <= 0 {
		t.Errorf("restored cooldown = %v, want an active cooldown", remaining)
	}
	if count, ban := restored.recordVerifyFail(-100, 42, restored.wallNow()); count != 3 || !ban {
		t.Errorf("first strike after restart = (%d, %v), want (3, true)", count, ban)
	}

	restored.clearVerifyFails(-100, 42)
	if remaining := restored.verifyCooldownRemaining(-100, 42); remaining != 0 {
		t.Errorf("cooldown after clear = %v, want 0", remaining)
	}
	if count, _ := restored.recordVerifyFail(-100, 42, restored.wallNow()); count != 1 {
		t.Errorf("strikes after clear restart at %d, want 1", count)
	}
	if count, ban := restored.recordVerifyFail(-100, 99, restored.wallNow()); count != 1 || ban {
		t.Errorf("independent user's first strike = (%d, %v), want (1, false)", count, ban)
	}
}

func TestVerifyNoAutoBan(t *testing.T) {
	v := newTestService(&settings.Config{VerifyMaxFails: -1})
	for i := range 10 {
		if _, ban := v.recordVerifyFail(-100, 7, v.wallNow()); ban {
			t.Fatalf("auto-ban should be disabled with verify_max_fails=-1 (fired at strike %d)", i+1)
		}
	}
}

func TestVerifyCooldownDisabled(t *testing.T) {
	v := newTestService(&settings.Config{VerifyRetrySeconds: -1})
	v.recordVerifyFail(-100, 5, v.wallNow())
	if v.verifyCooldownRemaining(-100, 5) != 0 {
		t.Error("cooldown should be disabled with verify_retry_seconds=-1")
	}
}

func TestVerifyStrikeDecay(t *testing.T) {
	v := newTestService(&settings.Config{VerifyMaxFails: 3})
	if count, _ := v.recordVerifyFail(-100, 42, v.wallNow()); count != 1 {
		t.Fatalf("first strike count=%d, want 1", count)
	}
	// back-date the last failure beyond the window
	v.mu.Lock()
	v.vfail[pkey{-100, 42}].last = time.Now().Add(-verifyFailWindow - time.Minute)
	v.mu.Unlock()
	if count, ban := v.recordVerifyFail(-100, 42, v.wallNow()); count != 1 || ban {
		t.Errorf("after window elapsed, strike = (%d,%v), want fresh (1,false)", count, ban)
	}
}

func TestSaveVerifyFailsPrunesOnlyFullyExpiredRecords(t *testing.T) {
	const (
		shortGroup int64 = -100
		longGroup  int64 = -200
	)
	cfg := &settings.Config{
		Groups:             []settings.GroupConfig{{ID: shortGroup}, {ID: longGroup}},
		GroupIDs:           []int64{shortGroup, longGroup},
		VerifyMaxFails:     3,
		VerifyRetrySeconds: 30,
	}
	v := newTestService(cfg)
	group, ok := v.settings.Settings(longGroup)
	if !ok {
		t.Fatal("long-cooldown group is missing")
	}
	overrides := group.Overrides()
	longRetrySeconds := int((8 * time.Hour) / time.Second)
	overrides.VerifyRetrySeconds = &longRetrySeconds
	if _, err := v.settings.Update(longGroup, group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	dead := pkey{shortGroup, 1}
	cooldownLive := pkey{longGroup, 2}
	historyLive := pkey{shortGroup, 3}
	v.vfail[dead] = &vfailRec{count: 1, last: now.Add(-7 * time.Hour)}
	v.vfail[cooldownLive] = &vfailRec{count: 1, last: now.Add(-7 * time.Hour)}
	v.vfail[historyLive] = &vfailRec{count: 2, last: now.Add(-verifyFailWindow + time.Minute)}
	v.vfailPath = filepath.Join(t.TempDir(), "verify-fails.json")

	v.saveVerifyFails()

	if _, ok := v.vfail[dead]; ok {
		t.Error("a record with expired strike and cooldown windows was retained")
	}
	if remaining := v.verifyCooldownRemaining(cooldownLive.gid, cooldownLive.uid); remaining <= 0 {
		t.Errorf("group-specific live cooldown was pruned, remaining = %v", remaining)
	}
	if count, ban := v.recordVerifyFail(historyLive.gid, historyLive.uid, v.wallNow()); count != 3 || !ban {
		t.Errorf("live ban history after pruning = (%d, %v), want (3, true)", count, ban)
	}

	restored := newTestService(cfg)
	restored.vfailPath = v.vfailPath
	restored.loadVerifyFails()
	if _, ok := restored.vfail[dead]; ok {
		t.Error("fully expired record was serialized")
	}
}

func TestRecordVerifyFailPrunesExpiredRecordsBeforeInsertion(t *testing.T) {
	v := newTestService(&settings.Config{VerifyRetrySeconds: 30})
	dead := pkey{-100, 1}
	v.vfail[dead] = &vfailRec{count: 1, last: time.Now().Add(-verifyFailWindow - time.Minute)}

	v.recordVerifyFail(-100, 2, v.wallNow())

	if _, ok := v.vfail[dead]; ok {
		t.Error("insertion retained a record whose strike and cooldown windows had expired")
	}
}

func TestVerifyFailCapacityEvictsOldestWithoutClearingLiveState(t *testing.T) {
	v := newTestService(&settings.Config{VerifyMaxFails: 3, VerifyRetrySeconds: 180})
	now := time.Now()
	protected := pkey{-100, 1}
	oldest := pkey{-100, 2}
	for uid := int64(1); uid <= vfailMax; uid++ {
		v.vfail[pkey{-100, uid}] = &vfailRec{count: 1, last: now.Add(-time.Hour)}
	}
	v.vfail[protected] = &vfailRec{count: 2, last: now}
	v.vfail[oldest].last = now.Add(-verifyFailWindow + time.Minute)

	v.recordVerifyFail(-100, vfailMax+1, v.wallNow())

	if len(v.vfail) != vfailMax {
		t.Fatalf("ledger size after capacity insertion = %d, want %d", len(v.vfail), vfailMax)
	}
	if _, ok := v.vfail[oldest]; ok {
		t.Error("oldest remaining strike record was not evicted")
	}
	if count, ban := v.recordVerifyFail(protected.gid, protected.uid, v.wallNow()); count != 3 || !ban {
		t.Errorf("live cooldown and ban history were lost at capacity: (%d, %v)", count, ban)
	}
}

func TestClaimPendingNonce(t *testing.T) {
	v := newTestService(&settings.Config{})
	key := pkey{-100, 42}
	p := &pending{nonce: "NEW"}
	v.pend[key] = p
	if _, ok, _ := v.claimPendingNonce(key.gid, key.uid, "OLD"); ok {
		t.Fatal("stale nonce claimed the replacement pending")
	}
	if p.done {
		t.Fatal("stale nonce marked the replacement done")
	}
	got, ok, _ := v.claimPendingNonce(key.gid, key.uid, "NEW")
	if !ok || got != p || !p.done {
		t.Fatalf("matching nonce claim = (%p, %v), pending done=%v", got, ok, p.done)
	}
}

func TestRecordVerifyFailCountsAtClaimTime(t *testing.T) {
	const gid, uid = int64(-100), int64(42)
	v := newTestService(&settings.Config{VerifyMaxFails: 2})
	failedAt := time.Unix(2_000_000_000, 0)
	key := pkey{gid, uid}
	v.vfail[key] = &vfailRec{count: 1, last: failedAt.Add(-verifyFailWindow + 10*time.Second)}

	recorder, ok := any(v).(interface {
		recordVerifyFail(int64, int64, time.Time) (int, bool)
	})
	if !ok {
		t.Fatal("verification failure recording is not bound to the claim timestamp")
	}
	if count, ban := recorder.recordVerifyFail(gid, uid, failedAt); count != 2 || !ban {
		t.Fatalf("failure recorded at claim time = (%d, %v), want (2, true)", count, ban)
	}
}

func TestSettlementForUnmanagedGroupDoesNotChargeApplicantStrike(t *testing.T) {
	const (
		groupID      = int64(-1009000000943)
		otherGroupID = int64(-1009000001943)
		userID       = int64(943)
	)
	for _, test := range []struct {
		name        string
		managed     bool
		wantStrikes int
	}{
		{name: "managed group records the failed verification", managed: true, wantStrikes: 1},
		{name: "unmanaged group leaves the applicant uncharged", managed: false, wantStrikes: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := newTestService(&settings.Config{
				Groups:         []settings.GroupConfig{{ID: groupID}},
				GroupIDs:       []int64{groupID},
				VerifyMaxFails: 2,
			})
			key := pkey{gid: groupID, uid: userID}
			p := &pending{nonce: "failed", failedAt: time.Unix(2_000_000_000, 0), done: true}
			v.pend[key] = p
			v.markTerminalLocked(key, p)
			if !test.managed {
				unmanaged := newTestService(&settings.Config{
					Groups:   []settings.GroupConfig{{ID: otherGroupID}},
					GroupIDs: []int64{otherGroupID},
				})
				v.settings = unmanaged.settings
			}
			if p.removed {
				t.Fatal("fixture used the separately covered removed-pending guard")
			}

			bot := newFakeVerifyBot()
			outcome, banned := v.finishDecline(
				context.Background(), bot, groupID, userID, p, wrongAnswerReason,
			)
			if outcome != declineConfirmed || banned || bot.declines != 1 {
				t.Fatalf("settlement outcome/banned/declines = %v/%v/%d, want confirmed/false/1",
					outcome, banned, bot.declines)
			}
			v.mu.Lock()
			strikes := 0
			if record := v.vfail[key]; record != nil {
				strikes = record.count
			}
			v.mu.Unlock()
			if strikes != test.wantStrikes {
				t.Fatalf("applicant strikes after settlement for managed=%v = %d, want %d; "+
					"a chat the bot no longer manages must not start a cooldown",
					test.managed, strikes, test.wantStrikes)
			}
		})
	}
}

func TestVerificationStrikesRestartAfterWindowWhileRetryCooldownRemains(t *testing.T) {
	const (
		groupID = int64(-1009000000944)
		userID  = int64(944)
	)
	now := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	v := newTestService(&settings.Config{
		Groups:             []settings.GroupConfig{{ID: groupID}},
		GroupIDs:           []int64{groupID},
		VerifyMaxFails:     2,
		VerifyRetrySeconds: int((8 * time.Hour) / time.Second),
	})
	v.timeNow = func() time.Time { return now }

	for _, test := range []struct {
		name      string
		age       time.Duration
		wantCount int
		wantBan   bool
	}{
		{name: "inside strike window accumulates", age: verifyFailWindow - time.Hour, wantCount: 2, wantBan: true},
		{name: "outside strike window restarts", age: verifyFailWindow + time.Hour, wantCount: 1, wantBan: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			key := pkey{gid: groupID, uid: userID}
			v.mu.Lock()
			v.vfail[key] = &vfailRec{count: 1, last: now.Add(-test.age)}
			v.mu.Unlock()
			if remaining := v.verifyCooldownRemaining(groupID, userID); remaining <= 0 {
				t.Fatalf("fixture cooldown = %s, want the old strike retained past pruning", remaining)
			}

			count, ban := v.recordVerifyFail(groupID, userID, now)
			if count != test.wantCount || ban != test.wantBan {
				t.Fatalf("failure after %s = count %d, ban %v, want %d/%v; "+
					"an old strike must not turn one fresh mistake into a ban",
					test.age, count, ban, test.wantCount, test.wantBan)
			}
		})
	}
}
