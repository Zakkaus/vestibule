package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const apiSettingsGroupID int64 = -1009000000101

type apiTestSettingsService struct {
	store       *settings.Store
	updateCalls int
}

func (s *apiTestSettingsService) Settings(groupID int64) (settings.GroupView, bool) {
	return s.store.Settings(groupID)
}

func (s *apiTestSettingsService) Update(
	groupID int64,
	expectedRevision uint64,
	next settings.GroupOverrides,
) (settings.CommitResult, error) {
	s.updateCalls++
	return s.store.Update(groupID, expectedRevision, next)
}

func TestGetSettingsRejectsUnauthorizedChat(t *testing.T) {
	server, cookies, _, service, checker := apiSettingsTestServer(t, false)
	response := getAuthenticatedPath(server, cookies, settingsPath(apiSettingsGroupID))
	counts := checker.counts()
	if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" ||
		counts.cachedCalls != 1 || counts.freshCalls != 0 || service.updateCalls != 0 {
		t.Fatalf("status=%d code=%s cached=%d fresh=%d updates=%d, want 403, chat_access_denied, 1, 0, 0",
			response.Code, decodeError(response), counts.cachedCalls, counts.freshCalls, service.updateCalls)
	}
}

func TestPatchSettingsUsesFreshAdminAfterCachedRead(t *testing.T) {
	server, cookies, csrf, service, checker := apiSettingsTestServer(t, true)
	read := getAuthenticatedPath(server, cookies, settingsPath(apiSettingsGroupID))
	if read.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200", read.Code)
	}
	checker.setAllowed(false)
	write := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"enabled":false}}`)
	counts := checker.counts()
	group, _ := service.store.Settings(apiSettingsGroupID)
	if write.Code != http.StatusForbidden || decodeError(write) != "chat_access_denied" ||
		counts.cachedCalls != 1 || counts.freshCalls != 1 || counts.telegramQueries != 2 ||
		service.updateCalls != 0 || group.Revision() != 0 || !group.Enabled().Value {
		t.Fatalf("status=%d code=%s cached=%d fresh=%d queries=%d updates=%d revision=%d enabled=%v",
			write.Code, decodeError(write), counts.cachedCalls, counts.freshCalls, counts.telegramQueries,
			service.updateCalls, group.Revision(), group.Enabled().Value)
	}
}

func TestPatchSettingsRequiresCSRF(t *testing.T) {
	server, cookies, _, service, checker := apiSettingsTestServer(t, true)
	response := patchGroupSettings(server, cookies, "", apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"enabled":false}}`)
	group, _ := service.store.Settings(apiSettingsGroupID)
	counts := checker.counts()
	if response.Code != http.StatusForbidden || decodeError(response) != "csrf_invalid" ||
		counts.freshCalls != 1 || service.updateCalls != 0 || group.Revision() != 0 || !group.Enabled().Value {
		t.Fatalf("status=%d code=%s fresh=%d updates=%d revision=%d enabled=%v",
			response.Code, decodeError(response), counts.freshCalls, service.updateCalls,
			group.Revision(), group.Enabled().Value)
	}
}

func TestPatchSettingsReturnsDedicatedRevisionConflict(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	group, _ := service.store.Settings(apiSettingsGroupID)
	next := group.Overrides()
	spoiler := false
	next.NameSpoiler = &spoiler
	if _, err := service.store.Update(apiSettingsGroupID, group.Revision(), next); err != nil {
		t.Fatal(err)
	}
	response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"enabled":false}}`)
	group, _ = service.store.Settings(apiSettingsGroupID)
	if response.Code != http.StatusConflict || decodeError(response) != "settings_conflict" ||
		service.updateCalls != 1 || group.Revision() != 1 || !group.Enabled().Value || group.NameSpoiler().Value {
		t.Fatalf("status=%d code=%s updates=%d revision=%d enabled=%v name_spoiler=%v",
			response.Code, decodeError(response), service.updateCalls, group.Revision(),
			group.Enabled().Value, group.NameSpoiler().Value)
	}
}

func TestPatchSettingsTreatsBaselineEqualChangeAsSuccess(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"enabled":true}}`)
	body := decodeSettings(t, response)
	if response.Code != http.StatusOK || service.updateCalls != 1 || body.Revision != 0 ||
		!body.Enabled.Value || body.Enabled.Source != settings.SourceFactory.String() {
		t.Fatalf("status=%d updates=%d revision=%d enabled=%v source=%q",
			response.Code, service.updateCalls, body.Revision, body.Enabled.Value, body.Enabled.Source)
	}
}

func TestPatchSettingsMergesSparseChangesAndRestoresDefault(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	group, _ := service.store.Settings(apiSettingsGroupID)
	next := group.Overrides()
	spoiler := false
	next.NameSpoiler = &spoiler
	if _, err := service.store.Update(apiSettingsGroupID, group.Revision(), next); err != nil {
		t.Fatal(err)
	}
	changed := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":1,"changes":{"enabled":false}}`)
	changedBody := decodeSettings(t, changed)
	if changed.Code != http.StatusOK || changedBody.Revision != 2 || changedBody.Enabled.Value ||
		changedBody.NameSpoiler.Value || changedBody.Enabled.Source != settings.SourceChatOverride.String() ||
		changedBody.NameSpoiler.Source != settings.SourceChatOverride.String() {
		t.Fatalf("changed status=%d body=%+v", changed.Code, changedBody)
	}
	restored := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":2,"changes":{"enabled":null}}`)
	restoredBody := decodeSettings(t, restored)
	if restored.Code != http.StatusOK || restoredBody.Revision != 3 || !restoredBody.Enabled.Value ||
		restoredBody.NameSpoiler.Value || restoredBody.Enabled.Source != settings.SourceFactory.String() ||
		restoredBody.NameSpoiler.Source != settings.SourceChatOverride.String() {
		t.Fatalf("restored status=%d body=%+v", restored.Code, restoredBody)
	}
}

