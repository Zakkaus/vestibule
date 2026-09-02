package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
)

const rollbackObservationGroupID int64 = -1009000000998

func TestChallengeDeliveryObservationRequiresSustainedFailures(t *testing.T) {
	now := time.Date(2026, time.September, 3, 11, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	service := rollbackObservationTestService(observations, &now)
	bot := &fakeVerifyBot{sendErr: errors.New("Bot API unavailable"), sendFailN: 3}

	first := service.deliverPendingChallenge(context.Background(), bot, rollbackObservationGroupID, 71, "Alice", &pending{})
	if first.delivered || observations.Snapshot().ChallengeDelivery.ExceedsWindow {
		t.Fatalf("first failed challenge delivery = %+v, observation=%+v", first, observations.Snapshot().ChallengeDelivery)
	}

	now = now.Add(status.RollbackObservationWindow)
	service.deliverPendingChallenge(context.Background(), bot, rollbackObservationGroupID, 72, "Bob", &pending{})
	atThreshold := observations.Snapshot().ChallengeDelivery
	if atThreshold.ExceedsWindow || atThreshold.FailedDeliveries != 2 {
		t.Fatalf("failed deliveries at threshold = %+v", atThreshold)
	}

	now = now.Add(time.Second)
	service.deliverPendingChallenge(context.Background(), bot, rollbackObservationGroupID, 73, "Carol", &pending{})
	sustained := observations.Snapshot().ChallengeDelivery
	if !sustained.ExceedsWindow || sustained.FailedDeliveries != 3 || sustained.DuplicateDeliveries != 0 {
		t.Fatalf("sustained failed challenge deliveries = %+v", sustained)
	}
}

func TestChallengeDeliveryObservationCountsRepeatedChallengeMessages(t *testing.T) {
	now := time.Date(2026, time.September, 3, 11, 0, 0, 0, time.UTC)
	observations := status.NewRollbackObservations(func() time.Time { return now })
	service := rollbackObservationTestService(observations, &now)
	result := service.deliverPendingChallenge(
		context.Background(), newFakeVerifyBot(), rollbackObservationGroupID, 71, "Alice", &pending{challengeDelivered: true},
	)
	snapshot := observations.Snapshot().ChallengeDelivery
	if !result.delivered || snapshot.DuplicateDeliveries != 1 || snapshot.FailedDeliveries != 0 ||
		snapshot.ProblemEvents != 1 || snapshot.ExceedsWindow {
		t.Fatalf("repeated challenge delivery result=%+v observation=%+v", result, snapshot)
	}
}

func rollbackObservationTestService(observer RollbackObserver, now *time.Time) *Service {
	service := newTestService(&settings.Config{
		Groups:       []settings.GroupConfig{{ID: rollbackObservationGroupID, DeliveryMode: settings.DeliveryGroup}},
		GroupIDs:     []int64{rollbackObservationGroupID},
		DeliveryMode: settings.DeliveryGroup,
	})
	service.rollbackObserver = observer
	service.timeNow = func() time.Time { return *now }
	return service
}
