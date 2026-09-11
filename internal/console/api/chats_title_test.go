package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	configuredTitleChatID int64 = -1009000001982
	registeredTitleChatID int64 = -1009000001983
)

type recordingChatTitleResolver struct {
	mu     sync.Mutex
	titles map[int64]string
	errs   map[int64]error
	calls  []int64
}

func (r *recordingChatTitleResolver) ChatTitle(_ context.Context, chatID int64) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, chatID)
	if err := r.errs[chatID]; err != nil {
		return "", err
	}
	return r.titles[chatID], nil
}

func (r *recordingChatTitleResolver) calledIDs() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int64(nil), r.calls...)
}

func TestChatsResolvesConfiguredTitleButPreservesRegisteredTitle(t *testing.T) {
	store := newChatTitleStore(t, configuredTitleChatID)
	registration := store.Registrations()
	registration.OwnerID = 9
	registration.RegisteredGroups = []settings.RegisteredGroup{{
		ID: registeredTitleChatID, RegisteredBy: 9, Title: "Registered title",
	}}
	if _, err := store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	resolver := &recordingChatTitleResolver{
		titles: map[int64]string{configuredTitleChatID: "Configured title"},
		errs:   map[int64]error{},
	}
	queue := &apiTestQueueService{groups: []int64{registeredTitleChatID, configuredTitleChatID}}
	server, cookies, _ := apiTestServer(t, &apiTestAdminChecker{allowed: true}, queue, nil,
		&apiTestSettingsService{store: store})
	routes := server.routes.Load().server
	server.ReplaceRoutes(Config{
		Authenticator:     routes.authenticator,
		Verification:      queue,
		Settings:          routes.settings,
		ChatTitleResolver: resolver,
	})

	response := getAuthenticatedPath(server, cookies, "/api/chats")
	var body struct {
		Chats []chatResponse `json:"chats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []chatResponse{
		{ID: strconv.FormatInt(registeredTitleChatID, 10), Title: "Registered title", Owner: nil, Administrators: []ChatAdministrator{}, AdministratorsStatus: "unavailable"},
		{ID: strconv.FormatInt(configuredTitleChatID, 10), Title: "Configured title", Owner: nil, Administrators: []ChatAdministrator{}, AdministratorsStatus: "unavailable"},
	}
	if response.Code != http.StatusOK || !reflect.DeepEqual(body.Chats, want) {
		t.Fatalf("chat titles = status %d, %+v; want status %d, %+v", response.Code, body.Chats, http.StatusOK, want)
	}
	if got := resolver.calledIDs(); !reflect.DeepEqual(got, []int64{configuredTitleChatID}) {
		t.Fatalf("Telegram title lookups = %v; want only configured chat", got)
	}
}

func TestChatsOmitsTitleWhenConfiguredLookupFails(t *testing.T) {
	store := newChatTitleStore(t, configuredTitleChatID)
	resolver := &recordingChatTitleResolver{
		titles: map[int64]string{},
		errs:   map[int64]error{configuredTitleChatID: errors.New("Telegram unavailable")},
	}
	queue := &apiTestQueueService{groups: []int64{configuredTitleChatID}}
	server, cookies, _ := apiTestServer(t, &apiTestAdminChecker{allowed: true}, queue, nil,
		&apiTestSettingsService{store: store})
	routes := server.routes.Load().server
	server.ReplaceRoutes(Config{
		Authenticator:     routes.authenticator,
		Verification:      queue,
		Settings:          routes.settings,
		ChatTitleResolver: resolver,
	})

	response := getAuthenticatedPath(server, cookies, "/api/chats")
	var body struct {
		Chats []chatResponse `json:"chats"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	want := []chatResponse{{ID: strconv.FormatInt(configuredTitleChatID, 10), Owner: nil, Administrators: []ChatAdministrator{}, AdministratorsStatus: "unavailable"}}
	if response.Code != http.StatusOK || !reflect.DeepEqual(body.Chats, want) {
		t.Fatalf("failed configured title = status %d, %+v; want status %d, %+v",
			response.Code, body.Chats, http.StatusOK, want)
	}
}

func newChatTitleStore(t *testing.T, chatIDs ...int64) *settings.Store {
	t.Helper()
	baselineConfig := &settings.Config{GroupIDs: append([]int64(nil), chatIDs...)}
	baseline, err := settings.LoadBaseline("", baselineConfig)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
