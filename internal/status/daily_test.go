package status

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

type dailyTestStore struct {
	mu         sync.Mutex
	enabled    bool
	lastDate   string
	loadErr    error
	claimErr   error
	claimCalls int
}

func (s *dailyTestStore) LoadDailyStatus(context.Context) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled, s.lastDate, s.loadErr
}

func (s *dailyTestStore) SetDailyEnabled(_ context.Context, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	return nil
}

func (s *dailyTestStore) ClaimDailyAttempt(_ context.Context, date string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	if s.claimErr != nil {
		return false, s.claimErr
	}
	if !s.enabled || (s.lastDate != "" && date <= s.lastDate) {
		return false, nil
	}
	s.lastDate = date
	return true, nil
}

func newDailyTestService(
	now *time.Time,
	store *dailyTestStore,
	owner *int64,
	send func(context.Context, int64, string) error,
) *DailyService {
	health := NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	observations := NewRollbackObservations(func() time.Time { return *now })
	return NewDailyService(DailyConfig{
		Store: store, Health: health, Observations: observations,
		PendingCount: func(context.Context) (int, error) { return 4, nil },
		OwnerID:      func() int64 { return *owner }, Send: send,
		Location: time.UTC, Now: func() time.Time { return *now }, Language: 2,
	})
}

func TestDailyServiceWaitsForNineAndDeduplicatesReload(t *testing.T) {
	now := time.Date(2026, 9, 11, 8, 59, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	sends := 0
	service := newDailyTestService(&now, store, &owner, func(context.Context, int64, string) error {
		sends++
		return nil
	})
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatalf("send before 09:00 = %d, want zero", sends)
	}
	now = now.Add(time.Minute)
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("send after 09:00 = %d, want one", sends)
	}
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	reloaded := newDailyTestService(&now, store, &owner, func(context.Context, int64, string) error {
		sends++
		return nil
	})
	if err := reloaded.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 1 {
		t.Fatalf("same-day sends after repeat/reload = %d, want one", sends)
	}
	now = now.AddDate(0, 0, 1)
	if err := reloaded.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 2 {
		t.Fatalf("next-day sends = %d, want two total", sends)
	}
}

func TestDailyServiceMissingOwnerAndDisabledDoNotClaim(t *testing.T) {
	now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	owner := int64(0)
	store := &dailyTestStore{enabled: true}
	sends := 0
	service := newDailyTestService(&now, store, &owner, func(context.Context, int64, string) error {
		sends++
		return nil
	})
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimCalls != 0 || sends != 0 {
		t.Fatalf("unbound owner claim calls=%d sends=%d, want zero", store.claimCalls, sends)
	}
	owner = 9001
	if err := service.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.claimCalls != 0 || sends != 0 {
		t.Fatalf("disabled daily check claims=%d sends=%d, want zero", store.claimCalls, sends)
	}
}

func TestDailyServicePreparesBeforeClaimAndFailureDoesNotRetry(t *testing.T) {
	var recorded bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&recorded)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	claimSeenBySender := false
	sends := 0
	failure := errors.New("telegram rejected")
	service := newDailyTestService(&now, store, &owner, func(_ context.Context, _ int64, _ string) error {
		sends++
		store.mu.Lock()
		claimSeenBySender = store.lastDate == "2026-09-11"
		store.mu.Unlock()
		return failure
	})
	if err := service.Check(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("daily send error = %v, want %v", err, failure)
	}
	if !claimSeenBySender || sends != 1 {
		t.Fatalf("claim/send ordering seen=%v sends=%d, want true and one", claimSeenBySender, sends)
	}
	if err := service.Check(context.Background()); err != nil {
		t.Fatalf("repeat failed daily check = %v, want nil after durable claim", err)
	}
	if sends != 1 {
		t.Fatalf("repeated failed daily sends = %d, want one", sends)
	}
	if strings.Count(recorded.String(), "\n") != 1 || strings.Contains(recorded.String(), failure.Error()) {
		t.Fatalf("one failed attempt must record one event without the upstream error: %q", recorded.String())
	}
	now = now.AddDate(0, 0, 1)
	if err := service.Check(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("next-day daily send error = %v, want %v", err, failure)
	}
	if sends != 2 {
		t.Fatalf("next-day daily sends = %d, want two", sends)
	}
}

