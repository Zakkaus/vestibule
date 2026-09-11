package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
)

const (
	chatOwnerTestChatID   int64 = -1009000002411
	chatOwnerDeniedChatID int64 = -1009000002412
	chatOwnerTestOwnerID        = "9000002414"
	chatOwnerTestAdminID        = "9000002415"
)

var chatOwnerPermissionKeys = []string{
	"can_manage_chat",
	"can_delete_messages",
	"can_manage_video_chats",
	"can_restrict_members",
	"can_promote_members",
	"can_change_info",
	"can_invite_users",
	"can_post_stories",
	"can_edit_stories",
	"can_delete_stories",
	"can_post_messages",
	"can_edit_messages",
	"can_pin_messages",
	"can_manage_topics",
}

type chatOwnerAccessChecker struct {
	allowed map[int64]bool
}

func (c *chatOwnerAccessChecker) CachedAdmin(_ context.Context, chatID, _ int64) (bool, error) {
	return c.allowed[chatID], nil
}

type chatOwnerResolver struct {
	mu        sync.Mutex
	results   map[int64]ChatAdministrators
	sequences map[int64][]ChatAdministrators
	errors    map[int64]error
	calls     []int64
}

func (r *chatOwnerResolver) ChatAdministrators(
	_ context.Context,
	chatID int64,
) (ChatAdministrators, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, chatID)
	if sequence := r.sequences[chatID]; len(sequence) > 0 {
		result := sequence[0]
		r.sequences[chatID] = sequence[1:]
		return result, r.errors[chatID]
	}
	return r.results[chatID], r.errors[chatID]
}

func (r *chatOwnerResolver) calledIDs() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.calls...)
}

func TestChatsExposeCurrentTelegramOwnerAndPermissionsForAuthorizedChats(t *testing.T) {
	checker := &chatOwnerAccessChecker{allowed: map[int64]bool{
		chatOwnerTestChatID:   true,
		chatOwnerDeniedChatID: false,
	}}
	resolver := &chatOwnerResolver{
		results: map[int64]ChatAdministrators{
			chatOwnerTestChatID: {
				Type: "supergroup",
				Members: []ChatAdministrator{
					{
						User:   ChatUser{ID: chatOwnerTestOwnerID, FirstName: "Mira", LastName: "Owner"},
						Status: "creator",
						Permissions: ChatAdministratorPermissions{
							CanManageChat:      new(true),
							CanDeleteMessages:  new(true),
							CanRestrictMembers: new(true),
						},
					},
					{
						User:   ChatUser{ID: chatOwnerTestAdminID, FirstName: "Ada"},
						Status: "administrator",
						Permissions: ChatAdministratorPermissions{
							CanManageChat:      new(true),
							CanDeleteMessages:  new(false),
							CanRestrictMembers: new(false),
						},
					},
				},
			},
		},
		errors: map[int64]error{},
	}
	server, cookies, _ := apiTestServer(
		t,
		checker,
		&apiTestQueueService{groups: []int64{chatOwnerTestChatID, chatOwnerDeniedChatID}},
		nil,
	)
	routes := server.routes.Load().server
	server.ReplaceRoutes(Config{
		Authenticator:              routes.authenticator,
		Verification:               routes.verification,
		ChatAdministratorsResolver: resolver,
	})

	response := getAuthenticatedPath(server, cookies, "/api/chats")
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/chats status=%d, want %d", response.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	chats, ok := payload["chats"].([]any)
	if !ok || len(chats) != 1 {
		t.Fatalf("authorized chats=%#v, want one authorized chat", payload["chats"])
	}
	chat, ok := chats[0].(map[string]any)
	if !ok {
		t.Fatalf("authorized chat=%#v, want object", chats[0])
	}
	owner, ok := chat["owner"].(map[string]any)
	if !ok || owner["id"] != chatOwnerTestOwnerID || owner["first_name"] != "Mira" {
		t.Fatalf("owner=%#v, want Mira/%s", chat["owner"], chatOwnerTestOwnerID)
	}
	if chat["administrators_status"] != chatAdministratorsAvailable {
		t.Fatalf("administrators_status=%#v, want available", chat["administrators_status"])
	}
	administrators, ok := chat["administrators"].([]any)
	if !ok || len(administrators) != 2 {
		t.Fatalf("administrators=%#v, want creator and one administrator", chat["administrators"])
	}
	assertChatOwnerPermissions(t, administrators)
	if got := resolver.calledIDs(); len(got) != 1 || got[0] != chatOwnerTestChatID {
		t.Fatalf("Telegram administrator lookups=%v, want only authorized chat %d", got, chatOwnerTestChatID)
	}
}

func assertChatOwnerPermissions(t *testing.T, administrators []any) {
	t.Helper()
	for _, administrator := range administrators {
		entry, ok := administrator.(map[string]any)
		if !ok {
			t.Fatalf("administrator=%#v, want object", administrator)
		}
		permissions, ok := entry["permissions"].(map[string]any)
		if !ok {
			t.Fatalf("administrator permissions=%#v, want object", entry["permissions"])
		}
		for _, key := range chatOwnerPermissionKeys {
			value, present := permissions[key]
			if !present {
				t.Fatalf("administrator permissions missing %q: %#v", key, permissions)
			}
			if value != nil {
				if _, boolean := value.(bool); !boolean {
					t.Fatalf("administrator permission %q=%#v, want boolean or null", key, value)
				}
			}
		}
	}
}

