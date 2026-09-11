package database

import (
	"context"
	"database/sql"
	"fmt"
)

// DailyStatusStore persists the singleton daily owner-status switch and attempt date.
type DailyStatusStore struct {
	db *Database
}

// NewDailyStatusStore binds daily status persistence to the shared database.
func NewDailyStatusStore(db *Database) *DailyStatusStore {
	return &DailyStatusStore{db: db}
}

// LoadDailyStatus returns the switch and the most recent attempted local date.
func (s *DailyStatusStore) LoadDailyStatus(ctx context.Context) (enabled bool, lastAttemptDate string, err error) {
	if s == nil || s.db == nil {
		return false, "", fmt.Errorf("daily status store is unavailable")
	}
	err = s.db.QueryRow(ctx, `
		SELECT enabled, last_attempt_date FROM daily_status WHERE singleton=1`).
		Scan(&enabled, &lastAttemptDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, "", fmt.Errorf("daily status singleton is missing")
		}
		return false, "", fmt.Errorf("load daily status: %w", err)
	}
	return enabled, lastAttemptDate, nil
}

// SetDailyEnabled changes only the persistent switch and never changes the attempt date.
func (s *DailyStatusStore) SetDailyEnabled(ctx context.Context, enabled bool) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("daily status store is unavailable")
	}
	result, err := s.db.Exec(ctx, `
		UPDATE daily_status SET enabled=$1 WHERE singleton=1`, enabled)
	if err != nil {
		return fmt.Errorf("set daily status enabled: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check daily status update: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("daily status singleton is missing")
	}
	return nil
}

// ClaimDailyAttempt atomically records date before a sender performs external I/O.
func (s *DailyStatusStore) ClaimDailyAttempt(ctx context.Context, date string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("daily status store is unavailable")
	}
	result, err := s.db.Exec(ctx, `
		UPDATE daily_status
		SET last_attempt_date=$1
		WHERE singleton=1 AND enabled=TRUE AND last_attempt_date < $1`, date)
	if err != nil {
		return false, fmt.Errorf("claim daily status attempt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("check daily status claim: %w", err)
	}
	return rows == 1, nil
}
