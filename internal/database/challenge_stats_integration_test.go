package database_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const statsIntegrationChatID int64 = -1009000000702

func TestConsoleStatsAssignsLocalDayBoundary(t *testing.T) {
	db, service := newStatsIntegrationService(t)
	location := mustLocation(t, "America/New_York")
	from := time.Date(2026, time.March, 8, 0, 0, 0, 0, location)
	boundary := time.Date(2026, time.March, 9, 0, 0, 0, 0, location)
	to := time.Date(2026, time.March, 10, 0, 0, 0, 0, location)

	seedStatsChallenge(t, db, "start", 1, verification.ChallengeApproved, "rule", from.Unix())
	seedStatsChallenge(t, db, "before-boundary", 2, verification.ChallengeExpired, "rule", boundary.Unix()-1)
	seedStatsChallenge(t, db, "at-boundary", 3, verification.ChallengeDeclined, "rule", boundary.Unix())
	seedStatsChallenge(t, db, "at-exclusive-end", 4, verification.ChallengeApproved, "rule", to.Unix())

	report, err := service.ConsoleStats(context.Background(), verification.ConsoleStatsRequest{
		GroupID: statsIntegrationChatID, From: from, To: to, Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Trend) != 2 || report.Trend[0].Date != "2026-03-08" ||
		report.Trend[0].Outcome.Challenges != 2 || report.Trend[0].Outcome.Approved != 1 ||
		report.Trend[0].Outcome.Expired != 1 || report.Trend[0].Outcome.PassRate != 0.5 {
		t.Fatalf("first local statistics day = %+v", report.Trend)
	}
	if report.Trend[1].Date != "2026-03-09" || report.Trend[1].Outcome.Challenges != 1 ||
		report.Trend[1].Outcome.Declined != 1 || report.Trend[1].Outcome.PassRate != 0 {
		t.Fatalf("second local statistics day = %+v", report.Trend[1])
	}
	if report.Summary.Challenges != 3 || report.Summary.Approved != 1 ||
		math.Abs(report.Summary.PassRate-1.0/3.0) > 1e-12 {
		t.Fatalf("statistics summary = %+v", report.Summary)
	}
}

func TestConsoleStatsEmptyIntervalReturnsZeros(t *testing.T) {
	_, service := newStatsIntegrationService(t)
	location := mustLocation(t, "UTC")
	from := time.Date(2026, time.September, 1, 0, 0, 0, 0, location)
	to := from.AddDate(0, 0, 1)

	report, err := service.ConsoleStats(context.Background(), verification.ConsoleStatsRequest{
		GroupID: statsIntegrationChatID, From: from, To: to, Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary != (verification.ConsoleStatsOutcome{}) || len(report.Trend) != 1 ||
		report.Trend[0].Outcome != (verification.ConsoleStatsOutcome{}) || report.Interceptions == nil ||
		len(report.Interceptions) != 0 {
		t.Fatalf("empty statistics interval = %+v", report)
	}
}

func TestConsoleStatsPreservesUnknownChallengeKind(t *testing.T) {
	db, service := newStatsIntegrationService(t)
	location := mustLocation(t, "UTC")
	from := time.Date(2026, time.August, 1, 0, 0, 0, 0, location)
	to := from.AddDate(0, 0, 1)

	seedStatsChallenge(t, db, "future-declined", 1, verification.ChallengeDeclined, "future-proof", from.Unix()+1)
	seedStatsChallenge(t, db, "future-banned", 2, verification.ChallengeBanned, "future-proof", from.Unix()+2)
	seedStatsChallenge(t, db, "future-expired", 3, verification.ChallengeExpired, "future-proof", from.Unix()+3)
	seedStatsChallenge(t, db, "rule-declined", 4, verification.ChallengeDeclined, "rule", from.Unix()+4)

	report, err := service.ConsoleStats(context.Background(), verification.ConsoleStatsRequest{
		GroupID: statsIntegrationChatID, From: from, To: to, Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []verification.ConsoleStatsInterception{{Kind: "future-proof", Count: 2}, {Kind: "rule", Count: 1}}
	if len(report.Interceptions) != len(want) {
		t.Fatalf("interception groups = %+v", report.Interceptions)
	}
	for index := range want {
		if report.Interceptions[index] != want[index] {
			t.Fatalf("interception groups = %+v, want %+v", report.Interceptions, want)
		}
	}
}

func newStatsIntegrationService(t *testing.T) (*database.Database, *verification.Service) {
	t.Helper()
	db, err := database.Open(context.Background(), database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(context.Background(), "INSERT INTO chat (id, title) VALUES ($1, $2)",
		statsIntegrationChatID, "Statistics test chat"); err != nil {
		t.Fatal(err)
	}
	config := &settings.Config{GroupIDs: []int64{statsIntegrationChatID}}
	baseline, err := settings.LoadBaseline(filepath.Join(t.TempDir(), "missing.yaml"), config)
	if err != nil {
		t.Fatal(err)
	}
	settingsStore, err := settings.NewStore("", baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(
		settingsStore, nil, database.NewVerificationStore(db), config, nil, nil, verification.Identity{}, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	return db, service
}

func seedStatsChallenge(
	t *testing.T,
	db *database.Database,
	id string,
	userID int64,
	state verification.ChallengeState,
	kind string,
	settledAt int64,
) {
	t.Helper()
	var reason any
	if state == verification.ChallengeDeclined {
		reason = "wrong_answer"
	}
	_, err := db.Exec(context.Background(), `
		INSERT INTO challenge
			(id, chat_id, user_id, state, kind, payload, delivery, reason, expires_at, settled_at, epoch)
		VALUES ($1, $2, $3, $4, $5, '{}', '{}', $6, $7, $8, 1)`,
		id, statsIntegrationChatID, userID, state, kind, reason, settledAt, settledAt)
	if err != nil {
		t.Fatal(err)
	}
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
