package verification

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
)

type actionTestStore struct {
	testVerificationStore
	mu      sync.Mutex
	actions map[string]testAction
	pending []PendingRecord
}
type testAction struct {
	PendingAction
	state      string
	claimOwner string
	claimUntil int64
}

func (s *actionTestStore) TransitionChallenge(_ string, transition ChallengeTransition) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.actions == nil {
		s.actions = make(map[string]testAction)
	}
	for _, intent := range transition.Actions {
		s.actions[intent.ID] = testAction{
			PendingAction: PendingAction{ActionIntent: intent, ChallengeID: "test"},
			state:         "pending", claimOwner: intent.ClaimOwner, claimUntil: intent.ClaimUntil,
		}
	}
	return true, nil
}

func (s *actionTestStore) ClaimExpired(_ string, now, claimUntil int64, limit int) ([]PendingRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	claimed := make([]PendingRecord, 0, limit)
	for i := range s.pending {
		record := &s.pending[i]
		if len(claimed) == limit || record.Deadline > now {
			continue
		}
		record.Epoch++
		record.Deadline = claimUntil
		claimed = append(claimed, *record)
	}
	return claimed, nil
}

func (s *actionTestStore) ClaimActions(_ string, owner string, now, claimUntil int64, limit int) ([]PendingAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var claimed []PendingAction
	for id, action := range s.actions {
		if len(claimed) == limit || action.state != "pending" || action.NextTryAt > now || action.claimUntil > now {
			continue
		}
		action.claimOwner, action.claimUntil = owner, claimUntil
		s.actions[id] = action
		claimed = append(claimed, action.PendingAction)
	}
	return claimed, nil
}

func (s *actionTestStore) CompleteAction(_ string, id, owner string, _ int64, followups []ActionIntent) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	action.state, action.claimOwner, action.claimUntil = "done", "", 0
	s.actions[id] = action
	for _, followup := range followups {
		s.actions[followup.ID] = testAction{PendingAction: PendingAction{ActionIntent: followup, ChallengeID: action.ChallengeID}, state: "pending"}
	}
	return true, nil
}

func (s *actionTestStore) RetryAction(_ string, id, owner string, attempts int, nextTryAt int64, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	action.Attempts, action.NextTryAt = attempts, nextTryAt
	action.claimOwner, action.claimUntil = "", 0
	s.actions[id] = action
	return true, nil
}

func (s *actionTestStore) FailAction(_ string, id, owner string, _ int64, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action, ok := s.actions[id]
	if !ok || action.state != "pending" || action.claimOwner != owner {
		return false, nil
	}
	action.state, action.claimOwner, action.claimUntil = "failed", "", 0
	s.actions[id] = action
	return true, nil
}

func TestDurableSettlementRetriesGroupDeletionWithoutDeletingDM(t *testing.T) {
	const gid, uid = int64(-100), int64(5)
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&config.Config{})
	v.timeNow = func() time.Time { return now }
	state := &actionTestStore{}
	v.stateStore = state
	v.statePath = "action-test"
	bot := &fakeVerifyBot{deleteErrAt: map[int]error{1: errors.New("temporary delete failure")}}
	v.gateway = bot
	p := &pending{nonce: "durable", deadline: now.Add(time.Hour), groupMsgID: 42, privateMsgID: 43}
	v.pend[pkey{gid, uid}] = p

	claimed, ok := v.claimPendingNonce(gid, uid, p.nonce)
	if !ok || claimed.actionID == "" {
		t.Fatal("terminal transition did not create a claimed durable action")
	}
	if outcome := v.executeApprove(context.Background(), bot, gid, uid, claimed); outcome != approveConfirmed {
		t.Fatalf("approval outcome = %v, want confirmed", outcome)
	}
	if bot.deletes != 0 {
		t.Fatalf("settlement deleted %d messages synchronously instead of queuing cleanup", bot.deletes)
	}

	v.RunPendingActionsOnce(context.Background())
	if bot.deletes != 1 || !reflect.DeepEqual(bot.deletedChats, []int64{gid}) || !reflect.DeepEqual(bot.deletedMessageIDs, []int{42}) {
		t.Fatalf("first durable delete = chats %v messages %v, want group challenge only", bot.deletedChats, bot.deletedMessageIDs)
	}
	now = now.Add(6 * time.Second)
	v.RunPendingActionsOnce(context.Background())
	if bot.deletes != 2 || !reflect.DeepEqual(bot.deletedChats, []int64{gid, gid}) || !reflect.DeepEqual(bot.deletedMessageIDs, []int{42, 42}) {
		t.Fatalf("retried delete = chats %v messages %v, want only retryable group challenge", bot.deletedChats, bot.deletedMessageIDs)
	}
	state.mu.Lock()
	deferred := state.actions[claimed.actionID+":group-delete"]
	state.mu.Unlock()
	if deferred.state != "done" || deferred.Attempts != 1 {
		t.Fatalf("group delete action = %#v, want done after one recorded retry", deferred)
	}
}

