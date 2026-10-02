package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestPrivateQueryRateIsProcessOnly(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	read := getAuthenticatedPath(server, cookies, settingsPath(apiSettingsGroupID))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(read.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, found := body["private_query_per_min"]; found {
		t.Fatal("group read exposes a process setting")
	}
	for _, value := range []string{"1", "null"} {
		response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
			`{"expected_revision":0,"changes":{"private_query_per_min":`+value+`}}`)
		if response.Code != http.StatusBadRequest || service.updateCalls != 0 {
			t.Fatalf("manager process-setting patch: status=%d writes=%d body=%s", response.Code, service.updateCalls, response.Body.String())
		}
	}
	config := loadProcessSettingsConfig(t, map[string]any{"private_query_per_min": 8})
	process := &apiTestProcessSettingsService{view: config.ProcessSettings()}
	for _, role := range []auth.Role{auth.RoleOperator, auth.RoleManager} {
		processServer, processCookies := processSettingsTestServer(t, role, process)
		response := processSettingsRequest(processServer, processCookies, http.MethodGet)
		if role == auth.RoleManager {
			if response.Code != http.StatusForbidden {
				t.Fatalf("manager process read: %d", response.Code)
			}
			continue
		}
		got := decodeProcessSettings(t, response).PrivateQueryPerMin
		if response.Code != http.StatusOK || got.Value != 8 || got.Source != settings.SourceUserFile.String() {
			t.Fatalf("operator process rate: status=%d value=%+v", response.Code, got)
		}
	}
}

func TestManagerCanRepairKnownChatCap(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	first := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"known_chat_ids":[-1009000000711,-1009000000712]}}`)
	if first.Code != http.StatusOK {
		t.Fatal(first.Body.String())
	}
	cap := int64(1)
	if _, err := service.store.UpdateOwnerLimits(0, settings.LimitChanges{"known_chat_ids": &cap}); err != nil {
		t.Fatal(err)
	}
	refused := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":1,"changes":{"enabled":false}}`)
	if refused.Code != http.StatusBadRequest || decodeError(refused) != "settings_limit_exceeded" {
		t.Fatal(refused.Body.String())
	}
	repaired := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":1,"changes":{"known_chat_ids":[-1009000000711]}}`)
	got := decodeSettings(t, repaired)
	if repaired.Code != http.StatusOK || got.Revision != 2 || len(got.KnownChatIDs.Value) != 1 || got.KnownChatIDs.Value[0] != -1009000000711 {
		t.Fatalf("repair status=%d body=%s", repaired.Code, repaired.Body.String())
	}
}
