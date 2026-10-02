package api

import "github.com/Zakkaus/vestibule/internal/settings"

type webModeAvailability struct {
	Mode      string `json:"mode"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

func (s *Server) webModeAvailability(groupID int64) []webModeAvailability {
	if s.webVerification == nil {
		return nil
	}
	modes := make([]webModeAvailability, 2)
	for index, mode := range [...]string{settings.ModePoW, settings.ModeCaptcha} {
		reason := s.webVerification.WebModeUnavailable(groupID, mode)
		modes[index] = webModeAvailability{Mode: mode, Available: reason == "", Reason: reason}
	}
	return modes
}
func (s *Server) webSettingsView(group settings.GroupView, groupID int64) settingsResponse {
	response := settingsView(group)
	response.WebModes = s.webModeAvailability(groupID)
	return response
}
