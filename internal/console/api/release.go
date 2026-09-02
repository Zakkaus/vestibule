package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/status"
)

const releaseLookupTimeout = 12 * time.Second

// ReleaseService supplies on-demand upstream release metadata.
type ReleaseService interface {
	Latest(context.Context) (status.ReleaseInfo, error)
}

type releaseResponse struct {
	Version         string                   `json:"version"`
	URL             string                   `json:"url"`
	Notes           string                   `json:"notes"`
	PublishedAt     time.Time                `json:"published_at"`
	UpdateAvailable bool                     `json:"update_available"`
	Rollback        *releaseRollbackResponse `json:"rollback"`
}

type releaseRollbackResponse struct {
	Available                    bool   `json:"available"`
	Reason                       string `json:"reason"`
	TargetSchemaVersion          int    `json:"target_schema_version"`
	RetainedSchemaVersion        int    `json:"retained_schema_version"`
	MinimumRollbackSchemaVersion int    `json:"minimum_rollback_schema_version"`
}

func (s *Server) readLatestRelease(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "release_lookup_access_denied")
		return
	}
	if s.release == nil {
		writeError(writer, http.StatusServiceUnavailable, "release_lookup_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), releaseLookupTimeout)
	defer cancel()
	info, err := s.release.Latest(ctx)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "release_lookup_unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, releaseView(info))
}

func releaseView(info status.ReleaseInfo) releaseResponse {
	response := releaseResponse{
		Version:         info.Version,
		URL:             info.URL,
		Notes:           info.Notes,
		PublishedAt:     info.PublishedAt,
		UpdateAvailable: info.UpdateAvailable,
	}
	if rollback := info.Rollback; rollback != nil {
		response.Rollback = &releaseRollbackResponse{
			Available:                    rollback.Available,
			Reason:                       string(rollback.Reason),
			TargetSchemaVersion:          rollback.TargetSchemaVersion,
			RetainedSchemaVersion:        rollback.RetainedSchemaVersion,
			MinimumRollbackSchemaVersion: rollback.MinimumRollbackSchemaVersion,
		}
	}
	return response
}
