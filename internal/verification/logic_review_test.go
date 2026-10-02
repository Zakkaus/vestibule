package verification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func logicRequestGone() error {
	return &verification.GatewayError{Cause: errors.New("HIDE_REQUESTER_MISSING"), Kinds: verification.FailureJoinRequestGone}
}

func TestReviewApproveGoneDeletesGroupQuestion(t *testing.T) {
	db, bot, service := logicFixture(t, settings.DeliveryBoth)
	logicJoin(t, service, bot)
	record := logicPending(t, db)
	bot.approveErr = logicRequestGone()
	bot.deleted = nil
	logicAnswer(t, db, service, bot, true)
	service.RunPendingActionsOnce(context.Background())
	if len(bot.deleted) != 1 || bot.deleted[0] != [2]int64{logicChatID, int64(record.GroupMsgID)} {
		t.Fatalf("group-only cleanup = %v, want message %d", bot.deleted, record.GroupMsgID)
	}
}

func TestReviewRestartThenCancelQueuedSettlement(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop", true: "remove"}[remove], func(t *testing.T) {
			db, bot, service := logicFixture(t, settings.DeliveryBoth)
			logicJoin(t, service, bot)
			bot.declineErr = errors.New("lost decline response")
			logicAnswer(t, db, service, bot, false)
			restarted := newLogicService(t, db, bot, settings.DeliveryBoth)
			if remove {
				restarted.RemoveGroup(logicChatID)
			} else if err := restarted.SetEnabled(logicChatID, false); err != nil {
				t.Fatal(err)
			}
			bot.declineErr = nil
			logicMakeRetryDue(t, db)
			restarted.RunPendingActionsOnce(context.Background())
			if bot.declines != 1 || len(logicFailures(t, db)) != 0 {
				t.Fatalf("canceled retry: declines=%d failures=%+v", bot.declines, logicFailures(t, db))
			}
		})
	}
}

func TestReviewRestartThenStopReleasesQueuedApprovalHold(t *testing.T) {
	db, bot, service := logicFixture(t, settings.DeliveryBoth)
	bot.member = &verification.ChatMemberMember{Status: verification.MemberStatusMember}
	logicMembership(t, service, bot)
	record := logicPending(t, db)
	if !record.Held || record.GroupMsgID == 0 {
		t.Fatalf("expected held member with group question: %+v", record)
	}
	bot.member = &verification.ChatMemberRestricted{
		Status: verification.MemberStatusRestricted, IsMember: true, UntilDate: record.HoldUntil,
	}
	bot.unmuteErr = errors.New("transient unmute failure")
	logicAnswer(t, db, service, bot, true)
	if bot.unmutes != 1 {
		t.Fatalf("initial approval: unmutes=%d", bot.unmutes)
	}
	bot.deleted = nil
	service.Shutdown()
	restarted := newLogicService(t, db, bot, settings.DeliveryBoth)
	records, err := database.NewVerificationStore(db).LoadPending("")
	if err != nil || len(records) != 0 {
		t.Fatalf("approved challenge restored as pending: %+v, error %v", records, err)
	}
	bot.unmuteErr = nil
	if err := restarted.SetEnabled(logicChatID, false); err != nil {
		t.Fatal(err)
	}
	if bot.unmutes != 2 || bot.member.MemberStatus() != verification.MemberStatusMember {
		t.Errorf("stop did not release verification hold: unmutes=%d member=%+v", bot.unmutes, bot.member)
	}
	if len(bot.deleted) != 1 || bot.deleted[0] != [2]int64{logicChatID, int64(record.GroupMsgID)} {
		t.Errorf("stop cleanup = %v, want group question %d", bot.deleted, record.GroupMsgID)
	}
	logicMakeRetryDue(t, db)
	restarted.RunPendingActionsOnce(context.Background())
	if bot.unmutes != 2 || bot.approves != 0 || bot.declines != 0 || len(logicFailures(t, db)) != 0 {
		t.Errorf("canceled approval retried or struck: unmutes=%d approvals=%d declines=%d failures=%+v",
			bot.unmutes, bot.approves, bot.declines, logicFailures(t, db))
	}
}

