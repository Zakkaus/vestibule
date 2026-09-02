package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	consoleapi "github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestNewServicesFailsWhenPendingStateCannotLoad(t *testing.T) {
	ctx := context.Background()
	stateDirectory := t.TempDir()
	db, err := database.Open(ctx, database.Config{StateDirectory: stateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "DROP TABLE challenge"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
	}))
	t.Cleanup(api.Close)

	runtime, err := newServices(ctx, Options{
		ConfigPath:     filepath.Join(stateDirectory, "missing-config.json"),
		StateDirectory: stateDirectory,
		Token:          "1:" + strings.Repeat("a", 35),
		TelegramAPIURL: api.URL,
	}, make(chan struct{}, 1))
	if runtime != nil {
		t.Cleanup(func() { _ = runtime.database.Close() })
	}
	t.Logf("newServices error=%v runtime_nil=%t", err, runtime == nil)
	if err == nil {
		t.Fatal("newServices continued after LoadPending failed")
	}
	if runtime != nil {
		t.Fatal("newServices returned a runnable service graph after LoadPending failed")
	}
	if !strings.Contains(err.Error(), "load pending challenges") {
		t.Fatalf("startup error %q does not identify the failed authoritative query", err)
	}
}

func TestNewServicesAllowsAllOptionalModulesDisabled(t *testing.T) {
	ctx := context.Background()
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"disabled_modules":["gentoo","linux"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
	}))
	t.Cleanup(api.Close)

	runtime, err := newServices(ctx, Options{
		ConfigPath:     configPath,
		StateDirectory: stateDirectory,
		Token:          "1:" + strings.Repeat("a", 35),
		TelegramAPIURL: api.URL,
	}, make(chan struct{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		runtime.verification.Shutdown()
		_ = runtime.database.Close()
	})
	if runtime.modules.commands.HasPrivateQueries() {
		t.Fatal("all-disabled service graph still accepts private lookup commands")
	}
	if done := runtime.modules.Start(ctx); done != nil {
		t.Fatal("all-disabled service graph started optional background services")
	}
	for module, names := range optionalModuleCommands {
		for _, name := range names {
			if routeCommandNames(runtime.modules.commands.Definitions())[name] {
				t.Errorf("all-disabled service graph registered %s command /%s", module, name)
			}
		}
	}
	if runtime.cfg.ModuleEnabled(settings.ModuleGentoo) || runtime.cfg.ModuleEnabled(settings.ModuleLinux) {
		t.Fatal("all-disabled configuration was not retained by the service graph")
	}
}

