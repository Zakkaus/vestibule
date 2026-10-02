package verification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	v "github.com/Zakkaus/vestibule/internal/verification"
)

func TestStopRemovalRetriesUndoBan(t *testing.T) {
	f := newChaosFixture(t)
	for _, step := range []string{"member", "fail/unrestrict/permanent", "fail/unban/transient", "stop"} {
		f.step(step)
	}
	pair := chaosPair{chaosGroups[0], chaosUsers[0]}
	if !f.bot.members[pair].banned {
		t.Fatal("witness did not leave the applicant banned after the failed unban")
	}
	var queued int
	chaosRequire(t, f.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM pending_action WHERE kind='undo_ban' AND state='pending'").Scan(&queued))
	if queued != 1 {
		t.Fatalf("applicant remains banned without durable undo: queued=%d", queued)
	}
	f.step("restart")
	f.step("due")
	if f.bot.members[pair].banned {
		t.Fatal("restart did not undo the cancellation removal's ban")
	}
}

func TestStopDuringQuestionSendRetainsDeletion(t *testing.T) {
	f := newChaosFixture(t)
	var question [2]int64
	f.bot.afterGroupSend = func(group int64, message int) {
		question = [2]int64{group, int64(message)}
		f.step("stop")
		f.step("fail/delete/transient")
	}
	f.step("join")
	if !f.bot.questions[question] || len(f.pending()) != 0 {
		t.Fatalf("witness did not cancel an in-flight question: questions=%v pending=%v", f.bot.questions, f.pending())
	}
	f.step("restart")
	f.step("advance/6")
	f.step("due")
	if f.bot.questions[question] {
		t.Fatalf("question %v survives cancellation and retry drain", question)
	}
}

type cleanupPersistenceFaultStore struct {
	v.Store
	unavailable bool
}

func (s *cleanupPersistenceFaultStore) EnqueueActions(namespace string, expected v.PendingRef, intents []v.ActionIntent) ([]v.PendingAction, error) {
	if s.unavailable {
		return nil, errors.New("cleanup database unavailable")
	}
	return s.Store.EnqueueActions(namespace, expected, intents)
}

func TestExternalApprovalRetriesCleanupPersistence(t *testing.T) {
	f := newChaosFixture(t)
	store := &cleanupPersistenceFaultStore{Store: &chaosClaimStore{Store: f.store, fixture: f}}
	var err error
	f.service, err = v.InterleavingService(f.groups, f.bot, store, f.cfg, f.directory, func() time.Time { return f.now })
	chaosRequire(t, err)
	f.step("join")
	record := f.pending()[0]
	pair := chaosPair{record.GroupID, record.UserID}
	f.bot.members[pair] = chaosMember{present: true}
	f.bot.admitted[pair] = f.now
	store.unavailable = true
	f.step("fail/approve/request-gone")
	f.step("right")
	var state string
	chaosRequire(t, f.db.QueryRow(context.Background(), "SELECT state FROM pending_action WHERE challenge_id=$1 AND kind='settle_approve'", chaosID(record)).Scan(&state))
	if state != "pending" {
		t.Fatalf("cleanup persistence failure stranded question: settlement action=%s", state)
	}
	store.unavailable = false
	f.step("restart")
	f.step("advance/31")
	f.step("fail/approve/request-gone")
	f.step("due")
	if len(f.bot.questions) != 0 || f.actionCount(false) != 0 {
		t.Fatalf("cleanup retry did not drain: questions=%v actions=%d", f.bot.questions, f.actionCount(false))
	}
}

func TestRecoveryRenewsMemberHold(t *testing.T) {
	f := newChaosFixture(t)
	f.step("member")
	old := f.pending()[0]
	f.step("advance/301")
	f.step("restart")
	f.step("advance/6")
	recovered := f.pending()[0]
	pair := chaosPair{old.GroupID, old.UserID}
	member := f.bot.members[pair]
	if recovered.Deadline <= f.now.Unix() || f.now.Unix() <= old.HoldUntil {
		t.Fatalf("witness is not inside recovery after old hold expiry: old=%+v recovered=%+v now=%d", old, recovered, f.now.Unix())
	}
	if member.heldUntil < recovered.Deadline || recovered.HoldUntil != member.heldUntil {
		t.Fatalf("unverified member can speak before recovery settles: actual hold=%d stored hold=%d recovery deadline=%d", member.heldUntil, recovered.HoldUntil, recovered.Deadline)
	}
}
