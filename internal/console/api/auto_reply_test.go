package api

import (
	"context"
	"encoding/json"
	"github.com/Zakkaus/vestibule/internal/rules"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAutoReplyWritesRejectInvalidDefinitions(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	valid := json.RawMessage(`{"trigger":"matrix","reply":"Bridge address"}`)
	original := []ruleInput{{ID: "auto-a", Enabled: true, Definition: valid}}
	if response := putRuleCollection(t, harness, "auto_reply", nil, original); response.Code != http.StatusOK {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	invalid := json.RawMessage(`{"trigger":"matrix","reply":"Bridge address","cooldown_seconds":0}`)
	expected := ruleStateInput{Collection: "auto_reply", Ordinal: 0, Enabled: true, Definition: valid}
	next := expected
	next.Definition = invalid
	responses := []struct {
		name, field string
		response    *httptest.ResponseRecorder
	}{
		{"item", "item.definition.cooldown_seconds", putRuleItem(t, harness, "auto-a", expected, next)},
		{"collection", "items[0].definition.cooldown_seconds",
			putRuleCollection(t, harness, "auto_reply", original, []ruleInput{{ID: "auto-a", Enabled: true, Definition: invalid}})},
	}
	for _, response := range responses {
		var envelope struct {
			Fields []struct {
				Name string `json:"name"`
				Code string `json:"code"`
			} `json:"fields"`
		}
		if err := json.Unmarshal(response.response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if response.response.Code != http.StatusBadRequest || len(envelope.Fields) != 1 ||
			envelope.Fields[0].Code != "invalid_cooldown" || envelope.Fields[0].Name != response.field {
			t.Fatalf("%s status=%d body=%s", response.name, response.response.Code, response.response.Body.String())
		}
	}
	records := listStoredRules(t, harness, "auto_reply")
	if len(records) != 1 || string(records[0].Definition) != string(valid) {
		t.Fatalf("invalid write changed rows: %+v", records)
	}
}

func TestAutoReplyUnchangedInvalidDefinitionsRemainManageable(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	inputs := []ruleInput{
		{ID: "legacy-a", Enabled: true, Definition: json.RawMessage(`{"reply":"ok"}`)},
		{ID: "legacy-b", Enabled: true, Definition: json.RawMessage(`["bridge",{"reply":"Use Matrix"}]`)},
	}
	if _, _, err := harness.store.ReplaceRules(context.Background(), apiRulesChatID, "auto_reply", nil,
		ruleRecords(apiRulesChatID, "auto_reply", inputs)); err != nil {
		t.Fatal(err)
	}
	expected := ruleStateInput{Collection: "auto_reply", Ordinal: 0, Enabled: true, Definition: inputs[0].Definition}
	next := expected
	next.Enabled = false
	response := putRuleItem(t, harness, inputs[0].ID, expected, next)
	if response.Code != http.StatusOK || decodeRuleItem(t, response).Enabled {
		t.Fatalf("disable legacy rule: status=%d body=%s", response.Code, response.Body.String())
	}
	inputs[0].Enabled = false
	reordered := []ruleInput{inputs[1], inputs[0]}
	response = putRuleCollection(t, harness, "auto_reply", inputs, reordered)
	if response.Code != http.StatusOK {
		t.Fatalf("reorder legacy rules: status=%d body=%s", response.Code, response.Body.String())
	}
	items := decodeRuleList(t, response)
	requireRuleResponseOrder(t, items, []string{"legacy-b", "legacy-a"})
	if items[1].Enabled || string(items[0].Definition) != string(inputs[1].Definition) ||
		string(items[1].Definition) != string(inputs[0].Definition) {
		t.Fatalf("reorder changed definitions or enabled states: %+v", items)
	}
}

func TestAutoReplyForgedExpectedCannotCreateInvalidRule(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	inputs := []ruleInput{{ID: "new", Enabled: true, Definition: json.RawMessage(`{"reply":"ok"}`)}}
	if response := putRuleCollection(t, harness, "auto_reply", nil, inputs); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid creation status=%d body=%s", response.Code, response.Body.String())
	}
	if response := putRuleCollection(t, harness, "auto_reply", inputs, inputs); response.Code != http.StatusConflict {
		t.Fatalf("forged expected status=%d body=%s", response.Code, response.Body.String())
	}
	if records := listStoredRules(t, harness, "auto_reply"); len(records) != 0 {
		t.Fatalf("forged expected created invalid rule: %+v", records)
	}
}

func TestAutoReplyMalformedStoredJSONReturnsErrorBeforeHeaders(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	record := rules.Record{ID: "corrupt", ChatID: apiRulesChatID, Collection: "auto_reply",
		Enabled: true, Definition: json.RawMessage(`{"trigger":"matrix","reply":"ok"}`)}
	if _, _, err := harness.store.ReplaceRules(context.Background(), apiRulesChatID, "auto_reply", nil,
		[]rules.Record{record}); err != nil {
		t.Fatal(err)
	}
	if _, err := harness.db.Exec(context.Background(), "UPDATE rule SET definition=$1 WHERE id='corrupt'", `{`); err != nil {
		t.Fatal(err)
	}
	response := getAuthenticatedPath(harness.server, harness.cookies, rulesPath(apiRulesChatID))
	if response.Code != http.StatusInternalServerError || !json.Valid(response.Body.Bytes()) ||
		decodeError(response) != "rules_unavailable" {
		t.Fatalf("corrupt read status=%d body=%s", response.Code, response.Body.String())
	}
}