func TestNewServicesAllowsEmptyGroups(t *testing.T) {
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	writeStartupConfig(t, configPath, `{"groups":[]}`)
	var commandMenus atomic.Int32
	telegramAPI := newStartupTelegramAPI(t, func() { commandMenus.Add(1) })

	runtime := openStartupTestServices(t, Options{
		ConfigPath:     configPath,
		StateDirectory: stateDirectory,
		Token:          "1:" + strings.Repeat("a", 35),
		TelegramAPIURL: telegramAPI.URL,
	})
	if groups := runtime.settings.ChatIDs(); len(groups) != 0 {
		t.Fatalf("zero-group service graph exposed groups %v", groups)
	}
	runtime.updates.SetupCommands(context.Background(), runtime.bot)
	if got := commandMenus.Load(); got != 6 {
		t.Fatalf("zero-group command menu registrations = %d, want 6 default scopes", got)
	}

	runtime.health.SetTelegramReady(true)
	console := consoleapi.New(consoleapi.Config{Health: runtime.health})
	for _, path := range []string{"/livez", "/readyz"} {
		response := httptest.NewRecorder()
		console.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s with zero groups = %d, want 200: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestRemovingConfiguredGroupRetainsTenantRows(t *testing.T) {
	const (
		remainingGroup = int64(-1009000000801)
		removedGroup   = int64(-1009000000802)
	)
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	writeStartupConfig(t, configPath, `{"groups":[{"id":-1009000000801},{"id":-1009000000802}]}`)
	telegramAPI := newStartupTelegramAPI(t, nil)
	options := Options{
		ConfigPath: configPath, StateDirectory: stateDirectory,
		Token: "1:" + strings.Repeat("a", 35), TelegramAPIURL: telegramAPI.URL,
	}

	initial := openStartupTestServices(t, options)
	removed, ok := initial.settings.Settings(removedGroup)
	if !ok {
		t.Fatal("removed-group fixture was not configured")
	}
	disabled := false
	overrides := removed.Overrides()
	overrides.Enabled = &disabled
	if _, err := initial.settings.Update(removedGroup, removed.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	initial.stopVerification()
	seedRemovedTenantRows(t, initial.services, removedGroup)
	before := removedTenantRows(t, initial.database, removedGroup)
	initial.close()

	writeStartupConfig(t, configPath, `{"groups":[{"id":-1009000000801}]}`)
	withoutRemoved := openStartupTestServices(t, options)
	after := removedTenantRows(t, withoutRemoved.database, removedGroup)
	if after != before {
		t.Fatalf("removing group configuration changed its rows: before=%+v after=%+v", before, after)
	}
	assertRemovedTenantHidden(t, withoutRemoved.services, remainingGroup, removedGroup)
	var pendingState string
	if err := withoutRemoved.database.QueryRow(context.Background(),
		"SELECT state FROM challenge WHERE chat_id=$1 AND user_id=$2", removedGroup, 4101).Scan(&pendingState); err != nil {
		t.Fatal(err)
	}
	t.Logf("removed group's pending challenge remains with state %q", pendingState)
	withoutRemoved.close()

	writeStartupConfig(t, configPath, `{"groups":[{"id":-1009000000801},{"id":-1009000000802}]}`)
	restored := openStartupTestServices(t, options)
	assertRemovedTenantRestored(t, restored.services, removedGroup)
}

type startupTestServices struct {
	*services
	t       *testing.T
	stopped bool
	closed  bool
}

func openStartupTestServices(t *testing.T, options Options) *startupTestServices {
	t.Helper()
	runtime, err := newServices(context.Background(), options, make(chan struct{}, 1))
	if err != nil {
		t.Fatal(err)
	}
	handle := &startupTestServices{services: runtime, t: t}
	t.Cleanup(handle.close)
	return handle
}

func (s *startupTestServices) stopVerification() {
	if !s.stopped {
		s.verification.Shutdown()
		s.stopped = true
	}
}

func (s *startupTestServices) close() {
	if s.closed {
		return
	}
	s.stopVerification()
	if err := s.database.Close(); err != nil {
		s.t.Errorf("close startup test database: %v", err)
	}
	s.closed = true
}

func newStartupTelegramAPI(t *testing.T, commandMenu func()) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.Path, "/getMe"):
			_, _ = io.WriteString(writer,
				`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
		case strings.HasSuffix(request.URL.Path, "/setMyCommands"):
			if commandMenu != nil {
				commandMenu()
			}
			_, _ = io.WriteString(writer, `{"ok":true,"result":true}`)
		default:
			http.Error(writer, `{"ok":false}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeStartupConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

type tenantRowCounts struct {
	chats      int
	challenges int
	actions    int
	rules      int
	failures   int
	warnings   int
}

func seedRemovedTenantRows(t *testing.T, runtime *services, groupID int64) {
	t.Helper()
	ctx := context.Background()
	state := database.NewVerificationStore(runtime.database)
	pending := verification.PendingRecord{
		GroupID: groupID, UserID: 4101, Nonce: "retained-pending", Mode: settings.ModeKernel,
		CreatedAt: time.Now().Unix(), Deadline: time.Now().Add(time.Hour).Unix(),
	}
	actionSource := verification.PendingRecord{
		GroupID: groupID, UserID: 4102, Nonce: "retained-action", Mode: settings.ModeKernel,
		CreatedAt: time.Now().Unix(), Deadline: time.Now().Add(time.Hour).Unix(),
	}
	for _, record := range []verification.PendingRecord{pending, actionSource} {
		if inserted, err := state.InsertPending("database", record); err != nil || !inserted {
			t.Fatalf("insert removed-tenant challenge = %t, %v", inserted, err)
		}
	}
	if changed, err := state.TransitionChallenge("database", verification.ChallengeTransition{
		Expected: actionSource.Ref(), Record: actionSource,
		From: verification.ChallengePending, To: verification.ChallengeDeclined, SettledAt: time.Now().Unix(),
		Actions: []verification.ActionIntent{{
			ID: "retained-action", Kind: "settle_decline", Payload: `{}`, NextTryAt: time.Now().Unix(),
		}},
	}); err != nil || !changed {
		t.Fatalf("create removed-tenant action = %t, %v", changed, err)
	}
	rule := rules.Record{
		ID: "retained-rule", ChatID: groupID, Collection: "challenge", Enabled: true,
		Definition: []byte(`{"message":"retained"}`),
	}
	if _, changed, err := database.NewRuleStore(runtime.database).ReplaceRules(
		ctx, groupID, rule.Collection, nil, []rules.Record{rule},
	); err != nil || !changed {
		t.Fatalf("create removed-tenant rule = %t, %v", changed, err)
	}
	if err := state.SaveFailures("database", func() []verification.FailureRecord {
		return []verification.FailureRecord{{GroupID: groupID, UserID: 4103, Count: 2, Last: time.Now().Unix()}}
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.NewWarningStore(runtime.database).SaveWarnings(func() []moderate.WarningRecord {
		return []moderate.WarningRecord{{GroupID: groupID, UserID: 4104, Count: 3}}
	}); err != nil {
		t.Fatal(err)
	}
}

func removedTenantRows(t *testing.T, db *database.Database, groupID int64) tenantRowCounts {
	t.Helper()
	ctx := context.Background()
	var counts tenantRowCounts
	queries := []struct {
		value *int
		query string
	}{
		{&counts.chats, "SELECT COUNT(*) FROM chat WHERE id=$1"},
		{&counts.challenges, "SELECT COUNT(*) FROM challenge WHERE chat_id=$1"},
		{&counts.actions, "SELECT COUNT(*) FROM pending_action a JOIN challenge c ON c.id=a.challenge_id WHERE c.chat_id=$1"},
		{&counts.rules, "SELECT COUNT(*) FROM rule WHERE chat_id=$1"},
		{&counts.failures, "SELECT COUNT(*) FROM verification_failure WHERE chat_id=$1"},
		{&counts.warnings, "SELECT COUNT(*) FROM warning_counter WHERE chat_id=$1"},
	}
	for _, item := range queries {
		if err := db.QueryRow(ctx, item.query, groupID).Scan(item.value); err != nil {
			t.Fatal(err)
		}
	}
	want := tenantRowCounts{chats: 1, challenges: 2, actions: 1, rules: 1, failures: 1, warnings: 1}
	if counts != want {
		t.Fatalf("removed-tenant fixture rows = %+v, want %+v", counts, want)
	}
	return counts
}

func assertRemovedTenantHidden(t *testing.T, runtime *services, remainingGroup, removedGroup int64) {
	t.Helper()
	groups := runtime.settings.ChatIDs()
	if len(groups) != 1 || groups[0] != remainingGroup {
		t.Fatalf("active groups after config removal = %v, want [%d]", groups, remainingGroup)
	}
	if _, ok := runtime.settings.Settings(removedGroup); ok {
		t.Fatal("removed group remained readable through active settings")
	}
	remaining, ok := runtime.settings.Settings(remainingGroup)
	if !ok || !remaining.Enabled().Value || remaining.Enabled().Source == settings.SourceChatOverride {
		t.Fatalf("remaining group inherited removed settings: %+v", remaining.Enabled())
	}
	queue, err := runtime.verification.ConsoleQueue(context.Background(), remainingGroup)
	if err != nil || len(queue) != 0 {
		t.Fatalf("remaining group read removed queue: entries=%+v err=%v", queue, err)
	}
	records, err := database.NewRuleStore(runtime.database).ListRules(context.Background(), remainingGroup, "")
	if err != nil || len(records) != 0 {
		t.Fatalf("remaining group read removed rules: records=%+v err=%v", records, err)
	}
}

func assertRemovedTenantRestored(t *testing.T, runtime *services, removedGroup int64) {
	t.Helper()
	restored, ok := runtime.settings.Settings(removedGroup)
	if !ok || restored.Enabled().Value || restored.Enabled().Source != settings.SourceChatOverride {
		t.Fatalf("re-added group settings = %+v, want retained disabled override", restored.Enabled())
	}
	records, err := database.NewRuleStore(runtime.database).ListRules(context.Background(), removedGroup, "")
	if err != nil || len(records) != 1 || records[0].ID != "retained-rule" {
		t.Fatalf("re-added group rules = %+v, err=%v", records, err)
	}
	queue, err := runtime.verification.ConsoleQueue(context.Background(), removedGroup)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("re-added group's queue has %d pending rows", len(queue))
}
