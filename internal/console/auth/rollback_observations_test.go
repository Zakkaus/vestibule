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
