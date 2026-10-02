package api

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/rules"
)

type ruleFieldError struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

func validateAutoReplyDefinitions(writer http.ResponseWriter, expected, records []rules.Record, prefix string) bool {
	previous := make(map[string]rules.Record, len(expected))
	for _, record := range expected {
		previous[record.ID] = record
	}
	fields := make([]ruleFieldError, 0)
	for index, record := range records {
		if record.Collection != rules.AutoReplyCollection {
			continue
		}
		old, exists := previous[record.ID]
		// The store's conditional write verifies expected against the persisted definition.
		if exists && old.Collection == record.Collection && bytes.Equal(old.Definition, record.Definition) {
			continue
		}
		if _, err := rules.DecodeAutoReply(record.Definition); err != nil {
			field := err.(*rules.DefinitionError)
			name := prefix
			if prefix == "items" {
				name = fmt.Sprintf("items[%d]", index)
			}
			name += ".definition"
			if field.Field != "definition" {
				name += "." + field.Field
			}
			fields = append(fields, ruleFieldError{Name: name, Code: field.Code})
		}
	}
	if len(fields) == 0 {
		return true
	}
	writeJSON(writer, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "invalid_rule"}, "fields": fields})
	return false
}
