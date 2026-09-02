package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

// PersistenceService supplies the settings persistence state needed by diagnostics.
type PersistenceService interface {
	Persistence() settings.PersistenceStatus
}

// RollbackObservationsService supplies process-local cutover measurements.
type RollbackObservationsService interface {
	Snapshot() status.RollbackSnapshot
}

// RollbackRejectionService supplies privacy-preserving rejected-challenge counts.
type RollbackRejectionService interface {
	RecentRejections(context.Context, time.Time) ([]verification.RejectionReasonCount, error)
}

type diagnosticsResponse struct {
	Version     string                         `json:"version"`
	Health      diagnosticsHealthResponse      `json:"health"`
	BotAPI      diagnosticsBotAPIResponse      `json:"bot_api"`
	Persistence diagnosticsPersistenceResponse `json:"persistence"`
	Rollback    diagnosticsRollbackResponse    `json:"rollback_observations"`
	Replacement diagnosticsReplacementResponse `json:"replacement"`
}

type diagnosticsHealthResponse struct {
	Live          bool `json:"live"`
	Ready         bool `json:"ready"`
	ConfigReady   bool `json:"config_ready"`
	TelegramReady bool `json:"telegram_ready"`
}

type diagnosticsBotAPIResponse struct {
	LastHeartbeatAt     *time.Time `json:"last_heartbeat_at"`
	LatencyMilliseconds *int64     `json:"latency_ms"`
}

type diagnosticsPersistenceResponse struct {
	Configured bool    `json:"configured"`
	Durable    bool    `json:"durable"`
	Writable   bool    `json:"writable"`
	LastError  *string `json:"last_error"`
}

type diagnosticsReplacementResponse struct {
	UnitAvailable bool                              `json:"unit_available"`
	LastResult    *diagnosticsReplacementResultView `json:"last_result"`
}

type diagnosticsReplacementResultView struct {
	Status           string `json:"status"`
	RequestedVersion string `json:"requested_version"`
	Reason           string `json:"reason"`
}

func (s *Server) readDiagnostics(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "diagnostics_access_denied")
		return
	}
	if s.health == nil || s.persistence == nil || s.rollbackObservations == nil || s.rollbackRejections == nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), healthProbeTimeout)
	defer cancel()
	health := s.health.Snapshot()
	observations := s.rollbackObservations.Snapshot()
	rejectionCtx, cancelRejections := context.WithTimeout(request.Context(), healthProbeTimeout)
	defer cancelRejections()
	rejections, err := s.rollbackRejections.RecentRejections(
		rejectionCtx, observations.ObservedAt.Add(-verification.RollbackRejectionWindow),
	)
	rollback := diagnosticsRollbackView(observations, rejections, err == nil)
	replacement := status.ReplacementStatus{}
	if s.replacement != nil {
		replacement = s.replacement.Status()
	}
	writeJSON(
		writer,
		http.StatusOK,
		diagnosticsView(
			s.version, health, s.health.Ready(ctx), s.persistence.Persistence(), replacement, rollback,
		),
	)
}

func diagnosticsView(
	version string,
	health status.HealthSnapshot,
	ready bool,
	persistence settings.PersistenceStatus,
	replacement status.ReplacementStatus,
	rollback diagnosticsRollbackResponse,
) diagnosticsResponse {
	response := diagnosticsResponse{
		Version: version,
		Health: diagnosticsHealthResponse{
			Live: health.Live, Ready: ready,
			ConfigReady: health.ConfigReady, TelegramReady: health.TelegramReady,
		},
		Persistence: diagnosticsPersistenceResponse{
			Configured: persistence.Configured,
			Durable:    persistence.Durable,
			Writable:   persistence.Writable,
		},
		Rollback: rollback,
		Replacement: diagnosticsReplacementResponse{
			UnitAvailable: replacement.UnitAvailable,
		},
	}
	if probe := health.TelegramProbe; probe != nil {
		at := probe.At.UTC()
		latency := probe.Latency.Milliseconds()
		response.BotAPI.LastHeartbeatAt = &at
		response.BotAPI.LatencyMilliseconds = &latency
	}
	if persistence.LastError != nil {
		lastError := status.RedactToken(persistence.LastError.Error())
		response.Persistence.LastError = &lastError
	}
	if result := replacement.LastResult; result != nil {
		response.Replacement.LastResult = &diagnosticsReplacementResultView{
			Status:           result.Status,
			RequestedVersion: result.RequestedVersion,
			Reason:           result.Reason,
		}
	}
	return response
}
