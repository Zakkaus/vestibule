package verification

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type actionTestStore struct {
	testVerificationStore
	mu           sync.Mutex
	actions      map[string]testAction
	pending      []PendingRecord
	claimStarted chan struct{}
	releaseClaim <-chan struct{}
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
	if s.claimStarted != nil {
		close(s.claimStarted)
	}
	if s.releaseClaim != nil {
		<-s.releaseClaim
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
	v := newTestService(&settings.Config{})
	v.timeNow = func() time.Time { return now }
	state := &actionTestStore{}
	v.stateStore = state
	v.statePath = "action-test"
	bot := &fakeVerifyBot{deleteErrAt: map[int]error{1: errors.New("temporary delete failure")}}
	v.gateway = bot
	p := &pending{nonce: "durable", deadline: now.Add(time.Hour), groupMsgID: 42, privateMsgID: 43}
	v.pend[pkey{gid, uid}] = p

	claimed, ok, _ := v.claimPendingNonce(gid, uid, p.nonce)
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
	v := newTestService(&settings.Config{})
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
	v := newTestService(&settings.Config{TimeoutSeconds: 240})
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
	v := newTestService(&settings.Config{TimeoutSeconds: 240})
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

func TestInFlightSettlementBlocksChallengeAfterPendingLeavesMap(t *testing.T) {
	const (
		groupID = int64(-1009000000945)
		userID  = int64(945)
	)
	v := newTestService(&settings.Config{
		Groups:         []settings.GroupConfig{{ID: groupID}},
		GroupIDs:       []int64{groupID},
		TimeoutSeconds: 240,
		VerifyMaxFails: 3,
	})
	t.Cleanup(v.stopForShutdown)
	key := pkey{gid: groupID, uid: userID}
	v.terminal[key] = &pending{nonce: "settling", done: true}
	bot := newFakeVerifyBot()

	replacement := &pending{nonce: "replacement"}
	if _, status, err := v.startPending(bot, groupID, userID, replacement); err != nil {
		t.Fatal(err)
	} else if status != pendingBlockedTerminal {
		t.Fatalf("re-application started while a decline or ban was still in flight; status = %v, want %v",
			status, pendingBlockedTerminal)
	}
	if _, exists := v.pend[key]; exists {
		t.Fatal("in-flight settlement let a replacement challenge enter the pending map")
	}

	delete(v.terminal, key)
	next := &pending{nonce: "next"}
	if _, status, err := v.startPending(bot, groupID, userID, next); err != nil {
		t.Fatal(err)
	} else if status != pendingStarted {
		t.Fatalf("challenge after settlement completion = %v, want %v", status, pendingStarted)
	}
	if v.pend[key] != next {
		t.Fatal("valid challenge after settlement completion was not installed")
	}
}

func TestDurableSettlementStopsRetryingAtAttemptLimit(t *testing.T) {
	const (
		groupID = int64(-1009000000946)
		userID  = int64(946)
	)
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&settings.Config{
		Groups:         []settings.GroupConfig{{ID: groupID}},
		GroupIDs:       []int64{groupID},
		VerifyMaxFails: 3,
	})
	v.timeNow = func() time.Time { return now }
	key := pkey{gid: groupID, uid: userID}
	p := &pending{nonce: "bounded", deadline: now.Add(time.Hour), failedAt: now}
	intent, err := v.newSettlementAction(key, p, ChallengeDeclined, wrongAnswerReason)
	if err != nil {
		t.Fatal(err)
	}
	state := &actionTestStore{actions: map[string]testAction{
		intent.ID: {
			PendingAction: PendingAction{
				ActionIntent: intent,
				ChallengeID:  "bounded-attempts",
				Attempts:     maxSettleFailures - 2,
			},
			state: "pending",
		},
	}}
	v.stateStore = state
	v.statePath = "bounded-action-test"
	bot := &fakeVerifyBot{declineErr: errors.New("not enough rights")}
	v.gateway = bot

	v.RunPendingActionsOnce(context.Background())
	state.mu.Lock()
	afterRetry := state.actions[intent.ID]
	state.mu.Unlock()
	if afterRetry.state != "pending" || afterRetry.Attempts != maxSettleFailures-1 {
		t.Fatalf("penultimate durable failure = state %q attempts %d, want pending/%d",
			afterRetry.state, afterRetry.Attempts, maxSettleFailures-1)
	}
	v.mu.Lock()
	_, pendingAfterRetry := v.pend[key]
	_, terminalAfterRetry := v.terminal[key]
	v.mu.Unlock()
	if !pendingAfterRetry || !terminalAfterRetry {
		t.Fatal("retryable durable failure did not retain the settlement for its final attempt")
	}

	now = time.Unix(afterRetry.NextTryAt, 0)
	v.RunPendingActionsOnce(context.Background())
	state.mu.Lock()
	afterLimit := state.actions[intent.ID]
	state.mu.Unlock()
	if afterLimit.state != "failed" {
		t.Fatalf("durable settlement after %d failures = state %q, want failed; "+
			"an impossible action must be handed back to an administrator instead of retrying forever",
			maxSettleFailures, afterLimit.state)
	}
	if bot.declines != 2 {
		t.Fatalf("durable decline attempts = %d, want 2 around the bounded-attempt threshold", bot.declines)
	}
	v.mu.Lock()
	_, pendingAfterLimit := v.pend[key]
	_, terminalAfterLimit := v.terminal[key]
	v.mu.Unlock()
	if pendingAfterLimit || terminalAfterLimit {
		t.Fatal("exhausted durable settlement still owns the applicant instead of releasing it for an administrator")
	}

	now = now.Add(10 * time.Minute)
	v.RunPendingActionsOnce(context.Background())
	if bot.declines != 2 {
		t.Fatalf("failed durable action retried again; decline calls = %d, want 2", bot.declines)
	}
}

func TestPendingActionPassStartsNoWorkAfterShutdown(t *testing.T) {
	const groupID = int64(-1009000000947)
	for _, test := range []struct {
		name        string
		shutdown    bool
		wantDeletes int
		wantState   string
	}{
		{name: "live service executes the action", wantDeletes: 1, wantState: "done"},
		{name: "shutting down service leaves the action untouched", shutdown: true, wantState: "pending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			state := &actionTestStore{actions: map[string]testAction{
				"shutdown-delete": {
					PendingAction: PendingAction{ActionIntent: ActionIntent{
						ID:        "shutdown-delete",
						Kind:      actionDeleteGroup,
						Payload:   fmt.Sprintf(`{"chat_id":%d,"message_id":42}`, groupID),
						NextTryAt: now.Unix(),
					}},
					state: "pending",
				},
			}}
			v := newTestService(&settings.Config{})
			v.timeNow = func() time.Time { return now }
			v.stateStore = state
			v.statePath = "shutdown-action-test"
			bot := newFakeVerifyBot()
			v.gateway = bot
			if test.shutdown {
				v.stopForShutdown()
			}

			v.RunPendingActionsOnce(context.Background())

			state.mu.Lock()
			actionState := state.actions["shutdown-delete"].state
			state.mu.Unlock()
			if bot.deletes != test.wantDeletes || actionState != test.wantState {
				t.Fatalf("shutdown=%v action deletes/state = %d/%q, want %d/%q; "+
					"shutdown must not start a new Telegram mutation",
					test.shutdown, bot.deletes, actionState, test.wantDeletes, test.wantState)
			}
		})
	}
}

