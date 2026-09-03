package verification

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const consoleStatsTestChatID int64 = -1009000000704

type consoleStatsTestStore struct {
	testVerificationStore
	counts []ChallengeStatsCount
}

func (s *consoleStatsTestStore) LoadChallengeStats(
	context.Context,
	int64,
	[]ChallengeStatsBucket,
) ([]ChallengeStatsCount, error) {
	return append([]ChallengeStatsCount(nil), s.counts...), nil
}

func TestConsoleStatsRefusesInvalidRequests(t *testing.T) {
	location := time.UTC
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, location)
	to := from.AddDate(0, 0, 1)
	for _, test := range []struct {
		name    string
		request ConsoleStatsRequest
	}{
		{
			name:    "zero group ID",
			request: ConsoleStatsRequest{From: from, To: to, Location: location},
		},
		{
			name: "nil location",
			request: ConsoleStatsRequest{
				GroupID: consoleStatsTestChatID, From: from, To: to,
			},
		},
		{
			name: "non-midnight From",
			request: ConsoleStatsRequest{
				GroupID: consoleStatsTestChatID, From: from.Add(time.Minute), To: to, Location: location,
			},
		},
		{
			name: "range longer than 366 local days",
			request: ConsoleStatsRequest{
				GroupID: consoleStatsTestChatID, From: from, To: from.AddDate(0, 0, maxConsoleStatsDays+1), Location: location,
			},
		},
		{
			name: "non-midnight To",
			request: ConsoleStatsRequest{
				GroupID: consoleStatsTestChatID, From: from, To: to.Add(time.Minute), Location: location,
			},
		},
		{
			name: "From after To",
			request: ConsoleStatsRequest{
				GroupID: consoleStatsTestChatID, From: to, To: from, Location: location,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := consoleStatsReportResult(test.request)
			if !errors.Is(err, ErrConsoleStatsInvalid) {
				t.Fatalf("%s request error = %v, want %v", test.name, err, ErrConsoleStatsInvalid)
			}
		})
	}
}

func TestConsoleStatsBuildsEmptyRange(t *testing.T) {
	from := time.Date(2026, time.March, 8, 0, 0, 0, 0, time.UTC)
	report, buckets, err := newConsoleStatsReport(ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: from, To: from, Location: time.UTC,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.From != "2026-03-08" || report.To != "2026-03-08" || report.Timezone != "UTC" ||
		len(report.Trend) != 0 || report.Interceptions == nil || len(report.Interceptions) != 0 || len(buckets) != 0 {
		t.Fatalf("empty statistics range report=%+v buckets=%+v", report, buckets)
	}
}

func TestConsoleStatsAcceptsMaximumLocalDayRange(t *testing.T) {
	end := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -maxConsoleStatsDays)
	report, buckets, err := newConsoleStatsReport(ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: start, To: end, Location: time.UTC,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Trend) != maxConsoleStatsDays || len(buckets) != maxConsoleStatsDays ||
		buckets[0].StartAt != start.Unix() || buckets[len(buckets)-1].EndAt != end.Unix() {
		t.Fatalf("366-day statistics range report=%+v buckets=%+v", report, buckets)
	}
	for index := range buckets {
		if buckets[index].EndAt <= buckets[index].StartAt ||
			(index > 0 && buckets[index-1].EndAt != buckets[index].StartAt) {
			t.Fatalf("366-day bucket %d is not a contiguous positive interval: %+v", index, buckets)
		}
	}
}

func TestConsoleStatsBuildsContiguousLocalDayBuckets(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, time.March, 8, 0, 0, 0, 0, location)
	to := time.Date(2026, time.March, 11, 0, 0, 0, 0, location)
	report, buckets, err := newConsoleStatsReport(ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: from, To: to, Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDates := []string{"2026-03-08", "2026-03-09", "2026-03-10"}
	wantTrend := make([]ConsoleStatsDay, 0, len(wantDates))
	wantBuckets := make([]ChallengeStatsBucket, 0, len(wantDates))
	for index, date := range wantDates {
		start := from.AddDate(0, 0, index)
		end := start.AddDate(0, 0, 1)
		wantTrend = append(wantTrend, ConsoleStatsDay{Date: date})
		wantBuckets = append(wantBuckets, ChallengeStatsBucket{StartAt: start.Unix(), EndAt: end.Unix()})
	}
	if report.From != wantDates[0] || report.To != "2026-03-11" || report.Timezone != location.String() ||
		!slices.Equal(report.Trend, wantTrend) || !slices.Equal(buckets, wantBuckets) {
		t.Fatalf("ordinary statistics range report=%+v buckets=%+v, want report trend=%+v buckets=%+v",
			report, buckets, wantTrend, wantBuckets)
	}
}

