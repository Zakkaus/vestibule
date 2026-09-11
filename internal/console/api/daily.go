package api

import (
	"context"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
)

// DailyService supplies the persistent owner daily switch to the HTTP adapter.
type DailyService interface {
	Status(context.Context) (status.DailyStatus, error)
	SetEnabled(context.Context, bool) error
}

type dailyResponse struct {
	Enabled  bool   `json:"enabled"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

type dailyPatchRequest struct {
	Enabled *bool `json:"enabled"`
}

func (s *Server) dailyRoute(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "diagnostics_access_denied")
		return
	}
	if s.daily == nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable")
		return
	}
	var input dailyPatchRequest
	if request.Method == http.MethodPatch {
		if err := s.authenticator.ValidateCSRF(request, session); err != nil {
			writeError(writer, http.StatusForbidden, "csrf_invalid")
			return
		}
		if !decodeJSON(writer, request, &input) {
			return
		}
		if input.Enabled == nil {
			writeError(writer, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	current, err := s.daily.Status(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable")
		return
	}
	if input.Enabled != nil {
		if err := s.daily.SetEnabled(request.Context(), *input.Enabled); err != nil {
			writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable")
			return
		}
		current.Enabled = *input.Enabled
	}
	writeJSON(writer, http.StatusOK, dailyResponse{
		Enabled: current.Enabled, Time: current.Time, Timezone: current.Timezone,
	})
}
