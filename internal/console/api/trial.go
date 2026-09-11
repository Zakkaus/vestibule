package api

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type trialRequest struct {
	Collection       string  `json:"collection"`
	QuestionIndex    *int    `json:"question_index"`
	ExpectedRevision *uint64 `json:"expected_revision"`
	Choice           *int    `json:"choice"`
	Answer           *string `json:"answer"`
	present          map[string]json.RawMessage
}

// UnmarshalJSON records field presence so null and absent values cannot be confused.
func (r *trialRequest) UnmarshalJSON(data []byte) error {
	type wire trialRequest
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = trialRequest(decoded)
	r.present = fields
	return nil
}

func (r trialRequest) has(name string) bool {
	_, ok := r.present[name]
	return ok
}

func (r trialRequest) valid() bool {
	if r.Collection != "questions" && r.Collection != "fallback_questions" {
		return false
	}
	if r.QuestionIndex == nil || *r.QuestionIndex < 0 || r.ExpectedRevision == nil {
		return false
	}
	if r.Collection == "questions" {
		return r.has("choice") && r.Choice != nil && !r.has("answer")
	}
	return r.has("answer") && r.Answer != nil && !r.has("choice")
}

func (s *Server) testRule(writer http.ResponseWriter, request *http.Request, chatID int64) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	var input trialRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if !input.valid() {
		writeError(writer, http.StatusBadRequest, "invalid_rule")
		return
	}
	group, ok := s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	if group.Revision() != *input.ExpectedRevision {
		writeError(writer, http.StatusConflict, "settings_conflict")
		return
	}

	correct, err := trialAnswer(group, input)
	if err != nil {
		writeRulesError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]bool{"correct": correct})
}

func trialAnswer(group settings.GroupView, input trialRequest) (bool, error) {
	index := *input.QuestionIndex
	if input.Collection == "questions" {
		questions := group.Questions().Value
		if index >= len(questions) {
			return false, rules.ErrRuleNotFound
		}
		question := questions[index]
		if *input.Choice < 0 || *input.Choice >= len(question.Options) {
			return false, rules.ErrRuleInvalid
		}
		return verification.QuizAnswerMatches(*input.Choice, question.Answer), nil
	}

	questions := group.FallbackQuestions().Value
	if index >= len(questions) {
		return false, rules.ErrRuleNotFound
	}
	question := questions[index]
	return (rules.OneOf{Values: question.Answers}).MatchesAnswer(*input.Answer), nil
}
