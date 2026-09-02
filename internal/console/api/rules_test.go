package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/database"
	storedrules "github.com/Zakkaus/vestibule/internal/rules"
)

const apiRulesChatID int64 = -1009000000201

type apiRulesHarness struct {
	server  *Server
	cookies []*http.Cookie
	csrf    string
	store   *database.RuleStore
	checker *apiTestAdminChecker
}

func TestRulesRejectUnauthorizedChat(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		harness := newAPIRulesHarness(t, false)
		response := getAuthenticatedPath(harness.server, harness.cookies, rulesPath(apiRulesChatID))
		counts := harness.checker.counts()
		if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" ||
			counts.cachedCalls != 1 || counts.freshCalls != 0 {
			t.Fatalf("status=%d code=%s cached=%d fresh=%d, want 403, chat_access_denied, 1, 0",
				response.Code, decodeError(response), counts.cachedCalls, counts.freshCalls)
		}
	})

	t.Run("write", func(t *testing.T) {
		harness := newAPIRulesHarness(t, true)
		if response := getAuthenticatedPath(harness.server, harness.cookies, rulesPath(apiRulesChatID)); response.Code != http.StatusOK {
			t.Fatalf("initial read status = %d, want 200", response.Code)
		}
		harness.checker.setAllowed(false)
		response := putRuleCollection(t, harness, "future_collection", nil, sampleRuleInputs()[:1])
		counts := harness.checker.counts()
		records := listStoredRules(t, harness, "")
		if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" ||
			counts.cachedCalls != 1 || counts.freshCalls != 1 || len(records) != 0 {
			t.Fatalf("status=%d code=%s cached=%d fresh=%d rows=%d, want 403, chat_access_denied, 1, 1, 0",
				response.Code, decodeError(response), counts.cachedCalls, counts.freshCalls, len(records))
		}
	})
}

func TestRulesRequireCSRF(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	csrf := harness.csrf
	harness.csrf = ""
	response := putRuleCollection(t, harness, "future_collection", nil, sampleRuleInputs()[:1])
	harness.csrf = csrf
	counts := harness.checker.counts()
	records := listStoredRules(t, harness, "")
	if response.Code != http.StatusForbidden || decodeError(response) != "csrf_invalid" ||
		counts.freshCalls != 1 || len(records) != 0 {
		t.Fatalf("status=%d code=%s fresh=%d rows=%d, want 403, csrf_invalid, 1, 0",
			response.Code, decodeError(response), counts.freshCalls, len(records))
	}
}

func TestRuleItemRejectsUnauthorizedChat(t *testing.T) {
	harness := newAPIRulesHarness(t, false)
	expected, next := seedStoredRuleItem(t, harness)
	response := putRuleItem(t, harness, "future-a", expected, next)
	counts := harness.checker.counts()
	records := listStoredRules(t, harness, "future_collection")
	if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" ||
		counts.cachedCalls != 0 || counts.freshCalls != 1 || len(records) != 1 || !records[0].Enabled {
		t.Fatalf("status=%d code=%s cached=%d fresh=%d rows=%+v, want 403, chat_access_denied, 0, 1, enabled",
			response.Code, decodeError(response), counts.cachedCalls, counts.freshCalls, records)
	}
}

func TestRuleItemRequiresCSRF(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	expected, next := seedStoredRuleItem(t, harness)
	harness.csrf = ""
	response := putRuleItem(t, harness, "future-a", expected, next)
	counts := harness.checker.counts()
	records := listStoredRules(t, harness, "future_collection")
	if response.Code != http.StatusForbidden || decodeError(response) != "csrf_invalid" ||
		counts.freshCalls != 1 || len(records) != 1 || !records[0].Enabled {
		t.Fatalf("status=%d code=%s fresh=%d rows=%+v, want 403, csrf_invalid, 1, enabled",
			response.Code, decodeError(response), counts.freshCalls, records)
	}
}