func TestDurableDeleteTreatsMissingGroupMessageAsComplete(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&config.Config{})
	v.timeNow = func() time.Time { return now }
	state := &actionTestStore{actions: map[string]testAction{
		"gone": {
			PendingAction: PendingAction{ActionIntent: ActionIntent{
				ID: "gone", Kind: actionDeleteGroup, Payload: `{"chat_id":-100,"message_id":42}`, NextTryAt: now.Unix(),
			}},
			state: "pending",
		},
	}}
	v.stateStore = state
	v.statePath = "action-test"
	bot := &fakeVerifyBot{deleteErrAt: map[int]error{
		1: &GatewayError{Cause: errors.New("message not found"), Kinds: FailureMessageGone},
	}}
	v.gateway = bot

	v.RunPendingActionsOnce(context.Background())

	state.mu.Lock()
	action := state.actions["gone"]
	state.mu.Unlock()
	if action.state != "done" || action.Attempts != 0 {
		t.Fatalf("missing group message action = %#v, want done without retry", action)
	}
}

// A shutdown must not settle somebody on its way out. The previous generation
// hung a time.AfterFunc on each pending verification and declined people during
// a graceful stop, which is the failure this scanner replaces; replacing the
// mechanism does not by itself preserve the property, so it is asserted here.
func TestScanExpiredDeclinesNobodyOnceShutdownStarted(t *testing.T) {
	const gid, uid = int64(-100), int64(7)
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&config.Config{TimeoutSeconds: 240})
	v.timeNow = func() time.Time { return now }
	p := &pending{nonce: "due", deadline: now, groupMsgID: 52, privateMsgID: 53}
	state := &actionTestStore{pending: []PendingRecord{pendingRecord(pkey{gid, uid}, p)}}
	v.stateStore = state
	v.statePath = "scan-shutdown-test"
	bot := &fakeVerifyBot{}
	v.gateway = bot
	v.pend[pkey{gid, uid}] = p

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	v.ScanExpired(ctx)

	if bot.declines != 0 {
		t.Fatalf("declines during shutdown = %d, want 0", bot.declines)
	}
	if _, exists := v.pend[pkey{gid, uid}]; !exists {
		t.Fatal("shutdown dropped a pending verification instead of leaving it for the next run")
	}
}

func TestScanExpiredClaimsDueChallengeWithoutLocalTimer(t *testing.T) {
	const gid, uid = int64(-100), int64(7)
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&config.Config{TimeoutSeconds: 240})
	v.timeNow = func() time.Time { return now }
	p := &pending{nonce: "due", deadline: now, groupMsgID: 52, privateMsgID: 53}
	state := &actionTestStore{pending: []PendingRecord{pendingRecord(pkey{gid, uid}, p)}}
	v.stateStore = state
	v.statePath = "scan-test"
	bot := &fakeVerifyBot{}
	v.gateway = bot
	v.pend[pkey{gid, uid}] = p

	v.ScanExpired(context.Background())

	if bot.declines != 1 {
		t.Fatalf("durable scanner decline calls = %d, want 1", bot.declines)
	}
	if _, exists := v.pend[pkey{gid, uid}]; exists {
		t.Fatal("scanner left settled pending in memory")
	}
	state.mu.Lock()
	action := state.actions["settle:-100:7:due:expired"]
	state.mu.Unlock()
	if action.state != "done" {
		t.Fatalf("scanner settlement action = %#v, want done", action)
	}
}
