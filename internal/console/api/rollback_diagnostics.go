package api

import (
	"time"

	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const retryStoreWriteScope = "retry_store_write"

type diagnosticsRollbackResponse struct {
	Rejections        diagnosticsRejectionsResponse        `json:"rejections"`
	ChallengeDelivery diagnosticsChallengeDeliveryResponse `json:"challenge_delivery"`
	ConsoleAccess     diagnosticsConsoleAccessResponse     `json:"console_access"`
	DatabaseWrites    diagnosticsDatabaseWriteResponse     `json:"database_writes"`
}

type diagnosticsRejectionsResponse struct {
	SourceAvailable     bool                                 `json:"source_available"`
	HumanReviewRequired bool                                 `json:"human_review_required"`
	WindowSeconds       int64                                `json:"window_seconds"`
	WindowStart         time.Time                            `json:"window_start"`
	WindowEnd           time.Time                            `json:"window_end"`
	ByReason            []diagnosticsRejectionReasonResponse `json:"by_reason"`
}

type diagnosticsRejectionReasonResponse struct {
	Reason *string `json:"reason"`
	Count  int64   `json:"count"`
}

type diagnosticsProblemStreakResponse struct {
	ThresholdSeconds   int64      `json:"threshold_seconds"`
	FirstProblemAt     *time.Time `json:"first_problem_at"`
	LastProblemAt      *time.Time `json:"last_problem_at"`
	LastRecoveredAt    *time.Time `json:"last_recovered_at"`
	ProblemEvents      uint64     `json:"problem_events"`
	ProblemSpanSeconds float64    `json:"problem_span_seconds"`
	ExceedsThreshold   bool       `json:"exceeds_threshold"`
}

type diagnosticsChallengeDeliveryResponse struct {
	Streak              diagnosticsProblemStreakResponse `json:"streak"`
	FailedDeliveries    uint64                           `json:"failed_deliveries"`
	DuplicateDeliveries uint64                           `json:"duplicate_deliveries"`
}

type diagnosticsConsoleAccessResponse struct {
	Streak              diagnosticsProblemStreakResponse `json:"streak"`
	UnavailableAttempts uint64                           `json:"unavailable_attempts"`
}

type diagnosticsDatabaseWriteResponse struct {
	Scope              string    `json:"scope"`
	WindowSeconds      int64     `json:"window_seconds"`
	WindowStart        time.Time `json:"window_start"`
	WindowEnd          time.Time `json:"window_end"`
	TotalWrites        uint64    `json:"total_writes"`
	FailedWrites       uint64    `json:"failed_writes"`
	FailureRatePercent float64   `json:"failure_rate_percent"`
	ExceedsOnePercent  bool      `json:"exceeds_one_percent"`
}

func diagnosticsRollbackView(
	snapshot status.RollbackSnapshot,
	rejections []verification.RejectionReasonCount,
	rejectionsAvailable bool,
) diagnosticsRollbackResponse {
	byReason := make([]diagnosticsRejectionReasonResponse, 0, len(rejections))
	if rejectionsAvailable {
		for _, entry := range rejections {
			view := diagnosticsRejectionReasonResponse{Count: entry.Count}
			if entry.Reason != nil {
				reason := *entry.Reason
				view.Reason = &reason
			}
			byReason = append(byReason, view)
		}
	}
	windowStart := snapshot.ObservedAt.Add(-verification.RollbackRejectionWindow)
	return diagnosticsRollbackResponse{
		Rejections: diagnosticsRejectionsResponse{
			SourceAvailable:     rejectionsAvailable,
			HumanReviewRequired: true,
			WindowSeconds:       int64(verification.RollbackRejectionWindow / time.Second),
			WindowStart:         windowStart,
			WindowEnd:           snapshot.ObservedAt,
			ByReason:            byReason,
		},
		ChallengeDelivery: diagnosticsChallengeDeliveryResponse{
			Streak:              diagnosticsProblemStreakView(snapshot.ChallengeDelivery.ProblemStreakSnapshot),
			FailedDeliveries:    snapshot.ChallengeDelivery.FailedDeliveries,
			DuplicateDeliveries: snapshot.ChallengeDelivery.DuplicateDeliveries,
		},
		ConsoleAccess: diagnosticsConsoleAccessResponse{
			Streak:              diagnosticsProblemStreakView(snapshot.ConsoleAccess),
			UnavailableAttempts: snapshot.ConsoleAccess.ProblemEvents,
		},
		DatabaseWrites: diagnosticsDatabaseWriteResponse{
			Scope:              retryStoreWriteScope,
			WindowSeconds:      int64(status.RollbackObservationWindow / time.Second),
			WindowStart:        snapshot.DatabaseWrites.WindowStart,
			WindowEnd:          snapshot.DatabaseWrites.WindowEnd,
			TotalWrites:        snapshot.DatabaseWrites.TotalWrites,
			FailedWrites:       snapshot.DatabaseWrites.FailedWrites,
			FailureRatePercent: snapshot.DatabaseWrites.FailureRatePercent,
			ExceedsOnePercent:  snapshot.DatabaseWrites.ExceedsOnePercent,
		},
	}
}

func diagnosticsProblemStreakView(snapshot status.ProblemStreakSnapshot) diagnosticsProblemStreakResponse {
	return diagnosticsProblemStreakResponse{
		ThresholdSeconds:   int64(status.RollbackObservationWindow / time.Second),
		FirstProblemAt:     snapshot.FirstProblemAt,
		LastProblemAt:      snapshot.LastProblemAt,
		LastRecoveredAt:    snapshot.LastRecoveredAt,
		ProblemEvents:      snapshot.ProblemEvents,
		ProblemSpanSeconds: snapshot.ObservedFor.Seconds(),
		ExceedsThreshold:   snapshot.ExceedsWindow,
	}
}
