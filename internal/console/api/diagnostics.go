package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
)

// PersistenceService supplies the settings persistence state needed by diagnostics.
type PersistenceService interface {
	Persistence() settings.PersistenceStatus
}

type diagnosticsResponse struct {
	Health      diagnosticsHealthResponse      `json:"health"`
	BotAPI      diagnosticsBotAPIResponse      `json:"bot_api"`
	Persistence diagnosticsPersistenceResponse `json:"persistence"`
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

func (s *Server) readDiagnostics(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "diagnostics_access_denied")
		return
	}
	if s.health == nil || s.persistence == nil {
		writeError(writer, http.StatusServiceUnavailable, "diagnostics_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), healthProbeTimeout)
	defer cancel()
	health := s.health.Snapshot()
	writeJSON(writer, http.StatusOK, diagnosticsView(health, s.health.Ready(ctx), s.persistence.Persistence()))
}

func diagnosticsView(
	health status.HealthSnapshot,
	ready bool,
	persistence settings.PersistenceStatus,
) diagnosticsResponse {
	response := diagnosticsResponse{
		Health: diagnosticsHealthResponse{
			Live: health.Live, Ready: ready,
			ConfigReady: health.ConfigReady, TelegramReady: health.TelegramReady,
		},
		Persistence: diagnosticsPersistenceResponse{
			Configured: persistence.Configured,
			Durable:    persistence.Durable,
			Writable:   persistence.Writable,
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
	return response
}