func logicPending(t *testing.T, db *database.Database) verification.PendingRecord {
	t.Helper()
	records, err := database.NewVerificationStore(db).LoadPending("")
	if err != nil || len(records) != 1 {
		t.Fatalf("pending = %+v, error %v", records, err)
	}
	return records[0]
}

func logicExpire(t *testing.T, db *database.Database) {
	t.Helper()
	record := logicPending(t, db)
	expected := record.Ref()
	record.Deadline = time.Now().Add(-time.Second).Unix()
	if changed, err := database.NewVerificationStore(db).UpdatePending("", expected, record); err != nil || !changed {
		t.Fatalf("advance deadline: %t %v", changed, err)
	}
}

func TestReviewRestoredExpiryRemainsStrikeFree(t *testing.T) {
	for _, cause := range []string{"restart-lapsed", "recovered"} {
		for _, retry := range []bool{false, true} {
			t.Run(cause+map[bool]string{false: "/direct", true: "/retry"}[retry], func(t *testing.T) {
				db, bot, service := logicFixture(t, settings.DeliveryBoth)
				logicJoin(t, service, bot)
				logicExpire(t, db)
				if cause == "recovered" {
					if err := database.NewVerificationStore(db).SaveHeartbeat("", verification.HeartbeatRecord{
						LastOnline: time.Now().Add(-time.Hour).Unix(),
					}); err != nil {
						t.Fatal(err)
					}
				}
				// The first restart records the lapsed cause; its service is not used again.
				_ = newLogicService(t, db, bot, settings.DeliveryBoth)
				if err := database.NewVerificationStore(db).SaveHeartbeat("", verification.HeartbeatRecord{LastOnline: time.Now().Unix()}); err != nil {
					t.Fatal(err)
				}
				// Restart again within the replacement window: its cause must survive too.
				restarted := newLogicService(t, db, bot, settings.DeliveryBoth)
				logicExpire(t, db)
				if retry {
					bot.declineErr = errors.New("lost decline response")
				}
				restarted.ScanExpired(context.Background())
				if retry {
					bot.declineErr = nil
					logicMakeRetryDue(t, db)
					restarted.RunPendingActionsOnce(context.Background())
				}
				want := 1
				if retry {
					want = 2
				}
				if bot.declines != want || bot.bans != 0 || len(logicFailures(t, db)) != 0 {
					t.Fatalf("restored expiry: declines=%d bans=%d failures=%+v", bot.declines, bot.bans, logicFailures(t, db))
				}
			})
		}
	}
}

func TestReviewConsoleWrongAnswerStrikeIndependentOfRetry(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "retry"}[retry], func(t *testing.T) {
			db, bot, service := logicFixture(t, settings.DeliveryBoth)
			logicJoin(t, service, bot)
			queue, err := service.ConsoleQueue(context.Background(), logicChatID)
			if err != nil || len(queue) != 1 {
				t.Fatalf("queue = %+v, error %v", queue, err)
			}
			bot.declineErr = logicRequestGone()
			if retry {
				bot.declineErr = errors.New("lost decline response")
			}
			_, err = service.SettleConsole(context.Background(), verification.ConsoleSettlement{
				ID: queue[0].ID, GroupID: logicChatID, ActorID: 912, Expected: verification.ChallengePending,
				Target: verification.ChallengeDeclined, Reason: "wrong_answer",
			})
			if err != nil {
				t.Fatal(err)
			}
			if retry {
				bot.declineErr = logicRequestGone()
				logicMakeRetryDue(t, db)
				service.RunPendingActionsOnce(context.Background())
			}
			fails := logicFailures(t, db)
			if len(fails) != 1 || fails[0].Count != 1 {
				t.Fatalf("wrong-answer failures = %+v, want one strike", fails)
			}
		})
	}
}
