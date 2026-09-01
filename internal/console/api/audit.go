package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const auditKindChallenge = "challenge"

type auditResponse struct {
	ID        string                        `json:"id"`
	Kind      string                        `json:"kind"`
	User      string                        `json:"user"`
	GroupKey  string                        `json:"group_key"`
	Result    resultResponse                `json:"result"`
	SettledAt time.Time                     `json:"settled_at"`
	SettledBy *string                       `json:"settled_by"`
	UndoState verification.ConsoleUndoState `json:"undo_state"`
}

func auditView(entry verification.ConsoleAuditEntry) auditResponse {
	user := entry.Name
	if user == "" {
		user = strconv.FormatInt(entry.UserID, 10)
	}
	response := auditResponse{
		ID: entry.ID, Kind: auditKindChallenge, User: user,
		GroupKey: strconv.FormatInt(entry.GroupID, 10), Result: resultResponse{State: entry.State},
		SettledAt: entry.SettledAt, UndoState: entry.UndoState,
	}
	if entry.State == verification.ChallengeDeclined {
		reason := entry.Reason
		response.Result.Reason = &reason
	}
	if entry.SettledBy != 0 {
		settledBy := strconv.FormatInt(entry.SettledBy, 10)
		response.SettledBy = &settledBy
	}
	return response
}

func (s *Server) audit(writer http.ResponseWriter, request *http.Request, chatID int64) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess)
	if !ok {
		return
	}
	entries, err := s.verification.ConsoleAudit(
		request.Context(), chatID, session.Principal.TelegramID,
	)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "audit_unavailable")
		return
	}
	items := make([]auditResponse, 0, len(entries))
	for _, entry := range entries {
		items = append(items, auditView(entry))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) undoAudit(
	writer http.ResponseWriter,
	request *http.Request,
	chatID int64,
	auditID string,
) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	entry, err := s.verification.UndoConsoleAudit(request.Context(), verification.ConsoleAuditUndo{
		ID: auditID, GroupID: chatID, ActorID: session.Principal.TelegramID,
	})
	if err != nil {
		writeAuditError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, auditView(entry))
}

func writeAuditError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, verification.ErrConsoleAuditInvalid):
		writeError(writer, http.StatusBadRequest, "invalid_audit")
	case errors.Is(err, verification.ErrConsoleAuditNotFound):
		writeError(writer, http.StatusNotFound, "audit_not_found")
	case errors.Is(err, verification.ErrConsoleAuditNotUndoable):
		writeError(writer, http.StatusConflict, "audit_not_undoable")
	case errors.Is(err, verification.ErrConsoleAuditConflict):
		writeError(writer, http.StatusConflict, "audit_conflict")
	default:
		writeError(writer, http.StatusServiceUnavailable, "audit_unavailable")
	}
}