func TestPatchSettingsCarriesEverySupportedFieldIntoTheResponse(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID, `{
		"expected_revision": 0,
		"changes": {
			"enabled": false,
			"delivery_mode": "dm",
			"verify_mode": "quiz",
			"name_spoiler": false,
			"ban_seconds": 60,
			"lookup_ttl_seconds": 120,
			"lookup_auto_delete_enabled": false,
			"timeout_seconds": 300,
			"verify_max_fails": 4,
			"verify_retry_seconds": 200,
			"mute_seconds": 600,
			"verify_invited": false,
			"warn_limit": 4,
			"antispam_enabled": false,
			"channel_whitelist": [-1009000000111],
			"trusted_member_group_ids": [-1009000000112],
			"known_chat_ids": [-1009000000113],
			"required_channel_id": -1009000000114,
			"channel_display": "@required",
			"channel_invite_url": "https://t.me/+required",
			"required_channel_fail_open": false,
			"questions": [{"q":"Pick one","options":["wrong","right"],"answer":1}],
			"fallback_questions": [{"q":"Name one","answers":["answer"]}],
			"fallback_builtin": false,
			"lang": "en",
			"rich_messages": true,
			"private_query_per_min": 5,
			"admin_log_chat_id": -1009000000115
		}
	}`)
	body := decodeSettings(t, response)
	source := settings.SourceChatOverride.String()
	want := settingsResponse{
		Revision:                1,
		Enabled:                 settingResponse[bool]{Value: false, Source: source},
		DeliveryMode:            settingResponse[string]{Value: "dm", Source: source},
		VerifyMode:              settingResponse[string]{Value: "quiz", Source: source},
		NameSpoiler:             settingResponse[bool]{Value: false, Source: source},
		BanSeconds:              settingResponse[int]{Value: 60, Source: source},
		LookupTTLSeconds:        settingResponse[int]{Value: 120, Source: source},
		LookupAutoDeleteEnabled: settingResponse[bool]{Value: false, Source: source},
		TimeoutSeconds:          settingResponse[int]{Value: 300, Source: source},
		VerifyMaxFails:          settingResponse[int]{Value: 4, Source: source},
		VerifyRetrySeconds:      settingResponse[int]{Value: 200, Source: source},
		MuteSeconds:             settingResponse[int]{Value: 600, Source: source},
		VerifyInvited:           settingResponse[bool]{Value: false, Source: source},
		WarnLimit:               settingResponse[int]{Value: 4, Source: source},
		AntispamEnabled:         settingResponse[bool]{Value: false, Source: source},
		ChannelWhitelist:        settingResponse[[]int64]{Value: []int64{-1009000000111}, Source: source},
		TrustedMemberGroupIDs:   settingResponse[[]int64]{Value: []int64{-1009000000112}, Source: source},
		KnownChatIDs:            settingResponse[[]int64]{Value: []int64{-1009000000113}, Source: source},
		RequiredChannelID:       settingResponse[int64]{Value: -1009000000114, Source: source},
		ChannelDisplay:          settingResponse[string]{Value: "@required", Source: source},
		ChannelInviteURL:        settingResponse[string]{Value: "https://t.me/+required", Source: source},
		Questions: settingResponse[[]settings.Question]{
			Value: []settings.Question{{Q: "Pick one", Options: []string{"wrong", "right"}, Answer: 1}}, Source: source,
		},
		FallbackQuestions: settingResponse[[]settings.ShortQuestion]{
			Value: []settings.ShortQuestion{{Q: "Name one", Answers: []string{"answer"}}}, Source: source,
		},
		FallbackBuiltin:         settingResponse[bool]{Value: false, Source: source},
		Lang:                    settingResponse[string]{Value: "en", Source: source},
		RichMessages:            settingResponse[bool]{Value: true, Source: source},
		PrivateQueryPerMin:      settingResponse[int]{Value: 5, Source: source},
		AdminLogChatID:          settingResponse[int64]{Value: -1009000000115, Source: source},
		RequiredChannelFailOpen: settingResponse[bool]{Value: false, Source: source},
	}
	if response.Code != http.StatusOK || service.updateCalls != 1 || !reflect.DeepEqual(body, want) {
		t.Fatalf("settings response lost a supported field: status=%d updates=%d body=%+v, want %+v",
			response.Code, service.updateCalls, body, want)
	}
}

func apiSettingsTestServer(
	t *testing.T,
	allowed bool,
) (*Server, []*http.Cookie, string, *apiTestSettingsService, *apiTestAdminChecker) {
	t.Helper()
	config := &settings.Config{GroupIDs: []int64{apiSettingsGroupID}}
	baseline, err := settings.LoadBaseline("", config)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := &apiTestSettingsService{store: store}
	checker := &apiTestAdminChecker{allowed: allowed}
	groups := &apiTestQueueService{groups: []int64{apiSettingsGroupID}}
	server, cookies, csrf := apiTestServer(t, checker, groups, nil, service)
	return server, cookies, csrf, service, checker
}

func settingsPath(groupID int64) string {
	return "/api/chats/" + strconv.FormatInt(groupID, 10) + "/settings"
}

func patchGroupSettings(
	server *Server,
	cookies []*http.Cookie,
	csrf string,
	groupID int64,
	body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPatch, settingsPath(groupID), strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", csrf)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func decodeSettings(t *testing.T, response *httptest.ResponseRecorder) settingsResponse {
	t.Helper()
	var body settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
