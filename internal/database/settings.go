package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// SettingsStore persists sparse per-chat settings and their optimistic revision.
type SettingsStore struct {
	db *Database
}

var _ settings.Repository = (*SettingsStore)(nil)

// NewSettingsStore binds per-chat settings to the shared database.
func NewSettingsStore(db *Database) *SettingsStore {
	return &SettingsStore{db: db}
}

// LoadSettings returns every chat row; the settings layer decides which chats are active.
func (s *SettingsStore) LoadSettings() ([]settings.Record, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT id, settings, settings_revision FROM chat ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load chat settings: %w", err)
	}
	defer rows.Close()

	var records []settings.Record
	for rows.Next() {
		var (
			record   settings.Record
			payload  string
			revision int64
		)
		if err = rows.Scan(&record.ChatID, &payload, &revision); err != nil {
			return nil, fmt.Errorf("scan chat settings: %w", err)
		}
		if revision < 0 {
			return nil, fmt.Errorf("chat %d has negative settings revision %d", record.ChatID, revision)
		}
		if err = json.Unmarshal([]byte(payload), &record.Overrides); err != nil {
			return nil, fmt.Errorf("decode chat %d settings: %w", record.ChatID, err)
		}
		record.Revision = uint64(revision)
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat settings: %w", err)
	}
	return records, nil
}

// SeedSettings creates missing chats and imports a value only into an untouched empty row.
func (s *SettingsStore) SeedSettings(records []settings.Record) error {
	encoded := make([]string, len(records))
	for i, record := range records {
		payload, err := json.Marshal(record.Overrides)
		if err != nil {
			return fmt.Errorf("encode seed settings for chat %d: %w", record.ChatID, err)
		}
		encoded[i] = string(payload)
	}
	return s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		for i, record := range records {
			result, err := s.db.Exec(ctx, `
				INSERT INTO chat (id, title, settings, settings_revision)
				VALUES ($1, '', $2, $3) ON CONFLICT (id) DO NOTHING`,
				record.ChatID, encoded[i], record.Revision)
			if err != nil {
				return fmt.Errorf("seed settings for chat %d: %w", record.ChatID, err)
			}
			inserted, err := changedRow(result)
			if err != nil {
				return err
			}
			if inserted || (record.Revision == 0 && encoded[i] == "{}") {
				continue
			}
			_, err = s.db.Exec(ctx, `
				UPDATE chat SET settings=$1, settings_revision=$2
				WHERE id=$3 AND settings_revision=0 AND settings='{}'`,
				encoded[i], record.Revision, record.ChatID)
			if err != nil {
				return fmt.Errorf("initialize settings for chat %d: %w", record.ChatID, err)
			}
		}
		return nil
	})
}

// CompareAndSwapSettings commits exactly one complete sparse record at the expected revision.
func (s *SettingsStore) CompareAndSwapSettings(chatID int64, expectedRevision uint64, next settings.GroupOverrides) (uint64, bool, error) {
	payload, err := json.Marshal(next)
	if err != nil {
		return 0, false, fmt.Errorf("encode settings for chat %d: %w", chatID, err)
	}
	result, err := s.db.Exec(context.Background(), `
		UPDATE chat
		SET settings=$1, settings_revision=settings_revision+1
		WHERE id=$2 AND settings_revision=$3`, string(payload), chatID, expectedRevision)
	if err != nil {
		return 0, false, fmt.Errorf("write settings for chat %d: %w", chatID, err)
	}
	written, err := changedRow(result)
	if err != nil {
		return 0, false, err
	}
	if written {
		return expectedRevision + 1, true, nil
	}
	var actual int64
	err = s.db.QueryRow(context.Background(), "SELECT settings_revision FROM chat WHERE id=$1", chatID).Scan(&actual)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, fmt.Errorf("%w: %d", settings.ErrUnknownGroup, chatID)
		}
		return 0, false, fmt.Errorf("read settings revision for chat %d: %w", chatID, err)
	}
	if actual < 0 {
		return 0, false, fmt.Errorf("chat %d has negative settings revision %d", chatID, actual)
	}
	return uint64(actual), false, nil
}
