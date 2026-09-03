package api

import (
	"net/http"
	"strings"
)

type instanceResponse struct {
	BotUsername string `json:"bot_username"`
}

// instance answers without a session on purpose: the screen that tells a visitor
// which bot to open in Telegram is exactly the screen they reach when they hold
// no session, and a bot's handle is public the moment the bot exists. The handle
// is empty while nobody has claimed the instance, and the screen then says there
// is no bot yet rather than naming one this deployment has nothing to do with.
func (s *Server) instance(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, instanceResponse{BotUsername: strings.TrimPrefix(s.botUsername, "@")})
}
