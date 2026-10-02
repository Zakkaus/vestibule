package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const apiControlChatID int64 = -1009000000641

func TestSettingsControlChatSetClearAndReject(t *testing.T) {
	for _, tc := range []struct {
		name   string
		chat   int64
		member bool
		status int
	}{
		{"set", apiControlChatID, true, http.StatusOK},
		{"self", apiSettingsGroupID, true, http.StatusBadRequest},
		{"bot absent", apiControlChatID, false, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
			service.store.SetControlChatMembership(func(int64, int64) error {
				if !tc.member {
					return errors.New("not a member")
				}
				return nil
			})
			body := fmt.Sprintf(`{"expected_revision":0,"changes":{"control_chat_id":%d}}`, tc.chat)
			response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID, body)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.status != http.StatusOK {
				group, _ := service.store.Settings(apiSettingsGroupID)
				if decodeError(response) != "control_chat_invalid" || group.Revision() != 0 || group.ControlChatID().Value != 0 {
					t.Fatal("invalid control assignment changed settings")
				}
				return
			}
			if decodeSettings(t, response).ControlChatID.Value != apiControlChatID {
				t.Fatal("control assignment not saved")
			}
			clear := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID, `{"expected_revision":1,"changes":{"control_chat_id":0}}`)
			if clear.Code != http.StatusOK || decodeSettings(t, clear).ControlChatID.Value != 0 {
				t.Fatalf("clear=%d %s", clear.Code, clear.Body.String())
			}
			_, found := service.store.ControlGroup(apiControlChatID)
			if found {
				t.Fatal("cleared assignment still resolves")
			}
		})
	}
}

func TestSettingsControlChatRound2ConflictIsGeneric(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	const otherGroup int64 = -1009000000642
	baseline := service.baseline
	other := baseline.Groups[0]
	other.ID = otherGroup
	other.ControlChatID = settings.BaselineValue[int64]{Value: apiControlChatID, Source: settings.SourceUserFile}
	baseline.Groups = append(baseline.Groups, other)
	store, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service.store = store
	response := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID, `{"expected_revision":0,"changes":{"control_chat_id":-1009000000641}}`)
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusConflict || decodeError(response) != "control_chat_conflict" || len(payload) != 1 || payload["error"] == nil {
		t.Fatalf("conflict=%d %s", response.Code, response.Body.String())
	}
	group, _ := store.Settings(apiSettingsGroupID)
	if group.Revision() != 0 || group.ControlChatID().Value != 0 {
		t.Fatal("conflicting write was published")
	}
}

func TestSettingsControlChatRound2UsesSessionAuthority(t *testing.T) {
	server, cookies, csrf, service, _ := apiSettingsTestServer(t, true)
	rights := map[[2]int64]bool{{apiControlChatID, 9}: true}
	service.store.SetControlChatMembership(func(chatID, actorID int64) error {
		if !rights[[2]int64{chatID, actorID}] {
			return errors.New("not a live control-chat administrator")
		}
		return nil
	})
	set := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":0,"changes":{"control_chat_id":-1009000000641}}`)
	if set.Code != http.StatusOK {
		t.Fatalf("control-chat administrator refused: %d %s", set.Code, set.Body.String())
	}
	delete(rights, [2]int64{apiControlChatID, 9})
	hijack := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":1,"changes":{"control_chat_id":-1009000000643}}`)
	group, _ := service.store.Settings(apiSettingsGroupID)
	if hijack.Code != http.StatusBadRequest || decodeError(hijack) != "control_chat_invalid" ||
		group.ControlChatID().Value != apiControlChatID || group.Revision() != 1 {
		t.Fatalf("unauthorized reassignment: status=%d group=%d revision=%d", hijack.Code, group.ControlChatID().Value, group.Revision())
	}
	unrelated := patchGroupSettings(server, cookies, csrf, apiSettingsGroupID,
		`{"expected_revision":1,"changes":{"warn_limit":3}}`)
	if unrelated.Code != http.StatusOK || decodeSettings(t, unrelated).WarnLimit.Value != 3 {
		t.Fatalf("unrelated settings blocked after control rights revoked: %d %s", unrelated.Code, unrelated.Body.String())
	}
}
