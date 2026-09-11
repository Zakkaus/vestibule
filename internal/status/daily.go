package status

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

var (
	// ErrDailyUnavailable indicates that daily status persistence or transport is unavailable.
	ErrDailyUnavailable = errors.New("daily status is unavailable")
)

const dailyDeliveryTimeout = 10 * time.Second

// DailyPersistence is the durable singleton state used by the daily scheduler.
type DailyPersistence interface {
	LoadDailyStatus(context.Context) (enabled bool, lastAttemptDate string, err error)
	SetDailyEnabled(context.Context, bool) error
	ClaimDailyAttempt(context.Context, string) (bool, error)
}

// DailySnapshot is the redacted operator-facing state included in a daily report.
type DailySnapshot struct {
	At                     time.Time
	Online                 bool
	Ready                  bool
	TelegramProbe          *TelegramProbe
	LastChallengeFailureAt *time.Time
	PendingChallenges      int
}

// DailyStatus is the operator API view of the persistent daily switch.
type DailyStatus struct {
	Enabled  bool
	Time     string
	Timezone string
}

// DailyConfig wires process state into a DailyService.
type DailyConfig struct {
	Store        DailyPersistence
	Health       *Health
	Observations *RollbackObservations
	PendingCount func(context.Context) (int, error)
	OwnerID      func() int64
	Send         func(context.Context, int64, string) error
	Messages     *i18n.Catalog
	Location     *time.Location
	Language     i18n.Lang
	Now          func() time.Time
}

// DailyService prepares, claims, and sends one owner report per local day.
type DailyService struct {
	store        DailyPersistence
	health       *Health
	observations *RollbackObservations
	pendingCount func(context.Context) (int, error)
	ownerID      func() int64
	send         func(context.Context, int64, string) error
	messages     *i18n.Catalog
	location     *time.Location
	language     i18n.Lang
	now          func() time.Time
	checkMu      sync.Mutex
}

// NewDailyService constructs the process-local scheduler around durable state.
func NewDailyService(config DailyConfig) *DailyService {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	messages := config.Messages
	if messages == nil {
		messages = &i18n.Messages
	}
	return &DailyService{
		store: config.Store, health: config.Health, observations: config.Observations,
		pendingCount: config.PendingCount, ownerID: config.OwnerID, send: config.Send,
		messages: messages, location: config.Location, language: config.Language, now: now,
	}
}

// Status reads the durable switch and reports its fixed schedule and actual time zone.
func (s *DailyService) Status(ctx context.Context) (DailyStatus, error) {
	if s == nil || s.store == nil || s.location == nil {
		return DailyStatus{}, ErrDailyUnavailable
	}
	enabled, _, err := s.store.LoadDailyStatus(ctx)
	if err != nil {
		return DailyStatus{}, fmt.Errorf("%w: load state: %v", ErrDailyUnavailable, err)
	}
	return DailyStatus{Enabled: enabled, Time: "09:00", Timezone: s.location.String()}, nil
}

// SetEnabled persists only the switch; it does not reset the last attempted date.
func (s *DailyService) SetEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.store == nil {
		return ErrDailyUnavailable
	}
	if err := s.store.SetDailyEnabled(ctx, enabled); err != nil {
		return fmt.Errorf("%w: save state: %v", ErrDailyUnavailable, err)
	}
	return nil
}

func (s *DailyService) prepareSnapshot(ctx context.Context, at time.Time) (DailySnapshot, error) {
	if s == nil || s.health == nil || s.pendingCount == nil || s.observations == nil {
		return DailySnapshot{}, ErrDailyUnavailable
	}
	health := s.health.Snapshot()
	pending, err := s.pendingCount(ctx)
	if err != nil {
		return DailySnapshot{}, fmt.Errorf("prepare daily snapshot: %w", err)
	}
	if pending < 0 {
		return DailySnapshot{}, fmt.Errorf("prepare daily snapshot: negative pending count")
	}
	observations := s.observations.Snapshot()
	return DailySnapshot{
		At: at.UTC(), Online: health.Live, Ready: s.health.Ready(ctx),
		TelegramProbe:          health.TelegramProbe,
		LastChallengeFailureAt: observations.ChallengeDelivery.LastFailureAt,
		PendingChallenges:      pending,
	}, nil
}

func (s *DailyService) attemptReport(ctx context.Context, owner int64, at time.Time) error {
	date := at.Format(time.DateOnly)
	enabled, lastDate, err := s.store.LoadDailyStatus(ctx)
	if err != nil {
		return fmt.Errorf("%w: load state: %v", ErrDailyUnavailable, err)
	}
	if !enabled || date <= lastDate {
		return nil
	}
	snapshot, err := s.prepareSnapshot(ctx, at)
	if err != nil {
		return err
	}
	text := s.render(snapshot)
	claimed, err := s.store.ClaimDailyAttempt(ctx, date)
	if err != nil {
		return fmt.Errorf("%w: claim attempt: %v", ErrDailyUnavailable, err)
	}
	if !claimed {
		return nil
	}
	if err := s.send(ctx, owner, text); err != nil {
		log.Printf("daily status delivery failed: date=%s", date)
		return err
	}
	return nil
}

// Check attempts the current local date once when the fixed 09:00 boundary has passed.
func (s *DailyService) Check(ctx context.Context) error {
	if s == nil || s.store == nil || s.send == nil || s.ownerID == nil || s.location == nil {
		return ErrDailyUnavailable
	}
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	at := s.now().In(s.location)
	if at.Hour() < 9 {
		return nil
	}
	owner := s.ownerID()
	if owner <= 0 {
		return nil
	}
	deliveryCtx, cancel := context.WithTimeout(ctx, dailyDeliveryTimeout)
	defer cancel()
	return s.attemptReport(deliveryCtx, owner, at)
}

// Run checks immediately and then once per minute until ctx is cancelled.
func (s *DailyService) Run(ctx context.Context) {
	if s == nil {
		return
	}
	_ = s.Check(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.Check(ctx)
		}
	}
}

func (s *DailyService) render(snapshot DailySnapshot) string {
	labels := &s.messages.Bot.Daily
	online := labels.Offline.For(s.language)
	if snapshot.Online {
		online = labels.Online.For(s.language)
	}
	ready := labels.NotReady.For(s.language)
	if snapshot.Ready {
		ready = labels.Ready.For(s.language)
	}
	probe := labels.Never.For(s.language)
	if snapshot.TelegramProbe != nil {
		probe = snapshot.TelegramProbe.At.In(s.location).Format(time.RFC3339)
	}
	failure := labels.NoFailure.For(s.language)
	if snapshot.LastChallengeFailureAt != nil {
		failure = snapshot.LastChallengeFailureAt.In(s.location).Format(time.RFC3339)
	}
	return labels.Report.Render(s.language,
		snapshot.At.In(s.location).Format(time.DateOnly), online, ready, probe, failure,
		snapshot.PendingChallenges,
	)
}
