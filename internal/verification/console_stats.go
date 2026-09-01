package verification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

const maxConsoleStatsDays = 366

var (
	ErrConsoleStatsInvalid     = errors.New("invalid console statistics request")
	ErrConsoleStatsUnavailable = errors.New("console statistics are unavailable")
)

// ConsoleStatsRequest selects local calendar days. From is inclusive and To is exclusive;
// both must be midnight in Location. The caller supplies Location from the process setting.
type ConsoleStatsRequest struct {
	GroupID  int64
	From     time.Time
	To       time.Time
	Location *time.Location
}

// ConsoleStatsOutcome is the complete set of applicant outcomes used by the statistics screen.
// Superseded challenges are attempts abandoned by another path, not applicant outcomes.
type ConsoleStatsOutcome struct {
	Challenges int64
	Approved   int64
	Declined   int64
	Banned     int64
	Expired    int64
	PassRate   float64
}

// ConsoleStatsDay is one local calendar day in the requested time zone.
type ConsoleStatsDay struct {
	Date    string
	Outcome ConsoleStatsOutcome
}

// ConsoleStatsInterception counts rejection outcomes by the challenge kind stored in the row.
type ConsoleStatsInterception struct {
	Kind  string
	Count int64
}

// ConsoleStatsReport is the bounded projection exposed to the HTTP adapter.
type ConsoleStatsReport struct {
	From          string
	To            string
	Timezone      string
	Summary       ConsoleStatsOutcome
	Trend         []ConsoleStatsDay
	Interceptions []ConsoleStatsInterception
}

// ChallengeStatsBucket is one contiguous UTC interval representing a local calendar day.
type ChallengeStatsBucket struct {
	StartAt int64
	EndAt   int64
}

// ChallengeStatsCount is a database-aggregated state and kind count for one requested bucket.
type ChallengeStatsCount struct {
	Bucket int
	State  ChallengeState
	Kind   string
	Count  int64
}

type challengeStatsStore interface {
	LoadChallengeStats(context.Context, int64, []ChallengeStatsBucket) ([]ChallengeStatsCount, error)
}

// ConsoleStats returns database-aggregated outcomes without loading challenge rows into memory.
func (v *Service) ConsoleStats(ctx context.Context, request ConsoleStatsRequest) (ConsoleStatsReport, error) {
	report, buckets, err := newConsoleStatsReport(request)
	if err != nil {
		return ConsoleStatsReport{}, err
	}
	if err = ctx.Err(); err != nil {
		return ConsoleStatsReport{}, err
	}
	if len(buckets) == 0 {
		return report, nil
	}
	store, ok := v.stateStore.(challengeStatsStore)
	if !ok {
		return ConsoleStatsReport{}, ErrConsoleStatsUnavailable
	}
	counts, err := store.LoadChallengeStats(ctx, request.GroupID, buckets)
	if err != nil {
		return ConsoleStatsReport{}, fmt.Errorf("%w: %v", ErrConsoleStatsUnavailable, err)
	}
	if err = applyChallengeStatsCounts(&report, counts); err != nil {
		return ConsoleStatsReport{}, err
	}
	return report, nil
}

func newConsoleStatsReport(request ConsoleStatsRequest) (ConsoleStatsReport, []ChallengeStatsBucket, error) {
	if request.GroupID == 0 || request.Location == nil {
		return ConsoleStatsReport{}, nil, ErrConsoleStatsInvalid
	}
	from := request.From.In(request.Location)
	to := request.To.In(request.Location)
	if !localMidnight(from) || !localMidnight(to) || from.After(to) {
		return ConsoleStatsReport{}, nil, ErrConsoleStatsInvalid
	}
	report := ConsoleStatsReport{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Timezone: request.Location.String(),
		Trend: make([]ConsoleStatsDay, 0), Interceptions: make([]ConsoleStatsInterception, 0),
	}
	buckets := make([]ChallengeStatsBucket, 0)
	for cursor := from; cursor.Before(to); {
		if len(buckets) == maxConsoleStatsDays {
			return ConsoleStatsReport{}, nil, ErrConsoleStatsInvalid
		}
		next := cursor.AddDate(0, 0, 1)
		if !next.After(cursor) || next.After(to) {
			return ConsoleStatsReport{}, nil, ErrConsoleStatsInvalid
		}
		buckets = append(buckets, ChallengeStatsBucket{StartAt: cursor.Unix(), EndAt: next.Unix()})
		report.Trend = append(report.Trend, ConsoleStatsDay{Date: cursor.Format(time.DateOnly)})
		cursor = next
	}
	return report, buckets, nil
}

func localMidnight(value time.Time) bool {
	return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}

func applyChallengeStatsCounts(report *ConsoleStatsReport, counts []ChallengeStatsCount) error {
	interceptions := make(map[string]int64)
	for _, count := range counts {
		if count.Bucket < 0 || count.Bucket >= len(report.Trend) || count.Count <= 0 {
			return ErrConsoleStatsUnavailable
		}
		day := &report.Trend[count.Bucket].Outcome
		if !addChallengeStatsOutcome(day, count.State, count.Count) ||
			!addChallengeStatsOutcome(&report.Summary, count.State, count.Count) {
			return ErrConsoleStatsUnavailable
		}
		if count.State == ChallengeDeclined || count.State == ChallengeBanned {
			interceptions[count.Kind] += count.Count
		}
	}
	finalizeChallengeStatsOutcome(&report.Summary)
	for index := range report.Trend {
		finalizeChallengeStatsOutcome(&report.Trend[index].Outcome)
	}
	kinds := make([]string, 0, len(interceptions))
	for kind := range interceptions {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		report.Interceptions = append(report.Interceptions, ConsoleStatsInterception{
			Kind: kind, Count: interceptions[kind],
		})
	}
	return nil
}

func addChallengeStatsOutcome(outcome *ConsoleStatsOutcome, state ChallengeState, count int64) bool {
	switch state {
	case ChallengeApproved:
		outcome.Approved += count
	case ChallengeDeclined:
		outcome.Declined += count
	case ChallengeBanned:
		outcome.Banned += count
	case ChallengeExpired:
		outcome.Expired += count
	default:
		return false
	}
	outcome.Challenges += count
	return true
}

func finalizeChallengeStatsOutcome(outcome *ConsoleStatsOutcome) {
	if outcome.Challenges != 0 {
		outcome.PassRate = float64(outcome.Approved) / float64(outcome.Challenges)
	}
}
