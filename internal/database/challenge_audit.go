package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// LoadChallengeAudit returns terminal challenges newest first. It does not project pending rows
// into history before their compare-and-set settlement has completed.
func (s *VerificationStore) LoadChallengeAudit(ctx context.Context, chatID int64) ([]verification.ChallengeAuditRecord, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("audit chat ID is required")
	}
	rows, err := s.db.Query(ctx, `
		SELECT challenge.id, challenge.chat_id, challenge.user_id, challenge.payload,
		       challenge.state, challenge.reason, challenge.settled_at, challenge.settled_by,
		       COALESCE(settlement.state, ''), COALESCE(undo.state, '')
		  FROM challenge
		  LEFT JOIN pending_action AS settlement
		    ON settlement.challenge_id=challenge.id
		   AND settlement.kind IN ('settle_approve', 'settle_decline', 'settle_ban')
		  LEFT JOIN pending_action AS undo
		    ON undo.challenge_id=challenge.id AND undo.kind='undo_ban'
		 WHERE challenge.chat_id=$1 AND challenge.state<>'pending' AND challenge.settled_at IS NOT NULL
		 ORDER BY challenge.settled_at DESC, challenge.id DESC`, chatID)
	if err != nil {
		return nil, fmt.Errorf("load challenge audit for chat %d: %w", chatID, err)
	}
	defer rows.Close()

	var records []verification.ChallengeAuditRecord
	for rows.Next() {
		record, scanErr := scanChallengeAudit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate challenge audit for chat %d: %w", chatID, err)
	}
	return records, nil
}

func scanChallengeAudit(row interface{ Scan(...any) error }) (verification.ChallengeAuditRecord, error) {
	var (
		record           verification.ChallengeAuditRecord
		payload          string
		state            string
		reason           sql.NullString
		settledBy        sql.NullInt64
		settlementAction string
		undoAction       string
	)
	if err := row.Scan(
		&record.ID, &record.Record.GroupID, &record.Record.UserID, &payload,
		&state, &reason, &record.SettledAt, &settledBy, &settlementAction, &undoAction,
	); err != nil {
		return record, fmt.Errorf("scan challenge audit: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &record.Record); err != nil {
		return record, fmt.Errorf("decode challenge audit %s: %w", record.ID, err)
	}
	if record.ID != challengeID(record.Record.Ref()) {
		return record, fmt.Errorf("challenge audit %s payload identity does not match", record.ID)
	}
	record.State = verification.ChallengeState(state)
	if !validTerminalChallengeState(record.State) {
		return record, fmt.Errorf("challenge audit %s has invalid terminal state %q", record.ID, state)
	}
	if reason.Valid {
		record.Reason = reason.String
	}
	if !validAuditReason(record.State, record.Reason) {
		return record, fmt.Errorf("challenge audit %s has invalid reason %q for %s", record.ID, record.Reason, record.State)
	}
	if record.SettledAt <= 0 {
		return record, fmt.Errorf("challenge audit %s has invalid settlement time %d", record.ID, record.SettledAt)
	}
	if settledBy.Valid {
		record.SettledBy = settledBy.Int64
	}
	record.SettlementAction = verification.ChallengeActionState(settlementAction)
	record.UndoAction = verification.ChallengeActionState(undoAction)
	if !validChallengeActionState(record.SettlementAction) || !validChallengeActionState(record.UndoAction) {
		return record, fmt.Errorf("challenge audit %s has invalid action states %q and %q",
			record.ID, settlementAction, undoAction)
	}
	return record, nil
}

func validTerminalChallengeState(state verification.ChallengeState) bool {
	switch state {
	case verification.ChallengeApproved, verification.ChallengeDeclined, verification.ChallengeBanned,
		verification.ChallengeExpired, verification.ChallengeSuperseded:
		return true
	default:
		return false
	}
}

func validAuditReason(state verification.ChallengeState, reason string) bool {
	if state != verification.ChallengeDeclined {
		return reason == ""
	}
	return reason == "wrong_answer" || reason == "rejected" || reason == "external_unmet"
}

func validChallengeActionState(state verification.ChallengeActionState) bool {
	return state == verification.ChallengeActionNone || state == verification.ChallengeActionPending ||
		state == verification.ChallengeActionDone || state == verification.ChallengeActionFailed
}

// EnqueueChallengeUndo inserts one durable unban only while the same manual ban is still the
// latest recorded decision for the applicant and its original settlement action is complete.
func (s *VerificationStore) EnqueueChallengeUndo(
	ctx context.Context,
	expected verification.ChallengeAuditRecord,
	action verification.ActionIntent,
) (bool, error) {
	if expected.ID == "" || expected.State != verification.ChallengeBanned || expected.SettledAt <= 0 ||
		expected.SettledBy <= 0 || expected.Record.GroupID == 0 || expected.Record.UserID <= 0 ||
		expected.ID != challengeID(expected.Record.Ref()) {
		return false, fmt.Errorf("invalid challenge undo expectation")
	}
	if action.Kind != "undo_ban" {
		return false, fmt.Errorf("invalid challenge undo action kind %q", action.Kind)
	}
	if err := validateActionIntents([]verification.ActionIntent{action}); err != nil {
		return false, err
	}
	result, err := s.db.Exec(ctx, `
		INSERT INTO pending_action (
			id, challenge_id, kind, payload, next_try_at, claim_owner, claim_until
		)
		SELECT $1, challenge.id, $2, $3, $4, $5, $6
		  FROM challenge
		 WHERE challenge.id=$7 AND challenge.chat_id=$8 AND challenge.user_id=$9
		   AND challenge.state='banned' AND challenge.reason IS NULL
		   AND challenge.settled_at=$10 AND challenge.settled_by=$11 AND challenge.epoch=$12
		   AND NOT EXISTS (
		       SELECT 1 FROM challenge AS newer
		        WHERE newer.chat_id=challenge.chat_id AND newer.user_id=challenge.user_id
		          AND newer.id<>challenge.id AND newer.settled_at IS NOT NULL
		          AND newer.settled_at>=challenge.settled_at
		   )
		   AND NOT EXISTS (
		       SELECT 1 FROM pending_action AS settlement
		        WHERE settlement.challenge_id=challenge.id AND settlement.kind='settle_ban'
		          AND settlement.state<>'done'
		   )
		   AND NOT EXISTS (
		       SELECT 1 FROM pending_action AS prior_undo
		        WHERE prior_undo.challenge_id=challenge.id AND prior_undo.kind='undo_ban'
		   )
		ON CONFLICT DO NOTHING`,
		action.ID, action.Kind, action.Payload, action.NextTryAt, action.ClaimOwner, action.ClaimUntil,
		expected.ID, expected.Record.GroupID, expected.Record.UserID, expected.SettledAt,
		expected.SettledBy, expected.Record.Epoch)
	if err != nil {
		return false, fmt.Errorf("enqueue undo for challenge %s: %w", expected.ID, err)
	}
	return changedRow(result)
}
