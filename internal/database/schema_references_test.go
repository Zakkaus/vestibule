package database

import (
	"context"
	"testing"
)

// Every table holding something that belongs to a group names the group by a foreign key, and
// the outbox names its challenge the same way. Enforcement is switched on, and
// TestOpenConfiguresSQLitePragmas proves it — but with two tables of its own, so nothing
// asserted that our tables carry the declaration. Dropping REFERENCES from all five left every
// test in the repository passing.
//
// The declaration is where a row naming a group this instance never registered stops. Without
// it the outbox would also accept an action for a challenge that does not exist, which no
// worker could ever settle because there is no state to settle.
func TestARowMustNameAParentThatExists(t *testing.T) {
	const (
		knownChat   = -1009000004401
		unknownChat = -1009000004402
	)
	for _, tc := range []struct {
		name       string
		statement  string
		absent     []any
		present    []any
		makeParent string
		parentArgs []any
	}{
		{
			name: "a challenge names its group",
			statement: `INSERT INTO challenge (id, chat_id, user_id, state, kind, payload, delivery, expires_at)
			            VALUES ($1, $2, 7, 'pending', 'quiz', '{}', 'group', 0)`,
			absent:  []any{"challenge-absent", int64(unknownChat)},
			present: []any{"challenge-present", int64(knownChat)},
		},
		{
			name: "a rule names its group",
			statement: `INSERT INTO rule (id, chat_id, collection, ordinal, definition)
			            VALUES ($1, $2, 'join', 0, '{}')`,
			absent:  []any{"rule-absent", int64(unknownChat)},
			present: []any{"rule-present", int64(knownChat)},
		},
		{
			name:      "a failure count names its group",
			statement: `INSERT INTO verification_failure (chat_id, user_id, count, last_at) VALUES ($1, $2, 1, 0)`,
			absent:    []any{int64(unknownChat), int64(11)},
			present:   []any{int64(knownChat), int64(11)},
		},
		{
			name:      "a warning count names its group",
			statement: `INSERT INTO warning_counter (chat_id, user_id, count) VALUES ($1, $2, 1)`,
			absent:    []any{int64(unknownChat), int64(12)},
			present:   []any{int64(knownChat), int64(12)},
		},
		{
			name: "an outbox action names its challenge",
			statement: `INSERT INTO pending_action (id, challenge_id, kind, payload, next_try_at)
			            VALUES ($1, $2, 'approve', '{}', 0)`,
			absent:  []any{"action-absent", "no-such-challenge"},
			present: []any{"action-present", "parent-challenge"},
			makeParent: `INSERT INTO challenge (id, chat_id, user_id, state, kind, payload, delivery, expires_at)
			             VALUES ('parent-challenge', $1, 9, 'pending', 'quiz', '{}', 'group', 0)`,
			parentArgs: []any{int64(knownChat)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := Open(ctx, testSQLiteConfig(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err = db.Exec(ctx, "INSERT INTO chat (id, title) VALUES ($1, 'known')", int64(knownChat)); err != nil {
				t.Fatal(err)
			}
			if tc.makeParent != "" {
				if _, err = db.Exec(ctx, tc.makeParent, tc.parentArgs...); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec(ctx, tc.statement, tc.absent...); err == nil {
				t.Fatal("the row was accepted although its parent does not exist")
			}
			// The same statement against a parent that does exist has to succeed, or the
			// rejection above would prove nothing more than a malformed statement.
			if _, err = db.Exec(ctx, tc.statement, tc.present...); err != nil {
				t.Fatalf("the same row with a parent that exists was rejected: %v", err)
			}
		})
	}
}
