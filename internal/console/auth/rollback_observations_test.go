package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/status"
)

type rollbackAvailabilityChecker struct {
	err error
}

func (c *rollbackAvailabilityChecker) CachedAdmin(context.Context, int64, int64) (bool, error) {
	return c.check()
}

func (c *rollbackAvailabilityChecker) FreshAdmin(context.Context, int64, int64) (bool, error) {
	return c.check()
}

func (c *rollbackAvailabilityChecker) check() (bool, error) {
	if c.err != nil {
		return false, c.err
	}
	return true, nil
}

func TestAuthorizeChatObservationRequiresSustainedUnavailability(t *testing.T) {
	now := time.Date(2026, time.September, 3, 13, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	checker := &rollbackAvailabilityChecker{err: errors.New("getChatMember unavailable")}
	manager, err := New(Config{
		BotToken: "123:token", Now: func() time.Time { return now }, AdminChecker: checker, AccessObserver: observations,
	})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{Principal: Principal{TelegramID: 7, Role: RoleManager}}

	if err = manager.AuthorizeChat(context.Background(), session, -1009000000997, ReadAccess); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatalf("first unavailable access error = %v", err)
	}
	if observations.Snapshot().ConsoleAccess.ExceedsWindow {
		t.Fatal("one unavailable access check reached the sustained threshold")
	}

	now = now.Add(status.RollbackObservationWindow)
	if err = manager.AuthorizeChat(context.Background(), session, -1009000000997, ReadAccess); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatalf("threshold unavailable access error = %v", err)
	}
	if observations.Snapshot().ConsoleAccess.ExceedsWindow {
		t.Fatal("an access outage at exactly ten minutes reached the threshold")
	}

	now = now.Add(time.Second)
	if err = manager.AuthorizeChat(context.Background(), session, -1009000000997, ReadAccess); !errors.Is(err, ErrAccessUnavailable) {
		t.Fatalf("sustained unavailable access error = %v", err)
	}
	sustained := observations.Snapshot().ConsoleAccess
	if !sustained.ExceedsWindow || sustained.ProblemEvents != 3 {
		t.Fatalf("sustained console access observation = %+v", sustained)
	}

	checker.err = nil
	if err = manager.AuthorizeChat(context.Background(), session, -1009000000997, ReadAccess); err != nil {
		t.Fatalf("recovered access error = %v", err)
	}
	recovered := observations.Snapshot().ConsoleAccess
	if recovered.FirstProblemAt != nil || recovered.LastProblemAt != nil || recovered.ProblemEvents != 0 ||
		recovered.LastRecoveredAt == nil || !recovered.LastRecoveredAt.Equal(now) {
		t.Fatalf("recovered console access observation = %+v", recovered)
	}
}

// The console_access reading is named for the console being unable to hand out a session, and it
// counted only AuthorizeChat. An instance that could not exchange any operator link at all — a
// full session table — still read as healthy, so the rollback condition covered half of what it
// says.
func TestAFullSessionTableCountsTowardConsoleAccess(t *testing.T) {
	now := time.Date(2026, time.September, 4, 2, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	const owner = int64(7)
	manager, err := New(Config{
		BotToken: "123:token", Now: func() time.Time { return now }, MaxEntries: 1,
		AdminChecker: &rollbackAvailabilityChecker{}, AccessObserver: observations,
		OperatorAllowed: func(int64) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := manager.IssueOperatorLink(owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.RedeemOperatorLink(first); err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.IssueOperatorLink(owner + 1)
	if err != nil {
		t.Fatal(err)
	}

	before := observations.Snapshot().ConsoleAccess
	if _, err = manager.RedeemOperatorLink(second); !errors.Is(err, ErrSessionCapacity) {
		t.Fatalf("redemption with a full session table = %v, want ErrSessionCapacity", err)
	}
	if observations.Snapshot().ConsoleAccess == before {
		t.Fatal("a refused redemption left the console access reading unchanged; the rollback " +
			"condition would read healthy while nobody can enter the console")
	}
}

// A link that is invalid or already redeemed is this instance answering correctly. Those must not
// move the reading, or an operator clicking a stale link twice would count towards a rollback.
func TestAnAnsweredOperatorLinkDoesNotCountAsUnavailable(t *testing.T) {
	now := time.Date(2026, time.September, 4, 2, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	manager, err := New(Config{
		BotToken: "123:token", Now: func() time.Time { return now },
		AdminChecker: &rollbackAvailabilityChecker{}, AccessObserver: observations,
		OperatorAllowed: func(int64) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.RedeemOperatorLink(link); err != nil {
		t.Fatal(err)
	}
	baseline := observations.Snapshot().ConsoleAccess

	if _, err = manager.RedeemOperatorLink(link); !errors.Is(err, ErrOperatorLinkRedeemed) {
		t.Fatalf("second redemption = %v, want it refused as already redeemed", err)
	}
	if _, err = manager.RedeemOperatorLink("not-a-link"); !errors.Is(err, ErrOperatorLinkInvalid) {
		t.Fatalf("unknown link = %v, want it refused as invalid", err)
	}
	if observations.Snapshot().ConsoleAccess != baseline {
		t.Fatalf("answered links moved the console access reading: %+v, want %+v",
			observations.Snapshot().ConsoleAccess, baseline)
	}
}
