package verification

import (
	"context"
	"errors"
	"time"
)

// RollbackRejectionWindow is the fixed lookback for the manual false-rejection review.
const RollbackRejectionWindow = 24 * time.Hour

// RejectionReasonCount is a privacy-preserving count of recent rejected challenges by stored reason.
type RejectionReasonCount struct {
	Reason *string
	Count  int64
}

// RollbackObserver receives non-user-content observations needed during a production cutover.
type RollbackObserver interface {
	RecordChallengeDeliverySuccess()
	RecordChallengeDeliveryFailure()
	RecordChallengeDeliveryDuplicate()
	RecordDatabaseWrite(success bool)
}

var ErrRollbackRejectionsUnavailable = errors.New("rollback rejection observations are unavailable")

type rejectionReasonStore interface {
	LoadRecentRejectionReasons(context.Context, int64) ([]RejectionReasonCount, error)
}

// RecentRejections returns declined and banned challenges settled since the supplied UTC boundary.
func (v *Service) RecentRejections(ctx context.Context, since time.Time) ([]RejectionReasonCount, error) {
	store, ok := v.stateStore.(rejectionReasonStore)
	if !ok {
		return nil, ErrRollbackRejectionsUnavailable
	}
	return store.LoadRecentRejectionReasons(ctx, since.UTC().Unix())
}
