package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zakkaus/vestibule/internal/verification"
)

const maxChallengeStatsBuckets = 366

// LoadChallengeStats aggregates terminal outcomes in the database. The generated CASE expression
// uses caller-computed UTC boundaries so SQLite and PostgreSQL share exact IANA-zone day semantics.
func (s *VerificationStore) LoadChallengeStats(
	ctx context.Context,
	chatID int64,
	buckets []verification.ChallengeStatsBucket,
) ([]verification.ChallengeStatsCount, error) {
	if err := validateChallengeStatsBuckets(chatID, buckets); err != nil {
		return nil, err
	}
	counts := make([]verification.ChallengeStatsCount, 0)
	if len(buckets) == 0 {
		return counts, nil
	}
	query, arguments := challengeStatsQuery(chatID, buckets)
	rows, err := s.db.Query(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load challenge statistics for chat %d: %w", chatID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var count verification.ChallengeStatsCount
		if err = rows.Scan(&count.Bucket, &count.State, &count.Kind, &count.Count); err != nil {
			return nil, fmt.Errorf("scan challenge statistics for chat %d: %w", chatID, err)
		}
		counts = append(counts, count)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate challenge statistics for chat %d: %w", chatID, err)
	}
	return counts, nil
}

func validateChallengeStatsBuckets(chatID int64, buckets []verification.ChallengeStatsBucket) error {
	if chatID == 0 {
		return fmt.Errorf("statistics chat ID is required")
	}
	if len(buckets) > maxChallengeStatsBuckets {
		return fmt.Errorf("statistics interval exceeds %d days", maxChallengeStatsBuckets)
	}
	for index, bucket := range buckets {
		if bucket.StartAt >= bucket.EndAt {
			return fmt.Errorf("statistics bucket %d is empty", index)
		}
		if index != 0 && bucket.StartAt != buckets[index-1].EndAt {
			return fmt.Errorf("statistics bucket %d is not contiguous", index)
		}
	}
	return nil
}

func challengeStatsQuery(
	chatID int64,
	buckets []verification.ChallengeStatsBucket,
) (string, []any) {
	var query strings.Builder
	query.WriteString("SELECT CASE")
	arguments := make([]any, 0, len(buckets)+2)
	arguments = append(arguments, chatID, buckets[0].StartAt)
	for index, bucket := range buckets {
		_, _ = fmt.Fprintf(&query, " WHEN settled_at < $%d THEN %d", index+3, index)
		arguments = append(arguments, bucket.EndAt)
	}
	lastBoundary := len(buckets) + 2
	_, _ = fmt.Fprintf(&query, ` END AS bucket, state, kind, COUNT(*)
		FROM challenge
		WHERE chat_id=$1 AND settled_at >= $2 AND settled_at < $%d
		  AND state NOT IN ('pending', 'superseded')
		GROUP BY 1, state, kind
		ORDER BY 1, state, kind`, lastBoundary)
	return query.String(), arguments
}
