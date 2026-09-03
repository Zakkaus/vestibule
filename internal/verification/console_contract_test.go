package verification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	consoleContractGroupID      int64 = -1009000001
	consoleContractOtherGroupID int64 = -1009000002
	consoleContractUserID       int64 = 420001
)

func TestConsoleSettlementRefAcceptsTerminalTargetsAndDeclineReasons(t *testing.T) {
	cases := []struct {
		name   string
		target ChallengeState
		reason string
	}{
		{name: "approved", target: ChallengeApproved},
		{name: "banned", target: ChallengeBanned},
		{name: "declined without a reason", target: ChallengeDeclined},
		{name: "declined for wrong answer", target: ChallengeDeclined, reason: "wrong_answer"},
		{name: "declined by rejection", target: ChallengeDeclined, reason: "rejected"},
		{name: "declined for unmet external condition", target: ChallengeDeclined, reason: "external_unmet"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settlement := consoleContractSettlement(consoleContractGroupID, consoleContractUserID, "valid", tc.target, tc.reason)
			ref, err := consoleSettlementRef(settlement)
			if err != nil {
				t.Fatalf("valid %s settlement was refused: %v", tc.name, err)
			}
			if ref.GroupID != settlement.GroupID || ref.UserID != consoleContractUserID || ref.Nonce != "valid" {
				t.Fatalf("valid %s settlement parsed as %+v", tc.name, ref)
			}
		})
	}

	for _, nonce := range []string{"n", strings.Repeat("n", 64)} {
		settlement := consoleContractSettlement(
			consoleContractGroupID, consoleContractUserID, nonce, ChallengeApproved, "")
		ref, err := consoleSettlementRef(settlement)
		if err != nil {
			t.Fatalf("valid %d-character nonce was refused: %v", len(nonce), err)
		}
		if ref.Nonce != nonce {
			t.Fatalf("valid %d-character nonce parsed as %q", len(nonce), ref.Nonce)
		}
	}
}

func TestConsoleSettlementRefRejectsInvalidInput(t *testing.T) {
	valid := consoleContractSettlement(consoleContractGroupID, consoleContractUserID, "valid", ChallengeApproved, "")
	cases := []struct {
		name   string
		harm   string
		mutate func(*ConsoleSettlement)
	}{
		{
			name: "zero group ID", harm: "zero GroupID",
			mutate: func(settlement *ConsoleSettlement) {
				settlement.GroupID = 0
				settlement.ID = challengeConsoleID(0, consoleContractUserID, "valid")
			},
		},
		{
			name: "non-positive actor ID", harm: "non-positive ActorID",
			mutate: func(settlement *ConsoleSettlement) { settlement.ActorID = 0 },
		},
		{
			name: "expected is not pending", harm: "Expected state other than pending",
			mutate: func(settlement *ConsoleSettlement) { settlement.Expected = ChallengeApproved },
		},
		{
			name: "target is not terminal", harm: "target outside approved, declined, and banned",
			mutate: func(settlement *ConsoleSettlement) { settlement.Target = ChallengePending },
		},
		{
			name: "reason is not allowed", harm: "reason outside the allowed set",
			mutate: func(settlement *ConsoleSettlement) { settlement.Reason = "invented" },
		},
		{
			name: "ID has fewer than three parts", harm: "ID not exactly three parts",
			mutate: func(settlement *ConsoleSettlement) { settlement.ID = "-1009000001:420001" },
		},
		{
			name: "ID has more than three parts", harm: "ID not exactly three parts",
			mutate: func(settlement *ConsoleSettlement) {
				settlement.ID = "-1009000001:420001:valid:extra"
			},
		},
		{
			name: "nonce is empty", harm: "empty nonce",
			mutate: func(settlement *ConsoleSettlement) { settlement.ID = "-1009000001:420001:" },
		},
		{
			name: "nonce exceeds 64 characters", harm: "nonce longer than 64 characters",
			mutate: func(settlement *ConsoleSettlement) {
				settlement.ID = challengeConsoleID(consoleContractGroupID, consoleContractUserID, string(make([]byte, 65)))
			},
		},
		{
			name: "ID names another group", harm: "embedded group differing from authorized GroupID",
			mutate: func(settlement *ConsoleSettlement) {
				settlement.ID = challengeConsoleID(consoleContractOtherGroupID, consoleContractUserID, "valid")
			},
		},
		{
			name: "ID names a non-positive user", harm: "non-positive embedded user",
			mutate: func(settlement *ConsoleSettlement) {
				settlement.ID = challengeConsoleID(consoleContractGroupID, 0, "valid")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settlement := valid
			tc.mutate(&settlement)
			if err := consoleSettlementRefError(t, settlement, tc.harm); !errors.Is(err, ErrConsoleSettlementInvalid) {
				t.Fatalf("%s returned %v, want %v", tc.harm, err, ErrConsoleSettlementInvalid)
			}
		})
	}
}

func TestConsoleQueueMemoryPreventsCrossGroupDisclosure(t *testing.T) {
	service := consoleContractService()
	live := consoleContractPending("live")
	service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID}] = live
	service.pend[pkey{gid: consoleContractOtherGroupID, uid: consoleContractUserID + 1}] = consoleContractPending("other-group")

	entries := service.consoleQueueMemory(consoleContractGroupID)
	if len(entries) != 1 || entries[0].ID != challengeConsoleID(consoleContractGroupID, consoleContractUserID, live.nonce) {
		t.Fatalf("cross-group disclosure: requested group queue contains %d challenges, want only its live applicant", len(entries))
	}
}

