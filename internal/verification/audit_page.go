package verification

import (
	"encoding/base64"
	"encoding/json"
)

const (
	DefaultAuditPageSize = 50
	MaxAuditPageSize     = 100
)

// AuditPageRequest selects decisions strictly older than an opaque, group-bound cursor.
type AuditPageRequest struct {
	Limit  int
	Cursor string
}

type auditCursor struct {
	GroupID   int64  `json:"g"`
	SettledAt int64  `json:"t"`
	ID        string `json:"i"`
}

// Boundary validates the page size and cursor before a store query.
func (p AuditPageRequest) Boundary(groupID int64) (limit int, at int64, id string, err error) {
	limit = p.Limit
	if limit == 0 {
		limit = DefaultAuditPageSize
	}
	if groupID == 0 || limit < 1 || limit > MaxAuditPageSize || len(p.Cursor) > 2048 {
		return 0, 0, "", ErrConsoleAuditInvalid
	}
	if p.Cursor == "" {
		return limit, 0, "", nil
	}
	data, decodeErr := base64.RawURLEncoding.DecodeString(p.Cursor)
	var cursor auditCursor
	if decodeErr != nil || json.Unmarshal(data, &cursor) != nil || cursor.GroupID != groupID || cursor.SettledAt <= 0 || cursor.ID == "" {
		return 0, 0, "", ErrConsoleAuditInvalid
	}
	return limit, cursor.SettledAt, cursor.ID, nil
}

// AuditNextCursor identifies the last visible row, not the lookahead row.
func AuditNextCursor(groupID int64, entry ConsoleAuditEntry) string {
	data, _ := json.Marshal(auditCursor{GroupID: groupID, SettledAt: entry.SettledAt.Unix(), ID: entry.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}
