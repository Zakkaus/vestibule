package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func (s *VerificationStore) EnqueueActions(_ string, expected verification.PendingRef, intents []verification.ActionIntent) ([]verification.PendingAction, error) {
	if err := validateActionIntents(intents); err != nil {
		return nil, err
	}
	var actions []verification.PendingAction
	err := s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		var err error
		actions, err = s.enqueueCleanupActions(ctx, challengeID(expected), intents)
		return err
	})
	return actions, err
}

// Cleanup can be rediscovered after a restart or a competing cancellation. Never
// steal an active lease, resurrect completed work, or bypass an existing backoff.
func (s *VerificationStore) enqueueCleanupActions(ctx context.Context, challenge string, intents []verification.ActionIntent) ([]verification.PendingAction, error) {
	var actions []verification.PendingAction
	for _, intent := range intents {
		action := verification.PendingAction{ActionIntent: intent, ChallengeID: challenge}
		err := s.db.QueryRow(ctx, `
			INSERT INTO pending_action (id, challenge_id, kind, payload, next_try_at, claim_owner, claim_until)
			VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, 0))
			ON CONFLICT (id) DO UPDATE SET claim_owner=excluded.claim_owner, claim_until=excluded.claim_until
			 WHERE pending_action.state='pending' AND pending_action.next_try_at <= excluded.next_try_at
			   AND excluded.claim_owner IS NOT NULL
			   AND (pending_action.claim_until IS NULL OR pending_action.claim_until <= excluded.next_try_at)
			RETURNING kind, payload, attempts, next_try_at`,
			intent.ID, challenge, intent.Kind, intent.Payload, intent.NextTryAt, intent.ClaimOwner, intent.ClaimUntil).
			Scan(&action.Kind, &action.Payload, &action.Attempts, &action.NextTryAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("enqueue cleanup %s: %w", intent.ID, err)
		}
		if intent.ClaimOwner != "" {
			actions = append(actions, action)
		}
	}
	return actions, nil
}

func (s *VerificationJSONStore) EnqueueActions(_ string, expected verification.PendingRef, intents []verification.ActionIntent) ([]verification.PendingAction, error) {
	if err := validateActionIntents(intents); err != nil {
		return nil, err
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	s.ensureActionsLocked()
	var actions []verification.PendingAction
	for _, intent := range intents {
		current, exists := s.actions[intent.ID]
		if exists && (current.state != "pending" || current.NextTryAt > intent.NextTryAt ||
			current.claimUntil > intent.NextTryAt || intent.ClaimOwner == "") {
			continue
		}
		if !exists {
			current = jsonPendingAction{PendingAction: verification.PendingAction{
				ActionIntent: intent, ChallengeID: challengeID(expected),
			}, state: "pending"}
		}
		current.claimOwner, current.claimUntil = intent.ClaimOwner, intent.ClaimUntil
		s.actions[intent.ID] = current
		if intent.ClaimOwner != "" {
			actions = append(actions, current.PendingAction)
		}
	}
	return actions, nil
}

func (s *VerificationStore) enqueueCancellationActions(ctx context.Context, record verification.PendingRecord, at int64) error {
	intents, err := verification.CleanupActions(record, at, true)
	if err != nil {
		return err
	}
	_, err = s.enqueueCleanupActions(ctx, challengeID(record.Ref()), intents)
	return err
}
