package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

const maxSafeJSONInteger = 1<<53 - 1

type ownerLimitsService interface {
	SettingsService
	Registrations() settings.RegistrationState
	OwnerLimits() settings.OwnerLimitsState
	UpdateOwnerLimits(uint64, settings.LimitChanges) (settings.OwnerLimitsState, error)
}

func (s *Server) sessionResponse(grant auth.Grant) sessionResponse {
	response := newSessionResponse(grant)
	registrations, ok := s.settings.(interface {
		Registrations() settings.RegistrationState
	})
	if ok {
		ownerID := registrations.Registrations().OwnerID
		response.IsOwner = ownerID != 0 && ownerID == grant.Session.Principal.TelegramID
	}
	return response
}

type ownerLimitsPatchRequest struct {
	ExpectedRevision *uint64                    `json:"expected_revision"`
	Changes          map[string]json.RawMessage `json:"changes"`
}

type ownerLimitsResponse struct {
	Revision   uint64                        `json:"revision"`
	Limits     settings.OwnerLimits          `json:"limits"`
	Violations []ownerLimitViolationResponse `json:"violations"`
}

type ownerLimitViolationResponse struct {
	ChatID string `json:"chat_id"`
	Field  string `json:"field"`
	Value  int64  `json:"value"`
	Limit  int64  `json:"limit"`
}

func (s *Server) ownerLimitsRoute(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodPatch {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	service, ok := s.ownerSettings(writer)
	if !ok {
		return
	}
	session, ok := s.ownerSession(writer, request, service)
	if !ok {
		return
	}
	switch request.Method {
	case http.MethodGet:
		s.readOwnerLimits(writer, service)
	case http.MethodPatch:
		s.patchOwnerLimits(writer, request, session, service)
	}
}

func (s *Server) ownerSettings(writer http.ResponseWriter) (ownerLimitsService, bool) {
	service, ok := s.settings.(ownerLimitsService)
	if !ok || service == nil {
		writeError(writer, http.StatusServiceUnavailable, "settings_unavailable")
		return nil, false
	}
	return service, true
}

func (s *Server) ownerSession(writer http.ResponseWriter, request *http.Request, service ownerLimitsService) (auth.Session, bool) {
	session, ok := s.session(writer, request)
	if !ok {
		return auth.Session{}, false
	}
	ownerID := service.Registrations().OwnerID
	if ownerID == 0 || session.Principal.TelegramID != ownerID {
		writeError(writer, http.StatusForbidden, "owner_required")
		return auth.Session{}, false
	}
	return session, true
}

func (s *Server) readOwnerLimits(writer http.ResponseWriter, service ownerLimitsService) {
	state := service.OwnerLimits()
	writeJSON(writer, http.StatusOK, ownerLimitsResponse{
		Revision:   state.Revision,
		Limits:     state.Limits,
		Violations: ownerLimitViolationViews(state.Violations),
	})
}

func (s *Server) patchOwnerLimits(
	writer http.ResponseWriter,
	request *http.Request,
	session auth.Session,
	service ownerLimitsService,
) {
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	var input ownerLimitsPatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.ExpectedRevision == nil || *input.ExpectedRevision > maxSafeJSONInteger || input.Changes == nil {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	changes, err := decodeOwnerLimitChanges(input.Changes)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	state, err := service.UpdateOwnerLimits(*input.ExpectedRevision, changes)
	if err != nil {
		writeOwnerLimitsError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, ownerLimitsResponse{
		Revision:   state.Revision,
		Limits:     state.Limits,
		Violations: ownerLimitViolationViews(state.Violations),
	})
}

func decodeOwnerLimitChanges(input map[string]json.RawMessage) (settings.LimitChanges, error) {
	changes := make(settings.LimitChanges, len(input))
	for field, raw := range input {
		token := strings.TrimSpace(string(raw))
		if token == "null" {
			changes[field] = nil
			continue
		}
		if token == "" {
			return nil, errors.New("owner limit values must be JSON integers or null")
		}
		value, err := strconv.ParseInt(token, 10, 64)
		if err != nil || value < 0 || value > maxSafeJSONInteger {
			return nil, errors.New("owner limit values must be non-negative safe integers")
		}
		changes[field] = &value
	}
	return changes, nil
}

func ownerLimitViolationViews(violations []settings.LimitViolation) []ownerLimitViolationResponse {
	views := make([]ownerLimitViolationResponse, 0, len(violations))
	for _, violation := range violations {
		views = append(views, ownerLimitViolationResponse{
			ChatID: strconv.FormatInt(violation.ChatID, 10),
			Field:  violation.Field,
			Value:  violation.Value,
			Limit:  violation.Limit,
		})
	}
	return views
}

func writeOwnerLimitsError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, settings.ErrSettingsConflict):
		writeError(writer, http.StatusConflict, "settings_conflict")
	case errors.Is(err, settings.ErrOwnerLimitsInvalid):
		writeError(writer, http.StatusBadRequest, "invalid_request")
	default:
		writeError(writer, http.StatusServiceUnavailable, "settings_unavailable")
	}
}
