package database

import (
	"context"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/moderate"
)

// WarningStore persists moderation warning snapshots in the shared database.
type WarningStore struct {
	db *Database
}

var _ moderate.WarningStore = (*WarningStore)(nil)

func NewWarningStore(db *Database) *WarningStore {
	return &WarningStore{db: db}
}

func (s *WarningStore) LoadWarnings() ([]moderate.WarningRecord, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT chat_id, user_id, count FROM warning_counter ORDER BY chat_id, user_id`)
	if err != nil {
		return nil, fmt.Errorf("load warning counters: %w", err)
	}
	defer rows.Close()
	var records []moderate.WarningRecord
	for rows.Next() {
		var record moderate.WarningRecord
		if err = rows.Scan(&record.GroupID, &record.UserID, &record.Count); err != nil {
			return nil, fmt.Errorf("scan warning counter: %w", err)
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("load warning counters: %w", err)
	}
	return records, nil
}

func (s *WarningStore) SaveWarnings(snapshot func() []moderate.WarningRecord) error {
	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()
	records := snapshot()
	return s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		return replaceWarnings(ctx, s.db, records)
	})
}

func replaceWarnings(ctx context.Context, db *Database, records []moderate.WarningRecord) error {
	if _, err := db.Exec(ctx, "DELETE FROM warning_counter"); err != nil {
		return fmt.Errorf("clear warning counters: %w", err)
	}
	for _, record := range records {
		if err := ensureChat(ctx, db, record.GroupID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
			INSERT INTO warning_counter (chat_id, user_id, count) VALUES ($1, $2, $3)`,
			record.GroupID, record.UserID, record.Count)
		if err != nil {
			return fmt.Errorf("insert warning counter for chat %d user %d: %w", record.GroupID, record.UserID, err)
		}
	}
	return nil
}
