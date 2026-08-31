package moderate

import "errors"

// WarningRecord is the durable form of one chat-user warning counter.
type WarningRecord struct {
	GroupID int64 `json:"group_id"`
	UserID  int64 `json:"user_id"`
	Count   int   `json:"count"`
}

// ErrWarningStoreReadOnly marks a legacy JSON path that cannot be safely overwritten.
var ErrWarningStoreReadOnly = errors.New("warning state is read-only")

// WarningStore persists complete snapshots while warning decisions remain in the service.
type WarningStore interface {
	LoadWarnings() ([]WarningRecord, error)
	SaveWarnings(func() []WarningRecord) error
}
