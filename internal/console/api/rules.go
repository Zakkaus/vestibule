package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/rules"
)

// RulesService is the ordered rule store required by the HTTP adapter.
type RulesService interface {
	ListRules(context.Context, int64, string) ([]rules.Record, error)
	ReplaceRules(context.Context, int64, string, []rules.Record, []rules.Record) ([]rules.Record, bool, error)
	UpdateRule(context.Context, int64, rules.Record, rules.Record) (rules.Record, bool, error)
}

type ruleResponse struct {
	ID         string          `json:"id"`
	Collection string          `json:"collection"`
	Ordinal    int             `json:"ordinal"`
	Enabled    bool            `json:"enabled"`
	Definition json.RawMessage `json:"definition"`
}

func ruleView(record rules.Record) ruleResponse {
	return ruleResponse{
		ID: record.ID, Collection: record.Collection, Ordinal: record.Ordinal,
		Enabled: record.Enabled, Definition: record.Definition,
	}
}

type ruleInput struct {
	ID         string          `json:"id"`
	Enabled    bool            `json:"enabled"`
	Definition json.RawMessage `json:"definition"`
}

type replaceRulesRequest struct {
	Collection string       `json:"collection"`
	Expected   *[]ruleInput `json:"expected"`
	Items      *[]ruleInput `json:"items"`
}

type ruleStateInput struct {
	Collection string          `json:"collection"`
	Ordinal    int             `json:"ordinal"`
	Enabled    bool            `json:"enabled"`
	Definition json.RawMessage `json:"definition"`
}

type updateRuleRequest struct {
	Expected *ruleStateInput `json:"expected"`
	Item     *ruleStateInput `json:"item"`
}

func (s *Server) rulesRoute(writer http.ResponseWriter, request *http.Request, chatID int64, rest []string) {
	switch {
	case request.Method == http.MethodGet && len(rest) == 0:
		s.readRules(writer, request, chatID)
	case request.Method == http.MethodPut && len(rest) == 0:
		s.replaceRules(writer, request, chatID)
	case request.Method == http.MethodPut && len(rest) == 1 && rest[0] != "":
		s.updateRule(writer, request, chatID, rest[0])
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) readRules(writer http.ResponseWriter, request *http.Request, chatID int64) {
	if _, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess); !ok {
		return
	}
	if s.rules == nil {
		writeError(writer, http.StatusServiceUnavailable, "rules_unavailable")
		return
	}
	records, err := s.rules.ListRules(request.Context(), chatID, request.URL.Query().Get("collection"))
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "rules_unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": ruleViews(records)})
}

func (s *Server) replaceRules(writer http.ResponseWriter, request *http.Request, chatID int64) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	if s.rules == nil {
		writeError(writer, http.StatusServiceUnavailable, "rules_unavailable")
		return
	}
	var input replaceRulesRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.Expected == nil || input.Items == nil {
		writeError(writer, http.StatusBadRequest, "invalid_rule")
		return
	}
	expected := ruleRecords(chatID, input.Collection, *input.Expected)
	next := ruleRecords(chatID, input.Collection, *input.Items)
	records, _, err := s.rules.ReplaceRules(request.Context(), chatID, input.Collection, expected, next)
	if err != nil {
		writeRulesError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": ruleViews(records)})
}

func (s *Server) updateRule(
	writer http.ResponseWriter,
	request *http.Request,
	chatID int64,
	ruleID string,
) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	if s.rules == nil {
		writeError(writer, http.StatusServiceUnavailable, "rules_unavailable")
		return
	}
	var input updateRuleRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.Expected == nil || input.Item == nil {
		writeError(writer, http.StatusBadRequest, "invalid_rule")
		return
	}
	expected := input.Expected.record(chatID, ruleID)
	next := input.Item.record(chatID, ruleID)
	record, _, err := s.rules.UpdateRule(request.Context(), chatID, expected, next)
	if err != nil {
		writeRulesError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, ruleView(record))
}

func (input ruleStateInput) record(chatID int64, ruleID string) rules.Record {
	return rules.Record{
		ID: ruleID, ChatID: chatID, Collection: input.Collection, Ordinal: input.Ordinal,
		Enabled: input.Enabled, Definition: input.Definition,
	}
}

func ruleRecords(chatID int64, collection string, inputs []ruleInput) []rules.Record {
	records := make([]rules.Record, len(inputs))
	for ordinal, input := range inputs {
		records[ordinal] = rules.Record{
			ID: input.ID, ChatID: chatID, Collection: collection, Ordinal: ordinal,
			Enabled: input.Enabled, Definition: input.Definition,
		}
	}
	return records
}

func ruleViews(records []rules.Record) []ruleResponse {
	items := make([]ruleResponse, len(records))
	for index, record := range records {
		items[index] = ruleView(record)
	}
	return items
}

func writeRulesError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, rules.ErrRuleInvalid):
		writeError(writer, http.StatusBadRequest, "invalid_rule")
	case errors.Is(err, rules.ErrRuleConflict):
		writeError(writer, http.StatusConflict, "rule_conflict")
	case errors.Is(err, rules.ErrRuleNotFound):
		writeError(writer, http.StatusNotFound, "rule_not_found")
	case errors.Is(err, rules.ErrRuleChatNotFound):
		writeError(writer, http.StatusNotFound, "chat_not_found")
	default:
		writeError(writer, http.StatusServiceUnavailable, "rules_unavailable")
	}
}
