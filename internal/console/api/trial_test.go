package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const trialTestChatID int64 = -1009000000901

type trialHarness struct {
	server   *Server
	cookies  []*http.Cookie
	csrf     string
	settings *settings.Store
	checker  *apiTestAdminChecker
}

func newTrialHarness(t *testing.T, allowed bool) *trialHarness {
	t.Helper()
	store := newTrialSettingsStore(t, nil)
	checker := &apiTestAdminChecker{allowed: allowed}
	groups := &apiTestQueueService{groups: []int64{trialTestChatID}}
	server, cookies, csrf := apiTestServer(t, checker, groups, nil, store)
	return &trialHarness{server: server, cookies: cookies, csrf: csrf, settings: store, checker: checker}
}

func newTrialSettingsStore(t *testing.T, repository settings.Repository) *settings.Store {
	return newTrialSettingsStoreForChat(t, trialTestChatID, repository)
}

func newTrialSettingsStoreForChat(t *testing.T, chatID int64, repository settings.Repository) *settings.Store {
	t.Helper()
	fallbackBuiltin := false
	fallback := []settings.ShortQuestion{{
		Q: "Name a package manager", Answers: []string{"Portage", "emerge"},
	}}
	baseline, err := settings.LoadBaseline("", &settings.Config{
		Groups: []settings.GroupConfig{{
			ID: chatID,
			Questions: []settings.Question{{
				Q: "Pick the triangle", Options: []string{"triangle", "square"}, Answer: 0,
			}},
			FallbackQuestions: &fallback,
			FallbackBuiltin:   &fallbackBuiltin,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, repository, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func postTrialRequest(
	t *testing.T,
	server *Server,
	cookies []*http.Cookie,
	csrf string,
	chatID int64,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/chats/" + fmt.Sprint(chatID) + "/rules/test"
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}

func trialCorrect(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	var body struct {
		Correct *bool `json:"correct"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Correct == nil {
		t.Fatalf("trial response omitted correct: %s", response.Body.String())
	}
	return *body.Correct
}

func TestQuestionTrialReturnsSavedQuestionAndFallbackResults(t *testing.T) {
	harness := newTrialHarness(t, true)
	cases := []struct {
		name   string
		body   string
		status int
		answer bool
	}{
		{
			name:   "quiz correct",
			body:   `{"collection":"questions","question_index":0,"expected_revision":0,"choice":0}`,
			status: http.StatusOK, answer: true,
		},
		{
			name:   "quiz incorrect",
			body:   `{"collection":"questions","question_index":0,"expected_revision":0,"choice":1}`,
			status: http.StatusOK, answer: false,
		},
		{
			name:   "fallback normalized correct",
			body:   `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":"HTTPS://PORTAGE/"}`,
			status: http.StatusOK, answer: true,
		},
		{
			name:   "fallback incorrect",
			body:   `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":"dnf"}`,
			status: http.StatusOK, answer: false,
		},
		{
			name:   "fallback empty answer",
			body:   `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":""}`,
			status: http.StatusOK, answer: false,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := postTrialRequest(t, harness.server, harness.cookies, harness.csrf, trialTestChatID, test.body)
			if response.Code != test.status || trialCorrect(t, response) != test.answer {
				t.Fatalf("status=%d answer=%v body=%s, want status=%d answer=%v",
					response.Code, trialCorrect(t, response), response.Body.String(), test.status, test.answer)
			}
		})
	}
}

func TestQuestionTrialRejectsInvalidRequestsAndPreservesPrecedence(t *testing.T) {
	harness := newTrialHarness(t, true)
	cases := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{"malformed JSON", `{`, http.StatusBadRequest, "invalid_json"},
		{"unknown field", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":0,"extra":true}`, http.StatusBadRequest, "invalid_json"},
		{"wrong JSON type", `{"collection":"questions","question_index":"0","expected_revision":0,"choice":0}`, http.StatusBadRequest, "invalid_json"},
		{"missing question index", `{"collection":"questions","expected_revision":0,"choice":0}`, http.StatusBadRequest, "invalid_rule"},
		{"null question index", `{"collection":"questions","question_index":null,"expected_revision":0,"choice":0}`, http.StatusBadRequest, "invalid_rule"},
		{"negative question index", `{"collection":"questions","question_index":-1,"expected_revision":0,"choice":0}`, http.StatusBadRequest, "invalid_rule"},
		{"missing revision", `{"collection":"questions","question_index":0,"choice":0}`, http.StatusBadRequest, "invalid_rule"},
		{"null revision", `{"collection":"questions","question_index":0,"expected_revision":null,"choice":0}`, http.StatusBadRequest, "invalid_rule"},
		{"wrong discriminator", `{"collection":"other","question_index":0,"expected_revision":0,"answer":"Portage"}`, http.StatusBadRequest, "invalid_rule"},
		{"quiz answer field", `{"collection":"questions","question_index":0,"expected_revision":0,"answer":"triangle"}`, http.StatusBadRequest, "invalid_rule"},
		{"quiz null answer field", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":0,"answer":null}`, http.StatusBadRequest, "invalid_rule"},
		{"quiz null choice", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":null}`, http.StatusBadRequest, "invalid_rule"},
		{"fallback choice field", `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"choice":0,"answer":"Portage"}`, http.StatusBadRequest, "invalid_rule"},
		{"fallback null choice", `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"choice":null,"answer":"Portage"}`, http.StatusBadRequest, "invalid_rule"},
		{"fallback missing answer", `{"collection":"fallback_questions","question_index":0,"expected_revision":0}`, http.StatusBadRequest, "invalid_rule"},
		{"fallback null answer", `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":null}`, http.StatusBadRequest, "invalid_rule"},
		{"quiz choice out of range", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":2}`, http.StatusBadRequest, "invalid_rule"},
		{"missing question", `{"collection":"questions","question_index":1,"expected_revision":0,"choice":0}`, http.StatusNotFound, "rule_not_found"},
		{"missing fallback question", `{"collection":"fallback_questions","question_index":1,"expected_revision":0,"answer":"Portage"}`, http.StatusNotFound, "rule_not_found"},
		{"stale revision precedes missing question", `{"collection":"questions","question_index":1,"expected_revision":99,"choice":0}`, http.StatusConflict, "settings_conflict"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := postTrialRequest(t, harness.server, harness.cookies, harness.csrf, trialTestChatID, test.body)
			if response.Code != test.status || decodeError(response) != test.code {
				t.Fatalf("status=%d code=%q body=%s, want status=%d code=%q",
					response.Code, decodeError(response), response.Body.String(), test.status, test.code)
			}
		})
	}
}

func TestQuestionTrialRequiresWriteAccessCSRFAndCurrentChat(t *testing.T) {
	body := `{"collection":"questions","question_index":0,"expected_revision":0,"choice":0}`
	denied := newTrialHarness(t, false)
	response := postTrialRequest(t, denied.server, denied.cookies, denied.csrf, trialTestChatID, body)
	if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" {
		t.Fatalf("unauthorized status=%d code=%q, want 403 chat_access_denied", response.Code, decodeError(response))
	}

	harness := newTrialHarness(t, true)
	read := getAuthenticatedPath(harness.server, harness.cookies, settingsPath(trialTestChatID))
	if read.Code != http.StatusOK {
		t.Fatalf("initial settings read status=%d, want 200", read.Code)
	}
	harness.checker.setAllowed(false)
	response = postTrialRequest(t, harness.server, harness.cookies, harness.csrf, trialTestChatID, body)
	if response.Code != http.StatusForbidden || decodeError(response) != "chat_access_denied" {
		t.Fatalf("revoked administrator status=%d code=%q, want 403 chat_access_denied", response.Code, decodeError(response))
	}
	harness.checker.setAllowed(true)
	response = postTrialRequest(t, harness.server, harness.cookies, "", trialTestChatID, body)
	if response.Code != http.StatusForbidden || decodeError(response) != "csrf_invalid" {
		t.Fatalf("missing CSRF status=%d code=%q, want 403 csrf_invalid", response.Code, decodeError(response))
	}

	response = postTrialRequest(t, harness.server, harness.cookies, harness.csrf, trialTestChatID-1, body)
	if response.Code != http.StatusNotFound || decodeError(response) != "chat_not_found" {
		t.Fatalf("other chat status=%d code=%q, want 404 chat_not_found", response.Code, decodeError(response))
	}
}

func TestQuestionTrialLeavesEveryApplicationTableUnchanged(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repository := database.NewSettingsStore(db)
	if err := repository.SeedSettings([]settings.Record{{ChatID: trialTestChatID}}); err != nil {
		t.Fatal(err)
	}
	store := newTrialSettingsStore(t, repository)
	seedTrialStateRows(t, db)
	checker := &apiTestAdminChecker{allowed: true}
	groups := &apiTestQueueService{groups: []int64{trialTestChatID}}
	server, cookies, csrf := apiTestServer(t, checker, groups, nil, store)

	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"correct", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":0}`, http.StatusOK},
		{"incorrect", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":1}`, http.StatusOK},
		{"fallback", `{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":"wrong"}`, http.StatusOK},
		{"missing", `{"collection":"questions","question_index":9,"expected_revision":0,"choice":0}`, http.StatusNotFound},
		{"conflict", `{"collection":"questions","question_index":9,"expected_revision":8,"choice":0}`, http.StatusConflict},
		{"invalid", `{"collection":"questions","question_index":0,"expected_revision":0,"choice":9}`, http.StatusBadRequest},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := applicationSnapshot(t, db)
			response := postTrialRequest(t, server, cookies, csrf, trialTestChatID, test.body)
			if response.Code != test.status {
				t.Fatalf("status=%d code=%q, want %d", response.Code, decodeError(response), test.status)
			}
			after := applicationSnapshot(t, db)
			if before != after {
				t.Fatalf("trial changed application tables:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func seedTrialStateRows(t *testing.T, db *database.Database) {
	t.Helper()
	ctx := context.Background()
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO challenge (id, chat_id, user_id, state, kind, payload, delivery, attempts, reason, expires_at, settled_at, settled_by, epoch) VALUES ('trial-challenge', $1, 77, 'declined', 'quiz', '{}', 'group', 1, 'wrong', 100, 200, 9, 3)`, []any{trialTestChatID}},
		{`INSERT INTO pending_action (id, challenge_id, kind, payload, state, attempts, next_try_at, claim_owner, claim_until, done_at, failed_at, last_error) VALUES ('trial-action', 'trial-challenge', 'delete', '{}', 'done', 1, 10, 'trial', 11, 12, 13, 'none')`, nil},
		{`INSERT INTO update_poll_lease (singleton, holder, expires_at) VALUES (1, 'trial', 20)`, nil},
		{`INSERT INTO rule (id, chat_id, collection, ordinal, enabled, definition) VALUES ('trial-rule', $1, 'allowlist', 0, 1, '{}')`, []any{trialTestChatID}},
		{`INSERT INTO verification_failure (chat_id, user_id, count, last_at) VALUES ($1, 77, 2, 30)`, []any{trialTestChatID}},
		{`INSERT INTO agent_tally (model, count) VALUES ('trial-model', 4)`, nil},
		{`INSERT INTO verification_runtime (key, value) VALUES ('trial-runtime', 5)`, nil},
		{`INSERT INTO warning_counter (chat_id, user_id, count) VALUES ($1, 77, 3)`, []any{trialTestChatID}},
	}
	for _, item := range queries {
		if _, err := db.Exec(ctx, item.query, item.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func applicationSnapshot(t *testing.T, db *database.Database) string {
	t.Helper()
	ctx := context.Background()
	rows, err := db.Query(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()

	var snapshot strings.Builder
	for _, table := range tables {
		result, err := db.Query(ctx, `SELECT * FROM "`+strings.ReplaceAll(table, `"`, `""`)+`"`)
		if err != nil {
			t.Fatal(err)
		}
		var tableRows []string
		columns, err := result.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for result.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := result.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			fields := make([]string, len(values))
			for i, value := range values {
				fields[i] = fmt.Sprintf("%T:%v", value, value)
			}
			tableRows = append(tableRows, strings.Join(fields, ","))
		}
		if err := result.Err(); err != nil {
			t.Fatal(err)
		}
		result.Close()
		sort.Strings(tableRows)
		snapshot.WriteString(table)
		snapshot.WriteByte('=')
		snapshot.WriteString(strings.Join(tableRows, ";"))
		snapshot.WriteByte('\n')
	}
	return snapshot.String()
}

type trialParityGateway struct {
	verification.Gateway
	lastMessage verification.OutgoingMessage
	approves    int
	declines    int
}

func (g *trialParityGateway) Send(_ context.Context, message verification.OutgoingMessage) (int, error) {
	g.lastMessage = message
	return 1, nil
}

func (g *trialParityGateway) SendHTMLFallback(ctx context.Context, chatID int64, rich, _ string) (int, error) {
	return g.Send(ctx, verification.OutgoingMessage{ChatID: chatID, Text: rich, HTML: true})
}

func (g *trialParityGateway) Delete(context.Context, int64, int) error { return nil }

func (g *trialParityGateway) ApproveJoin(context.Context, int64, int64) error {
	g.approves++
	return nil
}

func (g *trialParityGateway) DeclineJoin(context.Context, int64, int64) error {
	g.declines++
	return nil
}

func (g *trialParityGateway) AckResult(context.Context, string, verification.AckResult) error {
	return nil
}

func TestQuestionTrialMatchesObservedProductionQuiz(t *testing.T) {
	for _, choice := range []int{0, 1} {
		t.Run(fmt.Sprintf("quiz choice %d", choice), func(t *testing.T) {
			gateway := &trialParityGateway{}
			service, store := newTrialParityService(t, gateway)
			const userID int64 = 7001
			if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), verification.Update{
				ChatJoinRequest: &verification.ChatJoinRequest{
					Chat: verification.Chat{ID: trialTestChatID, Type: verification.ChatTypeSupergroup},
					From: verification.User{ID: userID, FirstName: "Trial", LanguageCode: "en"},
				},
			}); err != nil {
				t.Fatal(err)
			}
			service.SendDMChallenge(context.Background(), userID, "en", trialTestChatID)
			savedOptions := []string{"triangle", "square"}
			targetOption := savedOptions[choice]
			callback := ""
			for _, row := range gateway.lastMessage.Buttons {
				if len(row) != 0 && row[0].Text == targetOption {
					callback = row[0].CallbackData
					break
				}
			}
			if callback == "" {
				t.Fatalf("production quiz did not expose saved option %q: %+v", targetOption, gateway.lastMessage.Buttons)
			}

			checker := &apiTestAdminChecker{allowed: true}
			server, cookies, csrf := apiTestServer(t, checker, service, nil, store)
			body := fmt.Sprintf(`{"collection":"questions","question_index":0,"expected_revision":0,"choice":%d}`, choice)
			trial := postTrialRequest(t, server, cookies, csrf, trialTestChatID, body)
			if trial.Code != http.StatusOK {
				t.Fatalf("trial status=%d code=%q", trial.Code, decodeError(trial))
			}
			trialResult := trialCorrect(t, trial)

			if err := service.OnAnswer(verification.NewHandlerContext(context.Background(), gateway), verification.Update{
				CallbackQuery: &verification.CallbackQuery{
					ID: "trial-parity", From: verification.User{ID: userID, LanguageCode: "en"}, Data: callback,
				},
			}); err != nil {
				t.Fatal(err)
			}
			productionResult := gateway.approves == 1
			if trialResult != productionResult {
				t.Fatalf("trial result=%v production result=%v", trialResult, productionResult)
			}
		})
	}

}

func TestQuestionTrialMatchesObservedProductionFallback(t *testing.T) {
	for _, answer := range []string{"PORTAGE", "HTTPS://PORTAGE/", "dnf"} {
		t.Run("fallback "+answer, func(t *testing.T) {
			gateway := &trialParityGateway{}
			service, store := newTrialKernelParityService(t, gateway)
			const userID int64 = 7002
			if err := service.OnJoinRequest(verification.NewHandlerContext(context.Background(), gateway), verification.Update{
				ChatJoinRequest: &verification.ChatJoinRequest{
					Chat: verification.Chat{ID: trialTestChatID, Type: verification.ChatTypeSupergroup},
					From: verification.User{ID: userID, FirstName: "Trial", LanguageCode: "en"},
				},
			}); err != nil {
				t.Fatal(err)
			}
			service.SendDMChallenge(context.Background(), userID, "en", trialTestChatID)
			noLinux := fmt.Sprintf("还没装 Linux %d", time.Now().Minute())
			user := &verification.User{ID: userID, LanguageCode: "en"}
			if err := service.OnKernelAnswer(verification.NewHandlerContext(context.Background(), gateway), verification.Update{
				Message: &verification.Message{From: user, Text: noLinux},
			}); err != nil {
				t.Fatal(err)
			}
			if err := service.OnKernelAnswer(verification.NewHandlerContext(context.Background(), gateway), verification.Update{
				Message: &verification.Message{From: user, Text: answer},
			}); err != nil {
				t.Fatal(err)
			}
			productionResult := gateway.approves == 1

			checker := &apiTestAdminChecker{allowed: true}
			server, cookies, csrf := apiTestServer(t, checker, service, nil, store)
			body := fmt.Sprintf(`{"collection":"fallback_questions","question_index":0,"expected_revision":0,"answer":%q}`, answer)
			trial := postTrialRequest(t, server, cookies, csrf, trialTestChatID, body)
			if trial.Code != http.StatusOK {
				t.Fatalf("trial status=%d code=%q", trial.Code, decodeError(trial))
			}
			if trialResult := trialCorrect(t, trial); trialResult != productionResult {
				t.Fatalf("trial result=%v production kernel result=%v", trialResult, productionResult)
			}
		})
	}
}

func newTrialParityService(t *testing.T, gateway verification.Gateway) (*verification.Service, *settings.Store) {
	t.Helper()
	enabled := true
	mode := settings.ModeQuiz
	delivery := settings.DeliveryGroup
	cfg := &settings.Config{Groups: []settings.GroupConfig{{
		ID: trialTestChatID, Enabled: &enabled, VerifyMode: mode, DeliveryMode: delivery,
		Questions: []settings.Question{{Q: "Pick the triangle", Options: []string{"triangle", "square"}, Answer: 0}},
	}}}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(store, gateway, nil, cfg, &i18n.Messages, nil, verification.Identity{}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func newTrialKernelParityService(t *testing.T, gateway verification.Gateway) (*verification.Service, *settings.Store) {
	t.Helper()
	enabled := true
	mode := settings.ModeKernel
	delivery := settings.DeliveryGroup
	fallbackBuiltin := false
	fallback := []settings.ShortQuestion{{
		Q: "Name a package manager", Answers: []string{"Portage", "emerge"},
	}}
	cfg := &settings.Config{Groups: []settings.GroupConfig{{
		ID: trialTestChatID, Enabled: &enabled, VerifyMode: mode, DeliveryMode: delivery,
		FallbackQuestions: &fallback, FallbackBuiltin: &fallbackBuiltin,
	}}}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(store, gateway, nil, cfg, &i18n.Messages, nil, verification.Identity{}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}
