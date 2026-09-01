package verification

import (
	"context"
	"time"
)

// ChallengeActionState is the durable state of a settlement or undo side effect.
type ChallengeActionState string

const (
	ChallengeActionNone    ChallengeActionState = ""
	ChallengeActionPending ChallengeActionState = "pending"
	ChallengeActionDone    ChallengeActionState = "done"
	ChallengeActionFailed  ChallengeActionState = "failed"
)

// ChallengeAuditRecord is the storage-side projection of one terminal challenge.
type ChallengeAuditRecord struct {
	ID               string
	Record           PendingRecord
	State            ChallengeState
	Reason           string
	SettledAt        int64
	SettledBy        int64
	SettlementAction ChallengeActionState
	UndoAction       ChallengeActionState
}

// challengeAuditStore keeps audit reads and undo insertion at the database boundary.
// EnqueueChallengeUndo must compare every identity and settlement field in expected,
// reject a superseded decision, and insert the action at most once.
type challengeAuditStore interface {
	LoadChallengeAudit(context.Context, int64) ([]ChallengeAuditRecord, error)
	EnqueueChallengeUndo(context.Context, ChallengeAuditRecord, ActionIntent) (bool, error)
}

// ConsoleUndoState is the complete UI-facing lifecycle of an audit undo.
type ConsoleUndoState string

const (
	ConsoleUndoUnavailable ConsoleUndoState = "unavailable"
	ConsoleUndoAvailable   ConsoleUndoState = "available"
	ConsoleUndoPending     ConsoleUndoState = "pending"
	ConsoleUndoCompleted   ConsoleUndoState = "completed"
	ConsoleUndoFailed      ConsoleUndoState = "failed"
)

// ConsoleAuditEntry is one settled challenge exposed to the console adapter.
type ConsoleAuditEntry struct {
	ID        string
	GroupID   int64
	UserID    int64
	Name      string
	State     ChallengeState
	Reason    string
	SettledAt time.Time
	SettledBy int64
	UndoState ConsoleUndoState
}

// ConsoleAuditUndo identifies the exact decision and actor requesting reversal.
type ConsoleAuditUndo struct {
	ID      string
	GroupID int64
	ActorID int64
}