func TestDailyServiceReportsObservedStateToOwner(t *testing.T) {
	now := time.Date(2026, 9, 10, 19, 1, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	var recipient int64
	var message string
	service := newDailyTestService(&now, store, &owner, func(_ context.Context, id int64, text string) error {
		recipient, message = id, text
		return nil
	})
	service.location = mustLoadDailyLocation(t, "Pacific/Kiritimati")
	service.health.SetTelegramReady(false)
	heartbeat := now.Add(-time.Minute)
	service.health.RecordTelegramProbe(heartbeat, 7*time.Millisecond)
	service.observations.RecordChallengeDeliveryFailure()
	service.observations.RecordChallengeDeliverySuccess()
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	labels := &i18n.Messages.Bot.Daily
	want := labels.Report.Render(i18n.LangEN, "2026-09-11",
		labels.Online.For(i18n.LangEN), labels.NotReady.For(i18n.LangEN),
		heartbeat.In(service.location).Format(time.RFC3339), now.In(service.location).Format(time.RFC3339), 4)
	if recipient != owner || message != want {
		t.Fatalf("delivered to %d: %q; want owner %d: %q", recipient, message, owner, want)
	}
}

func TestDailyServiceDoesNotClaimWhenSnapshotFails(t *testing.T) {
	now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	sends := 0
	health := NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	service := NewDailyService(DailyConfig{
		Store: store, Health: health, Observations: NewRollbackObservations(func() time.Time { return now }),
		PendingCount: func(context.Context) (int, error) { return 0, errors.New("aggregate unavailable") },
		OwnerID:      func() int64 { return owner }, Send: func(context.Context, int64, string) error {
			sends++
			return nil
		}, Location: time.UTC, Now: func() time.Time { return now }, Language: 2,
	})
	if err := service.Check(context.Background()); err == nil {
		t.Fatal("snapshot failure was ignored")
	}
	if store.claimCalls != 0 || sends != 0 {
		t.Fatalf("snapshot failure claim calls=%d sends=%d, want zero", store.claimCalls, sends)
	}
}

func TestDailyServiceUsesLocalDateAcrossTimezonesAndDST(t *testing.T) {
	shanghai := mustLoadDailyLocation(t, "Asia/Shanghai")
	newYork := mustLoadDailyLocation(t, "America/New_York")
	cases := []struct {
		name     string
		loc      *time.Location
		before   time.Time
		at       time.Time
		wantDate string
	}{
		{
			name: "utc", loc: time.UTC,
			before: time.Date(2026, 9, 11, 8, 59, 0, 0, time.UTC),
			at:     time.Date(2026, 9, 11, 9, 0, 0, 0, time.UTC), wantDate: "2026-09-11",
		},
		{
			name: "shanghai", loc: shanghai,
			before: time.Date(2026, 9, 11, 0, 59, 0, 0, time.UTC),
			at:     time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC), wantDate: "2026-09-11",
		},
		{
			name: "new york spring", loc: newYork,
			before: time.Date(2026, 3, 8, 12, 59, 0, 0, time.UTC),
			at:     time.Date(2026, 3, 8, 13, 0, 0, 0, time.UTC), wantDate: "2026-03-08",
		},
		{
			name: "new york fall", loc: newYork,
			before: time.Date(2026, 11, 1, 13, 59, 0, 0, time.UTC),
			at:     time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC), wantDate: "2026-11-01",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			owner := int64(9001)
			store := &dailyTestStore{enabled: true}
			sends := 0
			now := test.before
			service := NewDailyService(DailyConfig{
				Store: store, Health: NewHealth(func(context.Context) error { return nil }),
				Observations: NewRollbackObservations(time.Now),
				PendingCount: func(context.Context) (int, error) { return 0, nil },
				OwnerID:      func() int64 { return owner }, Send: func(context.Context, int64, string) error {
					sends++
					return nil
				}, Location: test.loc, Now: func() time.Time { return now }, Language: 2,
			})
			if err := service.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if sends != 0 {
				t.Fatalf("before local 09:00 sends=%d, want zero", sends)
			}
			now = test.at
			if err := service.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if sends != 1 || store.lastDate != test.wantDate {
				t.Fatalf("local daily claim sends=%d date=%q, want one and %q", sends, store.lastDate, test.wantDate)
			}
		})
	}
}