// A settlement action claimed while the service is live is executed: the control the shutdown
// test below needs, so that its zero decline calls mean "the freeze stopped it" rather than
// "this arrangement never settles anything".
func TestAClaimedActionSettlesWhileTheServiceIsLive(t *testing.T) {
	v, _, bot := shutdownRaceService(t, nil, nil)
	v.RunPendingActionsOnce(context.Background())
	if bot.declines != 1 {
		t.Fatalf("live claimed action made %d decline calls, want 1", bot.declines)
	}
}

// An action already claimed when shutdown is declared must not go on to settle. The claim
// completes after the freeze, and installing its pending there would decide a verification
// against an applicant the process is no longer able to answer.
func TestActionClaimCompletingAfterShutdownStartsNoSettlement(t *testing.T) {
	claimStarted := make(chan struct{})
	releaseClaim := make(chan struct{})
	v, _, bot := shutdownRaceService(t, claimStarted, releaseClaim)
	done := make(chan struct{})
	go func() {
		v.RunPendingActionsOnce(context.Background())
		close(done)
	}()
	select {
	case <-claimStarted:
	case <-time.After(time.Second):
		t.Fatal("action claim did not reach the shutdown race point")
	}
	v.stopForShutdown()
	close(releaseClaim)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("action pass did not stop after shutdown")
	}

	if bot.declines != 0 {
		t.Fatalf("action claimed before shutdown made %d decline calls after the freeze, want 0", bot.declines)
	}
	key := pkey{gid: shutdownRaceGroup, uid: shutdownRaceUser}
	v.mu.Lock()
	_, installed := v.pend[key]
	_, terminal := v.terminal[key]
	v.mu.Unlock()
	if installed || terminal {
		t.Fatal("action claimed before shutdown installed new pending settlement state after the freeze")
	}
}

const (
	shutdownRaceGroup = int64(-1009000000948)
	shutdownRaceUser  = int64(948)
)

// One service holding one claimed settlement action. Passing channels makes the store block in
// the middle of its claim, which is where shutdown has to be declared for the race to be real.
func shutdownRaceService(
	t *testing.T,
	claimStarted chan struct{},
	releaseClaim <-chan struct{},
) (*Service, *actionTestStore, *fakeVerifyBot) {
	t.Helper()
	now := time.Unix(1_700_000_000, 0)
	v := newTestService(&settings.Config{
		Groups:   []settings.GroupConfig{{ID: shutdownRaceGroup}},
		GroupIDs: []int64{shutdownRaceGroup},
	})
	v.timeNow = func() time.Time { return now }
	intent, err := v.newSettlementAction(
		pkey{gid: shutdownRaceGroup, uid: shutdownRaceUser},
		&pending{nonce: "claimed-before-shutdown", deadline: now.Add(time.Hour)},
		ChallengeDeclined,
		wrongAnswerReason,
	)
	if err != nil {
		t.Fatal(err)
	}
	state := &actionTestStore{
		actions: map[string]testAction{
			intent.ID: {
				PendingAction: PendingAction{ActionIntent: intent, ChallengeID: "shutdown"},
				state:         "pending",
			},
		},
		claimStarted: claimStarted,
		releaseClaim: releaseClaim,
	}
	v.stateStore = state
	v.statePath = "action-claim-shutdown-test"
	bot := newFakeVerifyBot()
	v.gateway = bot
	return v, state, bot
}
