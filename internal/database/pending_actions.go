package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/Zakkaus/vestibule/internal/verification"
)

const maxStoredEpoch = uint64(1<<63 - 1)

func (s *VerificationStore) transitionChallenge(ctx context.Context, transition verification.ChallengeTransition) (bool, error) {
	if err := validatePendingReplacement(transition.Expected, transition.Record); err != nil {
		return false, err
	}
	if err := validateActionIntents(transition.Actions); err != nil {
		return false, err
	}
	payload, delivery, err := encodePending(transition.Record)
	if err != nil {
		return false, err
	}
	var reason, settledAt, settledBy any
	if transition.To != verification.ChallengePending {
		settledAt = transition.SettledAt
		if transition.Reason != "" {
			reason = transition.Reason
		}
		if transition.SettledBy != 0 {
			settledBy = transition.SettledBy
		}
	}
	changed := false
	err = s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		result, err := s.db.Exec(ctx, `
			UPDATE challenge
			   SET state=$1, payload=$2, delivery=$3, attempts=$4, expires_at=$5, epoch=$6,
			       reason=$7, settled_at=$8, settled_by=$9
			 WHERE id=$10 AND chat_id=$11 AND user_id=$12 AND state=$13 AND epoch=$14`,
			transition.To, payload, delivery, transition.Record.Tries, transition.Record.Deadline,
			transition.Record.Epoch, reason, settledAt, settledBy, challengeID(transition.Expected),
			transition.Expected.GroupID, transition.Expected.UserID, transition.From, transition.Expected.Epoch)
		if err != nil {
			return fmt.Errorf("transition challenge for chat %d user %d from %s to %s: %w",
				transition.Expected.GroupID, transition.Expected.UserID, transition.From, transition.To, err)
		}
		changed, err = changedRow(result)
		if err != nil || !changed {
			return err
		}
		for _, action := range transition.Actions {
			if _, err = s.db.Exec(ctx, `
				INSERT INTO pending_action (
					id, challenge_id, kind, payload, next_try_at, claim_owner, claim_until
				) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				action.ID, challengeID(transition.Expected), action.Kind, action.Payload, action.NextTryAt,
				action.ClaimOwner, action.ClaimUntil); err != nil {
				return fmt.Errorf("enqueue %s for challenge %s: %w", action.Kind, challengeID(transition.Expected), err)
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return changed, nil
}

func (s *VerificationStore) ClaimExpired(_ string, now, claimUntil int64, limit int) ([]verification.PendingRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	if claimUntil <= now {
		return nil, fmt.Errorf("expiry claim ends at %d, not after %d", claimUntil, now)
	}
	var claimed []verification.PendingRecord
	err := s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		rows, err := s.db.Query(ctx, `
			SELECT chat_id, user_id, payload, delivery, attempts, expires_at, epoch
			  FROM challenge
			 WHERE state='pending' AND expires_at <= $1
			 ORDER BY expires_at, id
			 LIMIT $2`, now, limit)
		if err != nil {
			return fmt.Errorf("select due challenges: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			record, err := scanPending(rows)
			if err != nil {
				return err
			}
			if record.Epoch >= maxStoredEpoch {
				return fmt.Errorf("due challenge for chat %d user %d has exhausted epoch %d",
					record.GroupID, record.UserID, record.Epoch)
			}
			nextEpoch := record.Epoch + 1
			result, err := s.db.Exec(ctx, `
				UPDATE challenge
				   SET expires_at=$1, epoch=$2
				 WHERE id=$3 AND chat_id=$4 AND user_id=$5 AND state='pending'
				   AND expires_at <= $6 AND epoch=$7`,
				claimUntil, nextEpoch, challengeID(record.Ref()), record.GroupID, record.UserID, now, record.Epoch)
			if err != nil {
				return fmt.Errorf("claim due challenge for chat %d user %d: %w", record.GroupID, record.UserID, err)
			}
			changed, err := changedRow(result)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			record.Deadline = claimUntil
			record.Epoch = nextEpoch
			claimed = append(claimed, record)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate due challenges: %w", err)
		}
		return nil
	})
	return claimed, err
}

func (s *VerificationStore) ClaimActions(
	_ string,
	owner string,
	now, claimUntil int64,
	limit int,
) ([]verification.PendingAction, error) {
	if limit <= 0 {
		return nil, nil
	}
	if strings.TrimSpace(owner) == "" {
		return nil, fmt.Errorf("action claim owner is empty")
	}
	if claimUntil <= now {
		return nil, fmt.Errorf("action claim ends at %d, not after %d", claimUntil, now)
	}
	var claimed []verification.PendingAction
	err := s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		rows, err := s.db.Query(ctx, `
			SELECT id, challenge_id, kind, payload, attempts, next_try_at
			  FROM pending_action
			 WHERE state='pending' AND next_try_at <= $1
			   AND (claim_until IS NULL OR claim_until <= $1)
			 ORDER BY next_try_at, id
			 LIMIT $2`, now, limit)
		if err != nil {
			return fmt.Errorf("select ready actions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var action verification.PendingAction
			if err := rows.Scan(&action.ID, &action.ChallengeID, &action.Kind, &action.Payload,
				&action.Attempts, &action.NextTryAt); err != nil {
				return fmt.Errorf("scan pending action: %w", err)
			}
			result, err := s.db.Exec(ctx, `
				UPDATE pending_action
				   SET claim_owner=$1, claim_until=$2
				 WHERE id=$3 AND state='pending' AND next_try_at <= $4
				   AND (claim_until IS NULL OR claim_until <= $4)`, owner, claimUntil, action.ID, now)
			if err != nil {
				return fmt.Errorf("claim action %s: %w", action.ID, err)
			}
			changed, err := changedRow(result)
			if err != nil {
				return err
			}
			if changed {
				claimed = append(claimed, action)
			}
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate ready actions: %w", err)
		}
		return nil
	})
	return claimed, err
}

func (s *VerificationStore) CompleteAction(
	_ string,
	id, owner string,
	completedAt int64,
	followups []verification.ActionIntent,
) (bool, error) {
	if err := validateActionIntents(followups); err != nil {
		return false, err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return false, fmt.Errorf("action completion requires id and owner")
	}
	changed := false
	err := s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		result, err := s.db.Exec(ctx, `
			UPDATE pending_action
			   SET state='done', done_at=$1, claim_owner=NULL, claim_until=NULL
			 WHERE id=$2 AND state='pending' AND claim_owner=$3`, completedAt, id, owner)
		if err != nil {
			return fmt.Errorf("complete action %s: %w", id, err)
		}
		changed, err = changedRow(result)
		if err != nil || !changed {
			return err
		}
		for _, action := range followups {
			result, err = s.db.Exec(ctx, `
				INSERT INTO pending_action (id, challenge_id, kind, payload, next_try_at)
				SELECT $1, challenge_id, $2, $3, $4
				  FROM pending_action WHERE id=$5`,
				action.ID, action.Kind, action.Payload, action.NextTryAt, id)
			if err != nil {
				return fmt.Errorf("enqueue follow-up %s after action %s: %w", action.Kind, id, err)
			}
			inserted, err := changedRow(result)
			if err != nil {
				return err
			}
			if !inserted {
				return fmt.Errorf("action %s disappeared while completing it", id)
			}
		}
		return nil
	})
	return changed, err
}

func (s *VerificationStore) RetryAction(
	_ string,
	id, owner string,
	attempts int,
	nextTryAt int64,
	detail string,
) (bool, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" || attempts < 1 {
		return false, fmt.Errorf("invalid action retry")
	}
	result, err := s.db.Exec(context.Background(), `
		UPDATE pending_action
		   SET attempts=$1, next_try_at=$2, last_error=$3, claim_owner=NULL, claim_until=NULL
		 WHERE id=$4 AND state='pending' AND claim_owner=$5`, attempts, nextTryAt, detail, id, owner)
	if err != nil {
		return false, fmt.Errorf("retry action %s: %w", id, err)
	}
	return changedRow(result)
}

func (s *VerificationStore) FailAction(
	_ string,
	id, owner string,
	failedAt int64,
	detail string,
) (bool, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(owner) == "" {
		return false, fmt.Errorf("invalid action failure")
	}
	result, err := s.db.Exec(context.Background(), `
		UPDATE pending_action
		   SET state='failed', failed_at=$1, last_error=$2, claim_owner=NULL, claim_until=NULL
		 WHERE id=$3 AND state='pending' AND claim_owner=$4`, failedAt, detail, id, owner)
	if err != nil {
		return false, fmt.Errorf("fail action %s: %w", id, err)
	}
	return changedRow(result)
}

func validateActionIntents(actions []verification.ActionIntent) error {
	ids := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if strings.TrimSpace(action.ID) == "" || strings.TrimSpace(action.Kind) == "" {
			return fmt.Errorf("action id and kind are required")
		}
		if action.ClaimOwner == "" && action.ClaimUntil != 0 {
			return fmt.Errorf("action %q has a lease expiry without an owner", action.ID)
		}
		if action.ClaimOwner != "" && action.ClaimUntil <= action.NextTryAt {
			return fmt.Errorf("action %q lease does not outlast its first attempt", action.ID)
		}
		if _, exists := ids[action.ID]; exists {
			return fmt.Errorf("duplicate action id %q", action.ID)
		}
		ids[action.ID] = struct{}{}
	}
	return nil
}
