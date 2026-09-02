package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
	"go.mau.fi/util/dbutil"
)

const observationVersionTable = "verification_observation_version"

//go:embed observation_migrations/*.sql
var rawObservationUpgrades embed.FS

var observationUpgrades = dbutil.BuildUpgradeTable().
	WithFSPath(rawObservationUpgrades, "observation_migrations").
	Finish()

// ObservationStore owns the independent observe-only journal schema. It does not share the main
// schema version because the journal is enabled before cutover without changing release metadata.
type ObservationStore struct {
	root *Database
	db   *dbutil.Database
	now  func() time.Time
}

var _ verification.ObservationRecorder = (*ObservationStore)(nil)

// StoredObservedAction is one durable suppressed write returned for cutover comparison.
type StoredObservedAction struct {
	ID         string
	ObservedAt int64
	verification.ObservedAction
}

// NewObservationStore upgrades the journal schema before any observe-only update is accepted.
func NewObservationStore(
	ctx context.Context,
	db *Database,
	now func() time.Time,
) (*ObservationStore, error) {
	if db == nil || db.Database == nil {
		return nil, fmt.Errorf("observation store requires a database")
	}
	if now == nil {
		return nil, fmt.Errorf("observation store requires a clock")
	}
	child := db.Child(observationVersionTable, observationUpgrades, nil)
	if err := child.Upgrade(ctx); err != nil {
		return nil, fmt.Errorf("upgrade observation schema: %w", err)
	}
	return &ObservationStore{root: db, db: child, now: now}, nil
}

// RecordObservedAction persists the write shape before reporting simulated success to the core.
func (s *ObservationStore) RecordObservedAction(ctx context.Context, action verification.ObservedAction) error {
	if err := validateObservedAction(action); err != nil {
		return err
	}
	id, err := newObservationID()
	if err != nil {
		return err
	}
	return s.db.DoTxn(ctx, nil, func(txCtx context.Context) error {
		chatID, userID := optionalObservationSubject(action)
		if chatID != nil {
			if err := ensureChat(txCtx, s.root, action.ChatID); err != nil {
				return err
			}
		}
		_, err := s.db.Exec(txCtx, `
			INSERT INTO verification_observation
				(id, observed_at, operation, chat_id, user_id, seconds, flag)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, s.now().Unix(), action.Operation, chatID, userID, action.Seconds, action.Flag)
		if err != nil {
			return fmt.Errorf("record observed %s: %w", action.Operation, err)
		}
		return nil
	})
}

func validateObservedAction(action verification.ObservedAction) error {
	if !action.Operation.Valid() {
		return fmt.Errorf("invalid observed operation %q", action.Operation)
	}
	hasSubject := observedOperationHasSubject(action.Operation)
	if hasSubject && (action.ChatID == 0 || action.UserID <= 0) {
		return fmt.Errorf("observed %s requires a chat and user", action.Operation)
	}
	if !hasSubject && (action.ChatID != 0 || action.UserID != 0) {
		return fmt.Errorf("observed %s must not retain a chat or user", action.Operation)
	}
	return nil
}

func observedOperationHasSubject(operation verification.ObservedOperation) bool {
	switch operation {
	case verification.ObservedApproveJoin, verification.ObservedDeclineJoin, verification.ObservedBan,
		verification.ObservedUnban, verification.ObservedMute, verification.ObservedUnmute:
		return true
	default:
		return false
	}
}

func optionalObservationSubject(action verification.ObservedAction) (any, any) {
	if action.ChatID == 0 {
		return nil, nil
	}
	return action.ChatID, action.UserID
}

func newObservationID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create observation ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

// LoadObservedActions returns the durable journal by observation time with a stable ID tie-breaker.
func (s *ObservationStore) LoadObservedActions(ctx context.Context) ([]StoredObservedAction, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, observed_at, operation, chat_id, user_id, seconds, flag
		  FROM verification_observation
		 ORDER BY observed_at, id`)
	if err != nil {
		return nil, fmt.Errorf("load observed actions: %w", err)
	}
	defer rows.Close()
	actions := make([]StoredObservedAction, 0)
	for rows.Next() {
		action, err := scanObservedAction(rows)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate observed actions: %w", err)
	}
	return actions, nil
}

func scanObservedAction(row interface{ Scan(...any) error }) (StoredObservedAction, error) {
	var action StoredObservedAction
	var chatID, userID sql.NullInt64
	if err := row.Scan(
		&action.ID, &action.ObservedAt, &action.Operation, &chatID, &userID,
		&action.Seconds, &action.Flag,
	); err != nil {
		return action, fmt.Errorf("scan observed action: %w", err)
	}
	if chatID.Valid {
		action.ChatID = chatID.Int64
	}
	if userID.Valid {
		action.UserID = userID.Int64
	}
	if err := validateObservedAction(action.ObservedAction); err != nil {
		return action, fmt.Errorf("load observation %s: %w", action.ID, err)
	}
	return action, nil
}