func TestRulesReorderPreservesRowsAndUnknownCollections(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	future := sampleRuleInputs()
	created := putRuleCollection(t, harness, "future_collection", nil, future)
	if created.Code != http.StatusOK {
		t.Fatalf("create future collection status=%d code=%s", created.Code, decodeError(created))
	}
	automatic := []ruleInput{{ID: "auto-a", Enabled: true, Definition: json.RawMessage(`{"reply":"ok"}`)}}
	if response := putRuleCollection(t, harness, "auto_reply", nil, automatic); response.Code != http.StatusOK {
		t.Fatalf("create auto collection status=%d code=%s", response.Code, decodeError(response))
	}

	reordered := []ruleInput{future[2], future[0], future[1]}
	response := putRuleCollection(t, harness, "future_collection", future, reordered)
	items := decodeRuleList(t, response)
	requireRuleResponseOrder(t, items, []string{"future-c", "future-a", "future-b"})

	all := decodeRuleList(t, getAuthenticatedPath(harness.server, harness.cookies, rulesPath(apiRulesChatID)))
	seen := make(map[string]int)
	for _, item := range all {
		seen[item.Collection]++
	}
	if response.Code != http.StatusOK || len(all) != 4 || seen["future_collection"] != 3 || seen["auto_reply"] != 1 {
		t.Fatalf("status=%d rows=%d collections=%v, want 200 and future_collection=3 auto_reply=1",
			response.Code, len(all), seen)
	}

	stale := putRuleCollection(t, harness, "future_collection", future, future)
	if stale.Code != http.StatusConflict || decodeError(stale) != "rule_conflict" {
		t.Fatalf("stale replacement status=%d code=%s, want 409 rule_conflict", stale.Code, decodeError(stale))
	}
}

func TestRuleUpdateTogglesOnlyOneRowAndRepeatsAsNoop(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	inputs := sampleRuleInputs()[:2]
	if response := putRuleCollection(t, harness, "future_collection", nil, inputs); response.Code != http.StatusOK {
		t.Fatalf("create collection status=%d code=%s", response.Code, decodeError(response))
	}
	expected := ruleStateInput{
		Collection: "future_collection", Ordinal: 0, Enabled: true, Definition: inputs[0].Definition,
	}
	next := expected
	next.Enabled = false
	first := putRuleItem(t, harness, inputs[0].ID, expected, next)
	updated := decodeRuleItem(t, first)
	if first.Code != http.StatusOK || updated.ID != inputs[0].ID || updated.Enabled {
		t.Fatalf("first update status=%d item=%+v, want 200 and disabled", first.Code, updated)
	}

	expectedRecord := expected.record(apiRulesChatID, inputs[0].ID)
	nextRecord := next.record(apiRulesChatID, inputs[0].ID)
	_, changed, err := harness.store.UpdateRule(context.Background(), apiRulesChatID, expectedRecord, nextRecord)
	if err != nil || changed {
		t.Fatalf("duplicate store update changed=%v err=%v, want false and nil", changed, err)
	}
	second := putRuleItem(t, harness, inputs[0].ID, expected, next)
	if second.Code != http.StatusOK || decodeRuleItem(t, second).Enabled {
		t.Fatalf("duplicate endpoint update status=%d body=%s, want 200 and disabled", second.Code, second.Body.String())
	}

	records := listStoredRules(t, harness, "future_collection")
	if len(records) != 2 || records[0].Enabled || !records[1].Enabled {
		t.Fatalf("enabled states = %+v, want only future-a disabled", records)
	}
}

func TestRuleStoreRejectsUnparseableDefinition(t *testing.T) {
	harness := newAPIRulesHarness(t, true)
	record := storedrules.Record{
		ID: "bad", ChatID: apiRulesChatID, Collection: "future_collection", Ordinal: 0,
		Enabled: true, Definition: json.RawMessage(`{`),
	}
	_, _, err := harness.store.ReplaceRules(
		context.Background(), apiRulesChatID, "future_collection", nil, []storedrules.Record{record},
	)
	if !errors.Is(err, storedrules.ErrRuleInvalid) {
		t.Fatalf("invalid definition error = %v, want ErrRuleInvalid", err)
	}
}

