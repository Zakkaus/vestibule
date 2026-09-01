package rules

import (
	"encoding/json"
	"errors"
)

var (
	// ErrRuleInvalid reports a structurally invalid stored rule or definition.
	ErrRuleInvalid = errors.New("invalid rule")
	// ErrRuleConflict reports that the expected rule snapshot is no longer current.
	ErrRuleConflict = errors.New("rule changed before update")
	// ErrRuleNotFound reports that an individual rule does not exist in the requested chat.
	ErrRuleNotFound = errors.New("rule not found")
	// ErrRuleChatNotFound reports that the requested chat has no persisted settings row.
	ErrRuleChatNotFound = errors.New("rule chat not found")
)

// Record is one ordered, independently enabled rule persisted for a chat collection.
type Record struct {
	ID         string
	ChatID     int64
	Collection string
	Ordinal    int
	Enabled    bool
	Definition json.RawMessage
}
