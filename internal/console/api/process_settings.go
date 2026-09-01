package api

import (
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// ProcessSettingsService supplies the immutable process-level settings view.
type ProcessSettingsService interface {
	ProcessSettings() settings.ProcessView
}

type processSettingsResponse struct {
	Feeds         settingResponse[[]settings.FeedConfig] `json:"feeds"`
	NewsURL       settingResponse[string]                `json:"news_url"`
	Overlays      settingResponse[[]settings.OverlayCfg] `json:"overlays"`
	StatsTimezone settingResponse[string]                `json:"stats_timezone"`
}

func processSettingsView(view settings.ProcessView) processSettingsResponse {
	return processSettingsResponse{
		Feeds:         settingView(view.Feeds()),
		NewsURL:       settingView(view.NewsURL()),
		Overlays:      settingView(view.Overlays()),
		StatsTimezone: settingView(view.StatsTimezone()),
	}
}

func (s *Server) readProcessSettings(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if session.Principal.Role != auth.RoleOperator {
		writeError(writer, http.StatusForbidden, "process_access_denied")
		return
	}
	if s.processSettings == nil {
		writeError(writer, http.StatusServiceUnavailable, "process_settings_unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, processSettingsView(s.processSettings.ProcessSettings()))
}