func TestConsoleStatsRefusesNonMidnightFromAtATimezoneTransition(t *testing.T) {
	from, to, location, ok := consoleStatsMidnightTransition(false, true)
	if !ok {
		t.Fatal("timezone experiment found no non-midnight start that advances to midnight")
	}
	_, _, err := newConsoleStatsReport(ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: from, To: to, Location: location,
	})
	if !errors.Is(err, ErrConsoleStatsInvalid) {
		t.Fatalf("non-midnight From at %s from=%s to=%s error = %v, want %v",
			location, from, to, err, ErrConsoleStatsInvalid)
	}
}

func TestConsoleStatsRefusesNonMidnightToAtATimezoneTransition(t *testing.T) {
	from, to, location, ok := consoleStatsMidnightTransition(true, false)
	if !ok {
		t.Fatal("timezone experiment found no midnight start that advances to a non-midnight end")
	}
	_, _, err := newConsoleStatsReport(ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: from, To: to, Location: location,
	})
	if !errors.Is(err, ErrConsoleStatsInvalid) {
		t.Fatalf("non-midnight To at %s from=%s to=%s error = %v, want %v",
			location, from, to, err, ErrConsoleStatsInvalid)
	}
}

func TestConsoleStatsRefusesNonProgressingLocalDay(t *testing.T) {
	location, err := time.LoadLocation("Pacific/Apia")
	if err != nil {
		t.Fatalf("load Pacific/Apia timezone: %v", err)
	}
	from := time.Date(2011, time.December, 29, 0, 0, 0, 0, location)
	to := time.Date(2011, time.December, 31, 0, 0, 0, 0, location)
	next := from.AddDate(0, 0, 1)
	if !from.Before(to) || next.After(from) || next.After(to) {
		t.Fatalf("Pacific/Apia timezone from=%s next=%s to=%s did not produce a non-progressing contained day",
			from, next, to)
	}

	result := make(chan error, 1)
	go func() {
		_, _, err := newConsoleStatsReport(ConsoleStatsRequest{
			GroupID: consoleStatsTestChatID, From: from, To: to, Location: location,
		})
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, ErrConsoleStatsInvalid) {
			t.Fatalf("non-progressing Pacific/Apia day error = %v, want %v", err, ErrConsoleStatsInvalid)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("non-progressing Pacific/Apia day did not return; bucket construction lost progress")
	}
}

func TestConsoleStatsRefusesInvalidStoreResponses(t *testing.T) {
	for _, test := range []struct {
		name  string
		count ChallengeStatsCount
	}{
		{name: "negative bucket index", count: ChallengeStatsCount{Bucket: -1, State: ChallengeApproved, Count: 1}},
		{name: "upper-bound bucket index", count: ChallengeStatsCount{Bucket: 3, State: ChallengeApproved, Count: 1}},
		{name: "zero count", count: ChallengeStatsCount{State: ChallengeApproved}},
		{name: "negative count", count: ChallengeStatsCount{State: ChallengeApproved, Count: -1}},
		{name: "pending state", count: ChallengeStatsCount{State: ChallengePending, Count: 1}},
		{name: "superseded state", count: ChallengeStatsCount{State: ChallengeSuperseded, Count: 1}},
		{name: "unknown state", count: ChallengeStatsCount{State: ChallengeState("future-state"), Count: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := consoleStatsResponse(test.count)
			if !errors.Is(err, ErrConsoleStatsUnavailable) {
				t.Fatalf("%s aggregate error = %v, want %v", test.name, err, ErrConsoleStatsUnavailable)
			}
		})
	}
}

