package verification

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type conditionalActionStore struct {
	*actionTestStore
	current PendingRecord
}

func (s *conditionalActionStore) TransitionChallenge(path string, transition ChallengeTransition) (bool, error) {
	if !testPendingMatches(s.current, transition.Expected) {
		return false, nil
	}
	return s.actionTestStore.TransitionChallenge(path, transition)
}

func TestExpiryClaimRequiresCurrentNonceAndEpoch(t *testing.T) {
	const (
		groupID = int64(-1009000000949)
		userID  = int64(949)
	)
	for _, test := range []struct {
		name  string
		nonce string
		epoch uint64
	}{
		{name: "stale nonce", nonce: "superseded", epoch: 7},
		{name: "stale epoch", nonce: "current", epoch: 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := newTestService(&settings.Config{})
			key := pkey{gid: groupID, uid: userID}
			current := &pending{nonce: "current", epoch: 7, deadline: time.Now().Add(time.Hour)}
			v.pend[key] = current

			claimed, ok, err := v.claimPendingExpiry(groupID, userID, test.nonce, test.epoch, "timeout")
			if err != nil {
				t.Fatal(err)
			}
			if ok || claimed != nil || current.done {
				t.Fatalf("stale expiry identity nonce=%q epoch=%d claimed the replacement: pending=%p ok=%v done=%v",
					test.nonce, test.epoch, claimed, ok, current.done)
			}
			if inFlight := v.terminal[key]; inFlight != nil {
				t.Fatal("stale expiry identity installed a terminal settlement for the replacement")
			}

			claimed, ok, err = v.claimPendingExpiry(groupID, userID, current.nonce, current.epoch, "timeout")
			if err != nil {
				t.Fatal(err)
			}
			if !ok || claimed != current || !current.done || v.terminal[key] != current {
				t.Fatalf("current expiry identity did not claim the live verification: pending=%p ok=%v done=%v terminal=%p",
					claimed, ok, current.done, v.terminal[key])
			}
		})
	}
}

func TestDurableExpiryIdentitySettlesOnlyCurrentPending(t *testing.T) {
	const (
		groupID = int64(-1009000000950)
		userID  = int64(950)
	)
	now := time.Unix(1_700_000_000, 0)
	for _, test := range []struct {
		name         string
		recordNonce  string
		wantClaim    bool
		wantDeclines int
	}{
		{name: "superseded row is harmless", recordNonce: "superseded"},
		{name: "current row settles", recordNonce: "current", wantClaim: true, wantDeclines: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := newTestService(&settings.Config{
				Groups:   []settings.GroupConfig{{ID: groupID}},
				GroupIDs: []int64{groupID},
			})
			v.timeNow = func() time.Time { return now }
			key := pkey{gid: groupID, uid: userID}
			live := &pending{
				nonce: test.recordNonce,
				epoch: 7,
			}
			if !test.wantClaim {
				live.nonce = "current"
			}
			live.deadline = now.Add(time.Hour)
			live.challengeDelivered = true
			currentRecord := pendingRecord(key, live)
			state := &conditionalActionStore{
				actionTestStore: &actionTestStore{},
				current:         currentRecord,
			}
			v.stateStore = state
			v.statePath = "durable-identity-test"
			bot := newFakeVerifyBot()
			v.gateway = bot
			v.pend[key] = live

			claimedRecord := currentRecord
			claimedRecord.Nonce = test.recordNonce
			claimedRecord.Deadline = now.Unix()
			if !v.installExpiryClaim(claimedRecord) {
				t.Fatal("live service refused to install a claimed expiry row")
			}
			claimed, ok, err := v.claimPendingExpiry(
				groupID, userID, claimedRecord.Nonce, claimedRecord.Epoch, "timeout",
			)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				_, _ = v.finishDecline(context.Background(), bot, groupID, userID, claimed, "timeout")
			}
			if bot.declines != test.wantDeclines {
				t.Fatalf("durable row nonce=%q made %d decline calls, want %d; "+
					"superseded work must not remove the applicant answering a replacement",
					test.recordNonce, bot.declines, test.wantDeclines)
			}
			if ok != test.wantClaim {
				t.Fatalf("durable row nonce=%q claim = %v, want %v", test.recordNonce, ok, test.wantClaim)
			}
			if !test.wantClaim {
				v.mu.Lock()
				_, struck := v.vfail[key]
				v.mu.Unlock()
				if struck {
					t.Fatal("superseded durable expiry charged the replacement applicant a strike")
				}
			}
		})
	}
}

func TestDurableActionIdentityDoesNotClaimReplacement(t *testing.T) {
	const (
		groupID = int64(-1009000000951)
		userID  = int64(951)
	)
	for _, test := range []struct {
		name        string
		recordNonce string
		wantCurrent bool
	}{
		{name: "superseded action leaves replacement unclaimed", recordNonce: "superseded"},
		{name: "current action claims current pending", recordNonce: "current", wantCurrent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			v := newTestService(&settings.Config{})
			v.statePath = "durable-action-identity-test"
			key := pkey{gid: groupID, uid: userID}
			current := &pending{nonce: "current", epoch: 9, deadline: time.Now().Add(time.Hour)}
			v.pend[key] = current
			record := pendingRecord(key, current)
			record.Nonce = test.recordNonce
			action := PendingAction{ActionIntent: ActionIntent{ID: "identity-action"}}

			installed := v.installActionPending(
				settlementActionPayload{Record: record, State: ChallengeDeclined, Reason: wrongAnswerReason},
				action,
				"identity-worker",
			)
			if test.wantCurrent {
				if installed != current || !current.done || v.terminal[key] != current {
					t.Fatalf("current durable action did not claim current pending: installed=%p current=%p done=%v terminal=%p",
						installed, current, current.done, v.terminal[key])
				}
				return
			}
			if installed == current || current.done || current.actionID != "" || v.terminal[key] == current {
				t.Fatalf("superseded durable action claimed the replacement pending: installed=%p current=%p done=%v action=%q",
					installed, current, current.done, current.actionID)
			}
		})
	}
}