func TestChatsExposeUnavailableTelegramDataWithoutResolver(t *testing.T) {
	server, cookies, _ := apiTestServer(
		t,
		&apiTestAdminChecker{allowed: true},
		&apiTestQueueService{groups: []int64{chatOwnerTestChatID}},
		nil,
	)

	response := getAuthenticatedPath(server, cookies, "/api/chats")
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/chats status=%d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		Chats []map[string]any `json:"chats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Chats) != 1 {
		t.Fatalf("chats=%#v, want one chat", payload.Chats)
	}
	chat := payload.Chats[0]
	if chat["id"] != strconv.FormatInt(chatOwnerTestChatID, 10) {
		t.Fatalf("chat id=%#v, want %d", chat["id"], chatOwnerTestChatID)
	}
	if chat["owner"] != nil {
		t.Fatalf("unavailable owner=%#v, want null", chat["owner"])
	}
	administrators, ok := chat["administrators"].([]any)
	if !ok || len(administrators) != 0 {
		t.Fatalf("unavailable administrators=%#v, want empty list", chat["administrators"])
	}
	if chat["administrators_status"] != chatAdministratorsUnavailable {
		t.Fatalf("administrators_status=%#v, want unavailable", chat["administrators_status"])
	}
}

func TestChatsRefreshTelegramOwnerOnEachRequest(t *testing.T) {
	firstOwnerID := "9000002421"
	secondOwnerID := "9000002422"
	resolver := &chatOwnerResolver{
		sequences: map[int64][]ChatAdministrators{
			chatOwnerTestChatID: {
				{Members: []ChatAdministrator{{
					User:   ChatUser{ID: firstOwnerID, FirstName: "First"},
					Status: "creator",
				}}},
				{Members: []ChatAdministrator{{
					User:   ChatUser{ID: secondOwnerID, FirstName: "Second"},
					Status: "creator",
				}}},
			},
		},
	}
	server, cookies, _ := apiTestServer(
		t,
		&apiTestAdminChecker{allowed: true},
		&apiTestQueueService{groups: []int64{chatOwnerTestChatID}},
		nil,
	)
	routes := server.routes.Load().server
	server.ReplaceRoutes(Config{
		Authenticator:              routes.authenticator,
		Verification:               routes.verification,
		ChatAdministratorsResolver: resolver,
	})

	readOwnerID := func() string {
		response := getAuthenticatedPath(server, cookies, "/api/chats")
		if response.Code != http.StatusOK {
			t.Fatalf("GET /api/chats status=%d, want %d", response.Code, http.StatusOK)
		}
		var payload struct {
			Chats []map[string]any `json:"chats"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Chats) != 1 {
			t.Fatalf("chats=%#v, want one chat", payload.Chats)
		}
		owner, ok := payload.Chats[0]["owner"].(map[string]any)
		if !ok {
			t.Fatalf("owner=%#v, want object", payload.Chats[0]["owner"])
		}
		id, ok := owner["id"].(string)
		if !ok {
			t.Fatalf("owner id=%#v, want string", owner["id"])
		}
		return id
	}

	if got := readOwnerID(); got != firstOwnerID {
		t.Fatalf("first owner=%q, want %q", got, firstOwnerID)
	}
	if got := readOwnerID(); got != secondOwnerID {
		t.Fatalf("second owner=%q, want fresh %q", got, secondOwnerID)
	}
	if got := resolver.calledIDs(); len(got) != 2 {
		t.Fatalf("Telegram administrator lookups=%v, want two fresh lookups", got)
	}
}

func TestChatsMarkTelegramAdministratorLookupUnavailable(t *testing.T) {
	resolver := &chatOwnerResolver{
		results: map[int64]ChatAdministrators{
			chatOwnerTestChatID: {
				Type: "supergroup",
				Members: []ChatAdministrator{{
					User:   ChatUser{ID: chatOwnerTestOwnerID, FirstName: "Mira"},
					Status: "creator",
				}},
			},
		},
		errors: map[int64]error{chatOwnerTestChatID: errors.New("Telegram unavailable")},
	}
	server, cookies, _ := apiTestServer(
		t,
		&apiTestAdminChecker{allowed: true},
		&apiTestQueueService{groups: []int64{chatOwnerTestChatID}},
		nil,
	)
	routes := server.routes.Load().server
	server.ReplaceRoutes(Config{
		Authenticator:              routes.authenticator,
		Verification:               routes.verification,
		ChatAdministratorsResolver: resolver,
	})

	response := getAuthenticatedPath(server, cookies, "/api/chats")
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	chats, ok := payload["chats"].([]any)
	if !ok || len(chats) != 1 {
		t.Fatalf("unavailable chats=%#v, want one chat", payload["chats"])
	}
	chat, ok := chats[0].(map[string]any)
	if !ok {
		t.Fatalf("unavailable chat=%#v, want object", chats[0])
	}
	if chat["owner"] != nil {
		t.Fatalf("unavailable owner=%#v, want null", chat["owner"])
	}
	administrators, ok := chat["administrators"].([]any)
	if !ok || len(administrators) != 0 {
		t.Fatalf("unavailable administrators=%#v, want empty list", chat["administrators"])
	}
	if chat["administrators_status"] != chatAdministratorsUnavailable {
		t.Fatalf("administrators_status=%#v, want unavailable", chat["administrators_status"])
	}
}
