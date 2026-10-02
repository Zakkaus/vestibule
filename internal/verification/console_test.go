package verification

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/store"
)

func TestSettleConsoleRejectsTimeoutSettledChallengeWithoutTelegramAction(t *testing.T) {
	const groupID, userID = int64(-100), int64(42)
	service := newTestService(&settings.Config{Groups: []settings.GroupConfig{{ID: groupID}}, GroupIDs: []int64{groupID}})
	bot := &fakeVerifyBot{member: &ChatMemberLeft{Status: MemberStatusLeft}}
	service.gateway = bot
	service.statePath = filepath.Join(t.TempDir(), "pending.json")
	pending := &pending{nonce: "expired", deadline: time.Now().Add(time.Hour)}
	key := pkey{gid: groupID, uid: userID}
	service.pend[key] = pending
	record := pendingRecord(key, pending)
	if err := store.Write(service.statePath, []PendingRecord{record}); err != nil {
		t.Fatal(err)
	}
	if err := markChallengeExpired(service, record); err != nil {
		t.Fatal(err)
	}
	_, err := service.SettleConsole(context.Background(), ConsoleSettlement{
		ID: challengeConsoleID(groupID, userID, pending.nonce), GroupID: groupID, ActorID: 9,
		Expected: ChallengePending, Target: ChallengeApproved,
	})
	if !errors.Is(err, ErrConsoleChallengeConflict) {
		t.Fatalf("settlement error = %v, want %v", err, ErrConsoleChallengeConflict)
	}
	actions := bot.approves + bot.declines + bot.bans
	if actions != 0 {
		t.Fatalf("Telegram actions = %d, want 0", actions)
	}
	if len(bot.memberRequests) != 0 {
		t.Fatalf("GetChatMember calls = %d, want 0", len(bot.memberRequests))
	}
	t.Logf("timeout-settled challenge -> conflict; Telegram actions=%d; GetChatMember calls=%d", actions, len(bot.memberRequests))
}

func markChallengeExpired(service *Service, record PendingRecord) error {
	changed, err := service.stateStore.TransitionChallenge(service.statePath, ChallengeTransition{
		Expected: PendingRef{GroupID: record.GroupID, UserID: record.UserID, Nonce: record.Nonce, Epoch: record.Epoch},
		From:     ChallengePending, To: ChallengeExpired,
	})
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("test timeout transition was not applied")
	}
	return nil
}

func TestSettleConsoleReportsApprovalPendingRetry(t *testing.T) {
	service := consoleContractService()
	t.Cleanup(service.stopForShutdown)
	gateway := &fakeVerifyBot{
		member: &ChatMemberLeft{Status: MemberStatusLeft}, approveErr: errors.New("temporary approval failure"),
	}
	service.gateway = gateway
	service.statePath = filepath.Join(t.TempDir(), "pending.json")
	state := &actionTestStore{}
	service.stateStore = state
	item := consoleContractPending("retry")
	key := pkey{gid: consoleContractGroupID, uid: consoleContractUserID}
	service.pend[key] = item
	if err := store.Write(service.statePath, []PendingRecord{pendingRecord(key, item)}); err != nil {
		t.Fatal(err)
	}

	entry, err := service.SettleConsole(context.Background(), consoleContractSettlement(
		key.gid, key.uid, item.nonce, ChallengeApproved, ""))
	if err != nil {
		t.Fatal(err)
	}
	if entry.State != "approval_pending_retry" {
		t.Fatalf("approval response state=%q, want approval_pending_retry", entry.State)
	}
	state.mu.Lock()
	action := state.actions[item.actionID]
	state.mu.Unlock()
	if action.state != "pending" || action.Attempts != 1 || gateway.approves != 1 {
		t.Fatalf("approval retry action=%+v calls=%d", action, gateway.approves)
	}
}
