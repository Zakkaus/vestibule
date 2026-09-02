package api

import (
	"errors"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
)

// ReplacementService supplies the host-unit facts and version-only request boundary.
type ReplacementService interface {
	Status() status.ReplacementStatus
	Request(string) error
}

type upgradeRequest struct {
	Version string `json:"version"`
}

type upgradeResponse struct {
	Status string `json:"status"`
}

func (s *Server) requestUpgrade(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "upgrade_access_denied")
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	if s.replacement == nil {
		writeError(writer, http.StatusServiceUnavailable, "upgrade_unavailable")
		return
	}
	var input upgradeRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if err := s.replacement.Request(input.Version); err != nil {
		switch {
		case errors.Is(err, status.ErrInvalidReplacementVersion):
			writeError(writer, http.StatusBadRequest, "invalid_upgrade_version")
		case errors.Is(err, status.ErrReplacementUnavailable):
			writeError(writer, http.StatusConflict, "upgrade_unavailable")
		default:
			writeError(writer, http.StatusServiceUnavailable, "upgrade_unavailable")
		}
		return
	}
	writeJSON(writer, http.StatusAccepted, upgradeResponse{Status: "requested"})
}
