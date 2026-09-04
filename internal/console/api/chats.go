package api

import (
	"net/http"
	"strconv"
)

type chatResponse struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

func (s *Server) chats(writer http.ResponseWriter, request *http.Request) {
	session, ok := s.session(writer, request)
	if !ok {
		return
	}
	if s.verification == nil {
		writeError(writer, http.StatusServiceUnavailable, "verification_unavailable")
		return
	}
	candidates := s.verification.ConsoleGroups()
	allowed := s.authenticator.AccessibleChats(request.Context(), session, candidates)
	chats := make([]chatResponse, 0, len(allowed))
	for _, chatID := range allowed {
		title := ""
		if s.settings != nil {
			title, _ = s.settings.RegisteredGroupTitle(chatID)
		}
		chats = append(chats, chatResponse{
			ID:    strconv.FormatInt(chatID, 10),
			Title: title,
		})
	}
	writeJSON(writer, http.StatusOK, map[string]any{"chats": chats})
}
