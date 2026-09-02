package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// LoadRecentRejectionReasons groups terminal rejections without exposing chat or user identities.
func (s *VerificationStore) LoadRecentRejectionReasons(
	ctx context.Context,
	since int64,
) ([]verification.RejectionReasonCount, error) {
	rows, err := s.db.Query(ctx, `
		SELECT reason, COUNT(*)
		  FROM challenge
		 WHERE settled_at >= $1
		   AND state IN ($2, $3)
		 GROUP BY reason
		 ORDER BY reason`,
		since, verification.ChallengeDeclined, verification.ChallengeBanned)
	if err != nil {
		return nil, fmt.Errorf("load recent rejected challenges: %w", err)
	}
	defer rows.Close()

	counts := make([]verification.RejectionReasonCount, 0)
	for rows.Next() {
		var reason sql.NullString
		var count int64
		if err = rows.Scan(&reason, &count); err != nil {
			return nil, fmt.Errorf("scan recent rejected challenge count: %w", err)
		}
		entry := verification.RejectionReasonCount{Count: count}
		if reason.Valid {
			value := reason.String
			entry.Reason = &value
		}
		counts = append(counts, entry)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent rejected challenge counts: %w", err)
	}
	return counts, nil
}
