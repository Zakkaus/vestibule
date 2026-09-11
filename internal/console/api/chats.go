package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

const (
	chatAdministratorsAvailable   = "available"
	chatAdministratorsUnavailable = "unavailable"
)

type chatResponse struct {
	ID                   string              `json:"id"`
	Title                string              `json:"title,omitempty"`
	Owner                *ChatUser           `json:"owner"`
	Administrators       []ChatAdministrator `json:"administrators"`
	AdministratorsStatus string              `json:"administrators_status"`
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
		if title == "" && s.chatTitleResolver != nil {
			title, _ = s.chatTitleResolver.ChatTitle(request.Context(), chatID)
			if strings.TrimSpace(title) == "" {
				title = ""
			}
		}
		chat := chatResponse{
			ID:                   strconv.FormatInt(chatID, 10),
			Title:                title,
			Administrators:       []ChatAdministrator{},
			AdministratorsStatus: chatAdministratorsUnavailable,
		}
		if s.chatAdministratorsResolver != nil {
			chat.Owner, chat.Administrators, chat.AdministratorsStatus =
				s.lookupChatAdministrators(request.Context(), chatID)
		}
		chats = append(chats, chat)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"chats": chats})
}

func (s *Server) lookupChatAdministrators(ctx context.Context, chatID int64) (
	*ChatUser, []ChatAdministrator, string,
) {
	data, err := s.chatAdministratorsResolver.ChatAdministrators(ctx, chatID)
	if err != nil {
		return nil, []ChatAdministrator{}, chatAdministratorsUnavailable
	}
	var owner *ChatUser
	for _, member := range data.Members {
		if member.Status == "creator" {
			candidate := member.User
			owner = &candidate
			continue
		}
		if member.Status != "administrator" {
			return nil, []ChatAdministrator{}, chatAdministratorsUnavailable
		}
	}
	if owner == nil {
		return nil, []ChatAdministrator{}, chatAdministratorsUnavailable
	}
	return owner, data.Members, chatAdministratorsAvailable
}
