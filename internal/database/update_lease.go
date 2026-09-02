package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// UpdatePollLease coordinates the one Telegram update stream accepted for a bot token.
type UpdatePollLease struct {
	db *Database
}

func NewUpdatePollLease(db *Database) *UpdatePollLease {
	if db == nil {
		panic("database: update poll lease requires a database")
	}
	return &UpdatePollLease{db: db}
}

// Acquire takes the singleton lease only after its previous holder has expired.
func (l *UpdatePollLease) Acquire(ctx context.Context, holder string, now, expiresAt int64) (bool, error) {
	if err := validateLease(holder, now, expiresAt); err != nil {
		return false, err
	}
	result, err := l.db.Exec(ctx, `
		INSERT INTO update_poll_lease (singleton, holder, expires_at)
		VALUES (1, $1, $2)
		ON CONFLICT (singleton) DO UPDATE
		   SET holder=excluded.holder, expires_at=excluded.expires_at
		 WHERE update_poll_lease.expires_at <= $3`, holder, expiresAt, now)
	if err != nil {
		return false, fmt.Errorf("acquire update polling lease: %w", err)
	}
	return changedRow(result)
}

// Holder reports the unexpired lease holder, if there is one. An empty name means no
// instance is polling Telegram against this database right now, which is what a migration
// has to establish before it replaces the state a running instance is using.
func (l *UpdatePollLease) Holder(ctx context.Context, now int64) (string, error) {
	var holder string
	err := l.db.QueryRow(ctx,
		`SELECT holder FROM update_poll_lease WHERE singleton = 1 AND expires_at > $1`,
		now).Scan(&holder)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read update polling lease: %w", err)
	}
	return holder, nil
}

// Renew extends only the lease still held by this process. False means ownership was lost.
func (l *UpdatePollLease) Renew(ctx context.Context, holder string, now, expiresAt int64) (bool, error) {
	if err := validateLease(holder, now, expiresAt); err != nil {
		return false, err
	}
	result, err := l.db.Exec(ctx, `
		UPDATE update_poll_lease
		   SET expires_at=$1
		 WHERE singleton=1 AND holder=$2 AND expires_at > $3`, expiresAt, holder, now)
	if err != nil {
		return false, fmt.Errorf("renew update polling lease: %w", err)
	}
	return changedRow(result)
}

// Release deletes only this holder's lease, so a stale shutdown cannot release a successor.
func (l *UpdatePollLease) Release(ctx context.Context, holder string) error {
	if strings.TrimSpace(holder) == "" {
		return fmt.Errorf("update polling lease holder is empty")
	}
	if _, err := l.db.Exec(ctx, "DELETE FROM update_poll_lease WHERE singleton=1 AND holder=$1", holder); err != nil {
		return fmt.Errorf("release update polling lease: %w", err)
	}
	return nil
}

func validateLease(holder string, now, expiresAt int64) error {
	if strings.TrimSpace(holder) == "" {
		return fmt.Errorf("update polling lease holder is empty")
	}
	if expiresAt <= now {
		return fmt.Errorf("update polling lease expiry %d is not after %d", expiresAt, now)
	}
	return nil
}