func TestConsoleQueueMemoryPreventsSettledApplicantsReappearing(t *testing.T) {
	service := consoleContractService()
	live := consoleContractPending("live")
	settled := consoleContractPending("settled")
	settled.done = true
	service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID}] = live
	service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID + 1}] = settled

	entries := service.consoleQueueMemory(consoleContractGroupID)
	if len(entries) != 1 || entries[0].ID != challengeConsoleID(consoleContractGroupID, consoleContractUserID, live.nonce) {
		t.Fatalf("settled applicants reappearing: group queue contains %d challenges, want only its live applicant", len(entries))
	}
}

func TestSettleConsoleRejectsCreator(t *testing.T) {
	t.Run("creator remains protected", func(t *testing.T) {
		service := consoleContractService()
		gateway := &fakeVerifyBot{member: &ChatMemberOwner{Status: MemberStatusCreator}}
		service.gateway = gateway
		item := consoleContractPending("creator")
		service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID}] = item

		_, err := service.SettleConsole(context.Background(), consoleContractSettlement(
			consoleContractGroupID, consoleContractUserID, item.nonce, ChallengeApproved, ""))
		if !errors.Is(err, ErrConsoleTargetProtected) {
			t.Fatalf("group creator was settled instead of protected: settlement error = %v, want %v", err, ErrConsoleTargetProtected)
		}
		if item.done || consoleContractActions(gateway) != 0 {
			t.Fatalf("group creator protection caused settlement/action: done=%t actions=%d", item.done, consoleContractActions(gateway))
		}
	})

	t.Run("ordinary member is settled", func(t *testing.T) {
		service := consoleContractService()
		gateway := &fakeVerifyBot{member: &ChatMemberLeft{Status: MemberStatusLeft}}
		service.gateway = gateway
		item := consoleContractPending("ordinary")
		service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID}] = item

		entry, err := service.SettleConsole(context.Background(), consoleContractSettlement(
			consoleContractGroupID, consoleContractUserID, item.nonce, ChallengeApproved, ""))
		if err != nil {
			t.Fatalf("ordinary member settlement failed: %v", err)
		}
		if !item.done || entry.State != ChallengeApproved || gateway.approves != 1 {
			t.Fatalf("ordinary member settlement did not approve exactly once: done=%t state=%q approvals=%d",
				item.done, entry.State, gateway.approves)
		}
	})
}

func TestSettleConsoleRefusesUnavailableTargetMembership(t *testing.T) {
	cases := []struct {
		name        string
		gateway     *fakeVerifyBot
		install     bool
		wantSuccess bool
	}{
		{name: "nil gateway"},
		{name: "membership error", gateway: &fakeVerifyBot{memberErr: errors.New("membership unavailable")}, install: true},
		{name: "nil member", gateway: &fakeVerifyBot{}, install: true},
		{
			name: "ordinary member", gateway: &fakeVerifyBot{member: &ChatMemberLeft{Status: MemberStatusLeft}},
			install: true, wantSuccess: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := consoleContractService()
			if tc.install {
				service.gateway = tc.gateway
			}
			item := consoleContractPending("membership")
			service.pend[pkey{gid: consoleContractGroupID, uid: consoleContractUserID}] = item

			entry, err := settleConsoleWithoutPanic(t, service, consoleContractSettlement(
				consoleContractGroupID, consoleContractUserID, item.nonce, ChallengeApproved, ""))
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("ordinary member settlement failed: %v", err)
				}
				if !item.done || entry.State != ChallengeApproved || tc.gateway.approves != 1 {
					t.Fatalf("ordinary member settlement did not approve exactly once: done=%t state=%q approvals=%d",
						item.done, entry.State, tc.gateway.approves)
				}
				return
			}
			if !errors.Is(err, ErrConsoleTargetUnavailable) {
				t.Fatalf("unavailable target membership was not refused: settlement error = %v, want %v", err, ErrConsoleTargetUnavailable)
			}
			if item.done || consoleContractActions(tc.gateway) != 0 {
				t.Fatalf("unavailable target membership caused settlement/action: done=%t actions=%d",
					item.done, consoleContractActions(tc.gateway))
			}
		})
	}
}

func consoleContractService() *Service {
	return newTestService(&settings.Config{
		Groups:   []settings.GroupConfig{{ID: consoleContractGroupID}},
		GroupIDs: []int64{consoleContractGroupID},
	})
}

func consoleContractSettlement(groupID, userID int64, nonce string, target ChallengeState, reason string) ConsoleSettlement {
	return ConsoleSettlement{
		ID:       challengeConsoleID(groupID, userID, nonce),
		GroupID:  groupID,
		ActorID:  900001,
		Expected: ChallengePending,
		Target:   target,
		Reason:   reason,
	}
}

func consoleContractPending(nonce string) *pending {
	return &pending{nonce: nonce, deadline: time.Unix(2_000_000_000, 0)}
}

func consoleSettlementRefError(t *testing.T, settlement ConsoleSettlement, harm string) (err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%s fail-open/panic harm: console settlement parsing panicked: %v", harm, recovered)
		}
	}()
	_, err = consoleSettlementRef(settlement)
	return err
}

func settleConsoleWithoutPanic(t *testing.T, service *Service, settlement ConsoleSettlement) (entry ConsoleQueueEntry, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("unavailable target membership fail-open/panic harm: settlement panicked: %v", recovered)
		}
	}()
	return service.SettleConsole(context.Background(), settlement)
}

func consoleContractActions(gateway *fakeVerifyBot) int {
	if gateway == nil {
		return 0
	}
	return gateway.approves + gateway.declines + gateway.bans
}
