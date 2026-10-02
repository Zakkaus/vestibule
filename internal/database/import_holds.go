package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// ImportedUnrestrict is executed by app, not by the challenge-settlement worker.
const ImportedUnrestrict = "unrestrict"

type ImportedHold struct {
	ChatID    int64 `json:"chat_id"`
	UserID    int64 `json:"user_id"`
	HoldUntil int64 `json:"hold_until,omitempty"`
}

func enqueueImportedUnrestrict(ctx context.Context, db *Database, records []verification.PendingRecord) error {
	now := time.Now().Unix()
	for _, record := range records {
		id := "import-drop:" + challengeID(record.Ref()) + ":" + strconv.FormatInt(record.HoldUntil, 10)
		payload, err := json.Marshal(ImportedHold{record.GroupID, record.UserID, record.HoldUntil})
		if err != nil {
			return fmt.Errorf("encode imported hold: %w", err)
		}
		if err := ensureChat(ctx, db, record.GroupID); err != nil {
			return err
		}
		// An unsettled anchor keeps the outbox reference valid without entering challenge audit history.
		if _, err = db.Exec(ctx, `
			INSERT INTO challenge (id, chat_id, user_id, state, kind, payload, delivery, expires_at, reason)
			VALUES ($1, $2, $3, 'superseded', 'rule', '{}', '{}', $4, 'import-drop')
			ON CONFLICT DO NOTHING`, id, record.GroupID, record.UserID, now); err != nil {
			return fmt.Errorf("retain imported hold target: %w", err)
		}
		if _, err = db.Exec(ctx, `
			INSERT INTO pending_action (id, challenge_id, kind, payload, next_try_at)
			VALUES ($1, $1, $2, $3, $4) ON CONFLICT DO NOTHING`, id, ImportedUnrestrict, string(payload), now); err != nil {
			return fmt.Errorf("enqueue imported hold release: %w", err)
		}
	}
	return nil
}

func cancelCarriedUnrestrict(ctx context.Context, db *Database) error {
	// Deleting an anchor also removes its release through the existing outbox foreign key.
	_, err := db.Exec(ctx, `
		DELETE FROM challenge
		 WHERE reason='import-drop'
		   AND EXISTS (
		       SELECT 1 FROM pending_action AS release
		        WHERE release.challenge_id=challenge.id AND release.kind=$1 AND release.state='pending'
		   )
		   AND EXISTS (
		       SELECT 1 FROM challenge AS carried
		        WHERE carried.chat_id=challenge.chat_id AND carried.user_id=challenge.user_id
		          AND carried.state='pending'
		   )`, ImportedUnrestrict)
	if err != nil {
		return fmt.Errorf("cancel imported releases for carried challenges: %w", err)
	}
	return nil
}

// ClaimImportedUnrestrict shares the durable action lease and completion/retry methods.
func (s *VerificationStore) ClaimImportedUnrestrict(owner string, now, claimUntil int64, limit int) ([]verification.PendingAction, error) {
	return s.claimActions(owner, now, claimUntil, limit, true)
}
