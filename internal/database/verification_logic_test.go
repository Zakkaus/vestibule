package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const logicChatID int64 = -1009000000911
const logicUserID int64 = 911

type logicGateway struct {
	verification.Gateway
	messages                        []verification.OutgoingMessage
	declines, approves, mutes, bans int
	declineErr, approveErr          error
	member                          verification.ChatMember
	deleted                         [][2]int64
	unmutes                         int
	unmuteErr                       error
}

func (b *logicGateway) Send(_ context.Context, message verification.OutgoingMessage) (int, error) {
	b.messages = append(b.messages, message)
	return len(b.messages), nil
}
func (b *logicGateway) SendHTMLFallback(c context.Context, id int64, rich, _ string) (int, error) {
	return b.Send(c, verification.OutgoingMessage{ChatID: id, Text: rich})
}
func (b *logicGateway) Delete(_ context.Context, chatID int64, messageID int) error {
	b.deleted = append(b.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}
func (b *logicGateway) Notify(c context.Context, id int64, text string, _ int) {
	_, _ = b.Send(c, verification.OutgoingMessage{ChatID: id, Text: text})
}
func (*logicGateway) Alert(context.Context, int64, string)            {}
func (*logicGateway) AuditLog(context.Context, int64, string)         {}
func (*logicGateway) FailAlert(context.Context, int64, int64, string) {}
func (b *logicGateway) ApproveJoin(context.Context, int64, int64) error {
	b.approves++
	return b.approveErr
}
func (b *logicGateway) DeclineJoin(context.Context, int64, int64) error {
	b.declines++
	return b.declineErr
}
func (b *logicGateway) Ban(context.Context, int64, int64, int, bool) error { b.bans++; return nil }
func (*logicGateway) Unban(context.Context, int64, int64, bool) error      { return nil }
func (b *logicGateway) Mute(context.Context, int64, int64, int) error      { b.mutes++; return nil }
func (b *logicGateway) Unmute(context.Context, int64, int64) error {
	b.unmutes++
	if b.unmuteErr == nil {
		b.member = &verification.ChatMemberMember{Status: verification.MemberStatusMember}
	}
	return b.unmuteErr
}
func (b *logicGateway) Member(context.Context, int64, int64) (verification.ChatMember, error) {
	return b.member, nil
}
func (*logicGateway) CachedAdmin(context.Context, int64, int64) (bool, error)         { return false, nil }
func (*logicGateway) FreshAdmin(context.Context, int64, int64) (bool, error)          { return false, nil }
func (*logicGateway) AckFast(context.Context, string) error                           { return nil }
func (*logicGateway) AckResult(context.Context, string, verification.AckResult) error { return nil }

func newLogicService(t *testing.T, db *Database, bot *logicGateway, delivery string) *verification.Service {
	t.Helper()
	cfg := &settings.Config{GroupIDs: []int64{logicChatID}, VerifyMode: settings.ModeQuiz,
		DeliveryMode: delivery, TimeoutSeconds: 30, VerifyMaxFails: 2, VerifyRetrySeconds: 180}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(groups, bot, NewVerificationStore(db), cfg, &i18n.Messages, nil,
		verification.Identity{ID: 999, Username: "logic_bot"}, filepath.Join(t.TempDir(), "state"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func logicFixture(t *testing.T, delivery string) (*Database, *logicGateway, *verification.Service) {
	t.Helper()
	db, err := Open(context.Background(), testDatabaseConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	bot := &logicGateway{member: &verification.ChatMemberLeft{Status: verification.MemberStatusLeft}}
	return db, bot, newLogicService(t, db, bot, delivery)
}

func logicJoin(t *testing.T, service *verification.Service, bot *logicGateway) {
	t.Helper()
	err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), bot), verification.Update{
		ChatJoinRequest: &verification.ChatJoinRequest{Chat: verification.Chat{ID: logicChatID, Type: verification.ChatTypeSupergroup},
			From: verification.User{ID: logicUserID, FirstName: "Applicant", LanguageCode: "en"}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func logicAnswer(t *testing.T, db *Database, service *verification.Service, bot *logicGateway, correct bool) {
	t.Helper()
	records, err := NewVerificationStore(db).LoadPending("")
	if err != nil || len(records) != 1 {
		t.Fatalf("pending challenge = %+v, error %v", records, err)
	}
	choice := records[0].CorrectIdx
	if !correct {
		choice = (choice + 1) % len(records[0].QOpts)
	}
	data := fmt.Sprintf("%s%d:%d:%s:%d", verification.AnswerCallbackPrefix, logicChatID, logicUserID, records[0].Nonce, choice)
	found := false
	for _, message := range bot.messages {
		for _, row := range message.Buttons {
			for _, button := range row {
				if button.CallbackData == data {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("answer callback %q was not delivered", data)
	}
	err = service.OnAnswer(verification.NewHandlerContext(context.Background(), bot), verification.Update{
		CallbackQuery: &verification.CallbackQuery{ID: "logic-answer", From: verification.User{ID: logicUserID, LanguageCode: "en"}, Data: data},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func logicMakeRetryDue(t *testing.T, db *Database) {
	t.Helper()
	// Advance only the scheduler boundary, not the challenge identity or settlement state.
	if _, err := db.Exec(context.Background(), "UPDATE pending_action SET next_try_at=0, claim_until=NULL WHERE state='pending'"); err != nil {
		t.Fatal(err)
	}
}

func logicFailures(t *testing.T, db *Database) []verification.FailureRecord {
	t.Helper()
	records, err := NewVerificationStore(db).LoadFailures("")
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestLogicStopCancelsQueuedSettlement(t *testing.T) {
	db, bot, service := logicFixture(t, settings.DeliveryBoth)
	logicJoin(t, service, bot)
	bot.declineErr = errors.New("lost decline response")
	logicAnswer(t, db, service, bot, false)
	if bot.declines != 1 {
		t.Fatalf("initial declines = %d", bot.declines)
	}
	if err := service.SetEnabled(logicChatID, false); err != nil {
		t.Fatal(err)
	}
	bot.declineErr = nil
	logicMakeRetryDue(t, db)
	service.RunPendingActionsOnce(context.Background())
	if bot.declines != 1 || len(logicFailures(t, db)) != 0 {
		t.Fatalf("canceled challenge settled: declines=%d failures=%+v", bot.declines, logicFailures(t, db))
	}
	var state string
	if err := db.QueryRow(context.Background(), "SELECT state FROM pending_action WHERE kind='settle_decline'").Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "failed" {
		t.Fatalf("obsolete action state = %q, want failed", state)
	}
}

func TestLogicDeliveredChallengeTimeoutStrikes(t *testing.T) {
	for _, delivery := range []string{settings.DeliveryGroup, settings.DeliveryDM, settings.DeliveryBoth} {
		t.Run(delivery, func(t *testing.T) {
			db, bot, service := logicFixture(t, delivery)
			logicJoin(t, service, bot)
			state := NewVerificationStore(db)
			records, err := state.LoadPending("")
			if err != nil || len(records) != 1 {
				t.Fatalf("pending = %+v, error %v", records, err)
			}
			record := records[0]
			if record.ChallengeDelivered || (record.GroupMsgID == 0 && record.PrivateMsgID == 0) {
				t.Fatalf("expected compact delivered record, got %+v", record)
			}
			expected := record.Ref()
			record.Deadline = time.Now().Unix() - 1
			if changed, err := state.UpdatePending("", expected, record); err != nil || !changed {
				t.Fatalf("advance deadline: %t %v", changed, err)
			}
			service.ScanExpired(context.Background())
			fails := logicFailures(t, db)
			if bot.declines != 1 || len(fails) != 1 || fails[0].Count != 1 {
				t.Fatalf("delivered timeout: declines=%d failures=%+v", bot.declines, fails)
			}
			logicJoin(t, service, bot)
			if bot.declines != 2 {
				t.Fatalf("timeout did not enforce cooldown: declines=%d", bot.declines)
			}
		})
	}
}

func logicMembership(t *testing.T, service *verification.Service, bot *logicGateway) {
	t.Helper()
	err := service.OnMemberJoined(verification.NewHandlerContext(context.Background(), bot), verification.Update{
		ChatMember: &verification.ChatMemberUpdated{Chat: verification.Chat{ID: logicChatID, Type: verification.ChatTypeSupergroup},
			From: verification.User{ID: 999}, OldChatMember: &verification.ChatMemberLeft{Status: verification.MemberStatusLeft},
			NewChatMember: &verification.ChatMemberMember{Status: verification.MemberStatusMember, User: verification.User{ID: logicUserID, LanguageCode: "en"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogicRestartPreservesConfirmedRecentPass(t *testing.T) {
	db, bot, service := logicFixture(t, settings.DeliveryBoth)
	logicJoin(t, service, bot)
	logicAnswer(t, db, service, bot, true)
	if bot.approves != 1 {
		t.Fatalf("approvals = %d", bot.approves)
	}
	// A delayed external approval must use its completion time, not its earlier claim time.
	if _, err := db.Exec(context.Background(), "UPDATE challenge SET settled_at=$1 WHERE state='approved'",
		time.Now().Add(-10*time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	restarted := newLogicService(t, db, bot, settings.DeliveryBoth)
	logicMembership(t, restarted, bot)
	records, err := NewVerificationStore(db).LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if bot.mutes != 0 || len(records) != 0 {
		t.Fatalf("confirmed pass rechallenged after restart: mutes=%d pending=%+v", bot.mutes, records)
	}
}

func TestLogicRestartDoesNotSuppressUnconfirmedOrExpiredPass(t *testing.T) {
	for _, outcome := range []string{"unconfirmed", "gone", "expired"} {
		t.Run(outcome, func(t *testing.T) {
			db, bot, service := logicFixture(t, settings.DeliveryBoth)
			logicJoin(t, service, bot)
			switch outcome {
			case "unconfirmed":
				bot.approveErr = errors.New("lost approval response")
			case "gone":
				bot.approveErr = &verification.GatewayError{
					Cause: errors.New("HIDE_REQUESTER_MISSING"), Kinds: verification.FailureJoinRequestGone,
				}
			}
			logicAnswer(t, db, service, bot, true)
			if outcome == "expired" {
				if _, err := db.Exec(context.Background(), "UPDATE pending_action SET done_at=$1 WHERE kind='settle_approve'",
					time.Now().Add(-5*time.Minute-time.Second).Unix()); err != nil {
					t.Fatal(err)
				}
			}
			restarted := newLogicService(t, db, bot, settings.DeliveryBoth)
			logicMembership(t, restarted, bot)
			if bot.mutes != 1 {
				t.Fatalf("%s approval suppressed a fresh arrival: mutes=%d", outcome, bot.mutes)
			}
		})
	}
}

func TestLogicRetryWrongAnswerStillStrikesWhenRequestGone(t *testing.T) {
	db, bot, service := logicFixture(t, settings.DeliveryBoth)
	logicJoin(t, service, bot)
	bot.declineErr = errors.New("lost decline response")
	logicAnswer(t, db, service, bot, false)
	if len(logicFailures(t, db)) != 0 {
		t.Fatal("unconfirmed decline recorded a strike")
	}
	bot.declineErr = &verification.GatewayError{Cause: errors.New("HIDE_REQUESTER_MISSING"), Kinds: verification.FailureJoinRequestGone}
	logicMakeRetryDue(t, db)
	service.RunPendingActionsOnce(context.Background())
	fails := logicFailures(t, db)
	if bot.declines != 2 || len(fails) != 1 || fails[0].Count != 1 {
		t.Fatalf("wrong-answer retry: declines=%d failures=%+v", bot.declines, fails)
	}
}