func TestConsoleStatsProjectsValidStoreResponse(t *testing.T) {
	report, err := consoleStatsResponse(
		ChallengeStatsCount{Bucket: 0, State: ChallengeApproved, Count: 2},
		ChallengeStatsCount{Bucket: 1, State: ChallengeDeclined, Kind: "abuse", Count: 3},
		ChallengeStatsCount{Bucket: 2, State: ChallengeBanned, Kind: "flood", Count: 1},
		ChallengeStatsCount{Bucket: 2, State: ChallengeExpired, Count: 4},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantTrend := []ConsoleStatsDay{
		{Date: "2026-01-01", Outcome: ConsoleStatsOutcome{Challenges: 2, Approved: 2, PassRate: 1}},
		{Date: "2026-01-02", Outcome: ConsoleStatsOutcome{Challenges: 3, Declined: 3}},
		{Date: "2026-01-03", Outcome: ConsoleStatsOutcome{Challenges: 5, Banned: 1, Expired: 4}},
	}
	wantSummary := ConsoleStatsOutcome{Challenges: 10, Approved: 2, Declined: 3, Banned: 1, Expired: 4, PassRate: 0.2}
	wantInterceptions := []ConsoleStatsInterception{{Kind: "abuse", Count: 3}, {Kind: "flood", Count: 1}}
	if report.From != "2026-01-01" || report.To != "2026-01-04" || report.Timezone != "UTC" ||
		!slices.Equal(report.Trend, wantTrend) || report.Summary != wantSummary ||
		!slices.Equal(report.Interceptions, wantInterceptions) {
		t.Fatalf("valid aggregate report = %+v, want trend=%+v summary=%+v interceptions=%+v",
			report, wantTrend, wantSummary, wantInterceptions)
	}
}

func consoleStatsReportResult(request ConsoleStatsRequest) (
	report ConsoleStatsReport,
	buckets []ChallengeStatsBucket,
	err error,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("statistics request panicked: %v", recovered)
		}
	}()
	return newConsoleStatsReport(request)
}

func consoleStatsMidnightTransition(
	startMidnight bool,
	endMidnight bool,
) (time.Time, time.Time, *time.Location, bool) {
	for _, name := range []string{
		"Africa/Cairo", "America/Argentina/San_Luis", "America/Asuncion", "America/Caracas", "America/Havana",
		"America/Santiago", "America/Sao_Paulo", "Asia/Amman", "Asia/Gaza", "Asia/Hebron", "Asia/Kathmandu",
		"Asia/Pyongyang", "Australia/Lord_Howe", "Pacific/Apia", "Pacific/Chatham", "Pacific/Enderbury",
		"Pacific/Fakaofo", "Pacific/Fiji", "Pacific/Kiritimati", "Pacific/Kwajalein",
	} {
		location, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		for date := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC); date.Before(time.Date(2031, time.January, 1, 0, 0, 0, 0, time.UTC)); date = date.AddDate(0, 0, 1) {
			hours, minutes := []int{0}, []int{0}
			if !startMidnight {
				hours = []int{0, 1, 2, 3, 22, 23}
				minutes = []int{0, 15, 30, 45}
			}
			for _, hour := range hours {
				for _, minute := range minutes {
					from := time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location)
					to := from.AddDate(0, 0, 1)
					if consoleStatsIsMidnight(from) == startMidnight && consoleStatsIsMidnight(to) == endMidnight && to.After(from) {
						return from, to, location, true
					}
				}
			}
		}
	}
	return time.Time{}, time.Time{}, nil, false
}

func consoleStatsResponse(counts ...ChallengeStatsCount) (report ConsoleStatsReport, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("statistics store response panicked: %v", recovered)
		}
	}()
	service := newTestService(&settings.Config{})
	service.stateStore = &consoleStatsTestStore{counts: counts}
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	return service.ConsoleStats(context.Background(), ConsoleStatsRequest{
		GroupID: consoleStatsTestChatID, From: from, To: from.AddDate(0, 0, 3), Location: time.UTC,
	})
}

func consoleStatsIsMidnight(value time.Time) bool {
	return value.Hour() == 0 && value.Minute() == 0 && value.Second() == 0 && value.Nanosecond() == 0
}