func TestDailyServiceCheckUsesOneNowForClaimAndRenderedDate(t *testing.T) {
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	times := []time.Time{
		time.Date(2026, 9, 11, 23, 59, 0, 0, time.UTC),
		time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
	}
	nowCalls := 0
	var message string
	health := NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	service := NewDailyService(DailyConfig{
		Store: store, Health: health, Observations: NewRollbackObservations(time.Now),
		PendingCount: func(context.Context) (int, error) { return 0, nil },
		OwnerID:      func() int64 { return owner },
		Send: func(_ context.Context, _ int64, text string) error {
			message = text
			return nil
		},
		Location: time.UTC, Language: i18n.LangEN,
		Now: func() time.Time {
			at := times[nowCalls]
			nowCalls++
			return at
		},
	})
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := i18n.Messages.Bot.Daily.Report.Render(
		i18n.LangEN, "2026-09-11",
		i18n.Messages.Bot.Daily.Online.For(i18n.LangEN),
		i18n.Messages.Bot.Daily.Ready.For(i18n.LangEN),
		i18n.Messages.Bot.Daily.Never.For(i18n.LangEN),
		i18n.Messages.Bot.Daily.NoFailure.For(i18n.LangEN),
		0,
	)
	if store.lastDate != "2026-09-11" || message != want {
		t.Fatalf("midnight-crossing attempt date=%q message=%q, want claimed 2026-09-11 and %q", store.lastDate, message, want)
	}
}

func TestDailyServiceRunChecksImmediatelyAndStopsOnCancel(t *testing.T) {
	now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	sent := make(chan struct{})
	service := newDailyTestService(&now, store, &owner, func(context.Context, int64, string) error {
		close(sent)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		service.Run(ctx)
		close(done)
	}()
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("Run did not perform its immediate check")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}

func mustLoadDailyLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}

func TestDailyServiceHonorsDisableDuringPreparation(t *testing.T) {
	now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
	owner := int64(9001)
	store := &dailyTestStore{enabled: true}
	sends := 0
	service := newDailyTestService(&now, store, &owner, func(context.Context, int64, string) error {
		sends++
		return nil
	})
	service.pendingCount = func(ctx context.Context) (int, error) {
		return 4, store.SetDailyEnabled(ctx, false)
	}
	if err := service.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sends != 0 || store.lastDate != "" {
		t.Fatalf("disabled during preparation: sends=%d date=%q, want no delivery or consumed date", sends, store.lastDate)
	}
}

func TestDailyServiceReturnsPersistenceErrorsWithoutDelivery(t *testing.T) {
	failure := errors.New("storage unavailable")
	cases := []struct {
		name  string
		store *dailyTestStore
	}{
		{"read", &dailyTestStore{enabled: true, loadErr: failure}},
		{"claim", &dailyTestStore{enabled: true, claimErr: failure}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 9, 11, 9, 1, 0, 0, time.UTC)
			owner := int64(9001)
			sends := 0
			service := newDailyTestService(&now, test.store, &owner, func(context.Context, int64, string) error {
				sends++
				return nil
			})
			err := service.Check(context.Background())
			if !errors.Is(err, ErrDailyUnavailable) || sends != 0 || test.store.lastDate != "" {
				t.Fatalf("persistence failure: error=%v sends=%d date=%q", err, sends, test.store.lastDate)
			}
		})
	}
}
