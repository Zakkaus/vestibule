package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

var _ verification.WebStore = (*VerificationStore)(nil)

func challengeKind(mode string) string {
	if settings.IsWebMode(mode) {
		return mode
	}
	return "rule"
}
func webClaimHash(t verification.ChallengeTransition) string {
	if t.WebClaim == nil {
		return ""
	}
	return t.WebClaim.TokenHash
}
func webClaimNow(t verification.ChallengeTransition) int64 {
	if t.WebClaim == nil {
		return 0
	}
	return t.WebClaim.Now
}

// IssueWebToken also rotates on explicit resend; reads and restore never call it.
func (s *VerificationStore) IssueWebToken(ctx context.Context, ref verification.PendingRef, token verification.WebTokenRecord) (bool, error) {
	if token.TokenHash == "" {
		return false, fmt.Errorf("empty web token hash")
	}
	result, err := s.db.Exec(ctx, `
 INSERT INTO verify_tokens (challenge_id, token_hash, salt, pow_bits, issued_at)
 SELECT id, $1, $2, $3, $4 FROM challenge
 WHERE id=$5 AND chat_id=$7 AND user_id=$8 AND state='pending' AND epoch=$6 AND expires_at>$4 AND kind IN ('pow','captcha')
 ON CONFLICT(challenge_id) DO UPDATE SET token_hash=excluded.token_hash, salt=excluded.salt,
 pow_bits=excluded.pow_bits, issued_at=excluded.issued_at
 WHERE EXISTS (SELECT 1 FROM challenge WHERE id=$5 AND chat_id=$7 AND user_id=$8)`,
		token.TokenHash, token.Salt[:], token.PoWBits, token.IssuedAt, challengeID(ref), ref.Epoch, ref.GroupID, ref.UserID)
	if err != nil {
		return false, fmt.Errorf("issue web token: %w", err)
	}
	return changedRow(result)
}

func (s *VerificationStore) LoadWebToken(ctx context.Context, id, hash string, now int64) (verification.WebTokenRecord, bool, error) {
	record, valid, err := s.ResolveWebToken(ctx, hash, now)
	return record, valid && record.ChallengeID == id, err
}

func (s *VerificationStore) ResolveWebToken(ctx context.Context, hash string, now int64) (verification.WebTokenRecord, bool, error) {
	var record verification.WebTokenRecord
	var salt []byte
	err := s.db.QueryRow(ctx, `SELECT challenge_id, token_hash, salt, pow_bits, issued_at FROM verify_tokens
 JOIN challenge ON challenge.id=verify_tokens.challenge_id
 WHERE token_hash=$1 AND state='pending' AND expires_at>$2
 AND kind IN ('pow','captcha')`, hash, now).Scan(&record.ChallengeID, &record.TokenHash, &salt, &record.PoWBits, &record.IssuedAt)
	if err == sql.ErrNoRows {
		return record, false, nil
	}
	if err != nil {
		return record, false, fmt.Errorf("load web token: %w", err)
	}
	if len(salt) != len(record.Salt) {
		return record, false, fmt.Errorf("invalid stored PoW salt")
	}
	copy(record.Salt[:], salt)
	return record, true, nil
}

// FallbackWeb changes the live row and its statistics kind without releasing gate ownership.
func (s *VerificationStore) FallbackWeb(ctx context.Context, ref verification.PendingRef, record verification.PendingRecord, claim *verification.WebClaim) (bool, error) {
	if err := validatePendingReplacement(ref, record); err != nil {
		return false, err
	}
	if record.Mode != settings.ModeQuiz && record.Mode != settings.ModeKernel {
		return false, fmt.Errorf("invalid web fallback mode")
	}
	payload, delivery, err := encodePending(record)
	if err != nil {
		return false, err
	}
	changed := false
	guard := verification.ChallengeTransition{WebClaim: claim}
	err = s.db.DoTxn(ctx, nil, func(ctx context.Context) error {
		result, err := s.db.Exec(ctx, `UPDATE challenge SET kind='rule', payload=$1, delivery=$2,
  expires_at=$3, epoch=$4 WHERE id=$5 AND state='pending' AND epoch=$6 AND kind IN ('pow','captcha')
  AND ($7='' OR (expires_at>$8 AND EXISTS (SELECT 1 FROM verify_tokens WHERE challenge_id=challenge.id AND token_hash=$7)))`,
			payload, delivery, record.Deadline, record.Epoch, challengeID(ref), ref.Epoch, webClaimHash(guard), webClaimNow(guard))
		if err != nil {
			return err
		}
		changed, err = changedRow(result)
		if err != nil || !changed {
			return err
		}
		_, err = s.db.Exec(ctx, `DELETE FROM verify_tokens WHERE challenge_id=$1`, challengeID(ref))
		return err
	})
	return changed, err
}
