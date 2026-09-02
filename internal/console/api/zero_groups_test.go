package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestZeroConfiguredGroupsReturnEmptyListAndChatNotFound(t *testing.T) {
	checker := &apiTestAdminChecker{allowed: true}
	service := &apiTestQueueService{}
	server, cookies, _ := apiTestServer(t, checker, service, nil)

	list := getAuthenticatedPath(server, cookies, "/api/chats")
	var payload struct {
		Chats []chatResponse `json:"chats"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if list.Code != http.StatusOK || payload.Chats == nil || len(payload.Chats) != 0 {
		t.Fatalf("zero-group chat list = status:%d chats:%#v, want 200 and []", list.Code, payload.Chats)
	}

	chatID := int64(-1009000000899)
	prefix := "/api/chats/" + strconv.FormatInt(chatID, 10)
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, prefix + "/queue"},
		{http.MethodPost, prefix + "/queue/challenge"},
		{http.MethodGet, prefix + "/audit"},
		{http.MethodPost, prefix + "/audit/action/undo"},
		{http.MethodGet, prefix + "/stats"},
		{http.MethodGet, prefix + "/settings"},
		{http.MethodPatch, prefix + "/settings"},
		{http.MethodGet, prefix + "/rules"},
		{http.MethodPut, prefix + "/rules"},
		{http.MethodPut, prefix + "/rules/rule"},
	}
	for _, endpoint := range endpoints {
		request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || decodeError(response) != "chat_not_found" {
			t.Errorf("%s %s = %d %q, want 404 chat_not_found",
				endpoint.method, endpoint.path, response.Code, decodeError(response))
		}
	}
	counts := checker.counts()
	if counts.cachedCalls != 0 || counts.freshCalls != 0 || counts.telegramQueries != 0 {
		t.Fatalf("zero-group endpoints queried Telegram: %+v", counts)
	}
}