func newAPIRulesHarness(t *testing.T, allowed bool) *apiRulesHarness {
	t.Helper()
	db, err := database.Open(context.Background(), database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(context.Background(),
		"INSERT INTO chat (id, title) VALUES ($1, 'rules test')", apiRulesChatID); err != nil {
		t.Fatal(err)
	}
	checker := &apiTestAdminChecker{allowed: allowed}
	groups := &apiTestQueueService{groups: []int64{apiRulesChatID}}
	server, cookies, csrf := apiTestServer(t, checker, groups, nil)
	store := database.NewRuleStore(db)
	server.ReplaceRoutes(Config{Authenticator: server.routes.Load().server.authenticator, Verification: groups, Rules: store})
	return &apiRulesHarness{server: server, cookies: cookies, csrf: csrf, store: store, checker: checker}
}

func seedStoredRuleItem(t *testing.T, harness *apiRulesHarness) (ruleStateInput, ruleStateInput) {
	t.Helper()
	input := sampleRuleInputs()[0]
	record := storedrules.Record{
		ID: input.ID, ChatID: apiRulesChatID, Collection: "future_collection", Ordinal: 0,
		Enabled: input.Enabled, Definition: input.Definition,
	}
	if _, _, err := harness.store.ReplaceRules(
		context.Background(), apiRulesChatID, "future_collection", nil, []storedrules.Record{record},
	); err != nil {
		t.Fatal(err)
	}
	expected := ruleStateInput{
		Collection: record.Collection, Ordinal: record.Ordinal, Enabled: record.Enabled, Definition: record.Definition,
	}
	next := expected
	next.Enabled = false
	return expected, next
}

func sampleRuleInputs() []ruleInput {
	return []ruleInput{
		{ID: "future-a", Enabled: true, Definition: json.RawMessage(`{"message":"alpha"}`)},
		{ID: "future-b", Enabled: true, Definition: json.RawMessage(`["beta"]`)},
		{ID: "future-c", Enabled: true, Definition: json.RawMessage(`"gamma"`)},
	}
}

func rulesPath(chatID int64) string {
	return "/api/chats/" + strconv.FormatInt(chatID, 10) + "/rules"
}

func putRuleCollection(
	t *testing.T,
	harness *apiRulesHarness,
	collection string,
	expected []ruleInput,
	items []ruleInput,
) *httptest.ResponseRecorder {
	t.Helper()
	if expected == nil {
		expected = []ruleInput{}
	}
	if items == nil {
		items = []ruleInput{}
	}
	body := replaceRulesRequest{Collection: collection, Expected: &expected, Items: &items}
	return putRulesJSON(t, harness, rulesPath(apiRulesChatID), body)
}

func putRuleItem(
	t *testing.T,
	harness *apiRulesHarness,
	ruleID string,
	expected ruleStateInput,
	item ruleStateInput,
) *httptest.ResponseRecorder {
	t.Helper()
	body := updateRuleRequest{Expected: &expected, Item: &item}
	return putRulesJSON(t, harness, rulesPath(apiRulesChatID)+"/"+ruleID, body)
}

func putRulesJSON(t *testing.T, harness *apiRulesHarness, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", harness.csrf)
	for _, cookie := range harness.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	harness.server.Handler().ServeHTTP(response, request)
	return response
}

func decodeRuleList(t *testing.T, response *httptest.ResponseRecorder) []ruleResponse {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	payload, ok := envelope["items"]
	if !ok || len(envelope) != 1 {
		t.Fatalf("list response keys=%v, want only items", envelope)
	}
	var items []ruleResponse
	if err := json.Unmarshal(payload, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func decodeRuleItem(t *testing.T, response *httptest.ResponseRecorder) ruleResponse {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if _, wrapped := envelope["item"]; wrapped {
		t.Fatal("single rule response is wrapped in item")
	}
	var item ruleResponse
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	return item
}

func listStoredRules(t *testing.T, harness *apiRulesHarness, collection string) []storedrules.Record {
	t.Helper()
	records, err := harness.store.ListRules(context.Background(), apiRulesChatID, collection)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func requireRuleResponseOrder(t *testing.T, items []ruleResponse, ids []string) {
	t.Helper()
	if len(items) != len(ids) {
		t.Fatalf("rule count=%d, want %d", len(items), len(ids))
	}
	seenOrdinals := make(map[int]struct{}, len(items))
	for ordinal, item := range items {
		if item.ID != ids[ordinal] || item.Ordinal != ordinal {
			t.Fatalf("item %d = id %q ordinal %d, want id %q ordinal %d",
				ordinal, item.ID, item.Ordinal, ids[ordinal], ordinal)
		}
		seenOrdinals[item.Ordinal] = struct{}{}
	}
	if len(seenOrdinals) != len(items) {
		t.Fatalf("ordinals are not unique: %+v", items)
	}
}
