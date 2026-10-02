package database

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func (s *VerificationStore) SupersedeGroupSettlements(_ string, groupID, at int64) ([]verification.PendingRecord, error) {
	var records []verification.PendingRecord
	err := s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		rows, err := s.db.Query(ctx, `
			UPDATE challenge SET state='superseded', reason=NULL, settled_at=$1, settled_by=NULL
			 WHERE chat_id=$2 AND state IN ('approved', 'declined', 'banned', 'expired')
			   AND EXISTS (
			       SELECT 1 FROM pending_action
			        WHERE challenge_id=challenge.id AND state='pending'
			          AND kind IN ('settle_approve', 'settle_decline', 'settle_ban')
			   )
			RETURNING chat_id, user_id, payload, delivery, attempts, expires_at, epoch`, at, groupID)
		if err != nil {
			return fmt.Errorf("supersede unsettled challenges for chat %d: %w", groupID, err)
		}
		defer rows.Close()
		for rows.Next() {
			record, err := scanPending(rows)
			if err != nil {
				return err
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, record := range records {
			if err := s.enqueueCancellationActions(ctx, record, at); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *VerificationStore) SettlementActionCurrent(
	_ string, id, owner string, expected verification.PendingRef, state verification.ChallengeState,
) (bool, error) {
	var current bool
	err := s.db.QueryRow(context.Background(), `
		SELECT EXISTS (
		    SELECT 1 FROM pending_action AS action JOIN challenge ON challenge.id=action.challenge_id
		     WHERE action.id=$1 AND action.state='pending' AND action.claim_owner=$2
		       AND challenge.id=$3 AND challenge.chat_id=$4 AND challenge.user_id=$5
		       AND challenge.epoch=$6 AND challenge.state=$7
		)`, id, owner, challengeID(expected), expected.GroupID, expected.UserID, expected.Epoch, state).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("check settlement action %s: %w", id, err)
	}
	return current, nil
}

func (s *VerificationStore) LoadRecentPasses(_ string, since, until int64) ([]verification.RecentPassRecord, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT challenge.chat_id, challenge.user_id, action.done_at, challenge.payload
		  FROM challenge JOIN pending_action AS action ON action.challenge_id=challenge.id
		 WHERE challenge.state='approved' AND action.kind='settle_approve' AND action.state='done'
		   AND action.done_at >= $1 AND action.done_at <= $2`, since, until)
	if err != nil {
		return nil, fmt.Errorf("load recent passes: %w", err)
	}
	defer rows.Close()
	latest := make(map[[2]int64]verification.RecentPassRecord)
	for rows.Next() {
		var record verification.RecentPassRecord
		var payload string
		if err := rows.Scan(&record.GroupID, &record.UserID, &record.PassedAt, &payload); err != nil {
			return nil, fmt.Errorf("scan recent pass: %w", err)
		}
		var pending verification.PendingRecord
		if err := json.Unmarshal([]byte(payload), &pending); err != nil {
			return nil, fmt.Errorf("decode recent pass: %w", err)
		}
		// An unheld member was already admitted; their answer grants no new admission.
		if pending.Gate == "mute" && !pending.Held {
			continue
		}
		key := [2]int64{record.GroupID, record.UserID}
		if previous, exists := latest[key]; !exists || record.PassedAt > previous.PassedAt {
			latest[key] = record
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records := make([]verification.RecentPassRecord, 0, len(latest))
	for _, record := range latest {
		records = append(records, record)
	}
	return records, nil
}
