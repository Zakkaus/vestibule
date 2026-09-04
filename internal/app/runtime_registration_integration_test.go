package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func runRuntimeHandler(t *testing.T, bot *telego.Bot, register func(*th.BotHandler), update telego.Update) {
	t.Helper()
	handler, err := th.NewBotHandler(bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	register(handler)
	if err := handler.BaseGroup().HandleUpdate(context.Background(), bot, update); err != nil {
		t.Fatal(err)
	}
}

type runtimeRegistrationFixture struct {
	bot          *telego.Bot
	caller       *dispatchCaller
	cfg          *settings.Config
	settings     *settings.Store
	verification *verification.Service
	updates      *telegram.Updates
	registration *telegram.Registration
}

func newRuntimeRegistrationFixture(
	t *testing.T,
	groupID, ownerID, botID int64,
) *runtimeRegistrationFixture {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"lang":"zh-Hant","notify_ttl_seconds":-1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, settings, err := loadRuntimeState(configPath, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	nonce, _, err := settings.EnsureOwnerClaim(now, cfg.OwnerClaimLifetime())
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.ClaimOwner(ownerID, nonce, now); err != nil {
		t.Fatal(err)
	}
	caller := &dispatchCaller{
		botID:        botID,
		members:      make(map[[2]int64]telego.ChatMember),
		commandMenus: make(chan struct{}, 1),
	}
	caller.setMember(groupID, ownerID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator,
		User:   telego.User{ID: ownerID},
	})
	caller.setMember(groupID, botID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator,
		User:   telego.User{ID: botID},
	})
	bot := testBot(t, caller)
	connector := telegram.NewConnector(bot)
	identity := verification.Identity{ID: botID, Username: "verify_test_bot"}
	verification := newTestVerifier(t, settings, connector, cfg, identity, "")
	verificationGateway := telegram.NewVerificationGateway(connector)
	t.Cleanup(verification.Shutdown)
	moderation, err := moderate.New(settings, connector, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	lookups := lookup.New(settings, connector, cfg, "")
	administration := panel.New(
		settings, connector, cfg, &i18n.Messages,
		verification, moderation, lookups, "test", time.Now(),
	)
	modules, err := newRuntimeModules(cfg, bot, t.TempDir(), administration, moderation, lookups, false)
	if err != nil {
		t.Fatal(err)
	}
	administration.SetCommandModules(modules.commands)
	updates := telegram.NewUpdates(
		cfg, settings, connector,
		telegramHandlers(verification, verificationGateway, administration, moderation, modules.commands, nil),
	)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &runtimeRegistrationFixture{
		bot:          bot,
		caller:       caller,
		cfg:          cfg,
		settings:     settings,
		verification: verification,
		updates:      updates,
		registration: newRegistration(ctx, bot, cfg, settings, identity, moderation, verification, updates),
	}
}

func TestRuntimeRegistrationActivatesServicesWithoutRebuiltConfig(t *testing.T) {
	const (
		groupID = int64(-1009000000999)
		userID  = int64(7001)
		ownerID = int64(7002)
		botID   = int64(7003)
	)
	fixture := newRuntimeRegistrationFixture(t, groupID, ownerID, botID)
	registrationUpdate := telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Runtime Group"},
		From:          telego.User{ID: ownerID, LanguageCode: "en"},
		OldChatMember: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: telego.User{ID: botID}},
		NewChatMember: &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator, User: telego.User{ID: botID}},
	}}
	runRuntimeHandler(t, fixture.bot, fixture.registration.Register, registrationUpdate)
	if !fixture.settings.IsGroup(groupID) {
		t.Fatalf("registration did not publish the runtime group: state=%+v calls=%+v",
			fixture.settings.Registrations(), fixture.caller.snapshotCalls())
	}
	if fixture.cfg.IsGroup(groupID) {
		t.Fatal("test rebuilt or mutated the startup config")
	}
	runtimeSettings, _ := fixture.settings.Settings(groupID)
	runtimeOverrides := runtimeSettings.Overrides()
	runtimeLanguage := "zh-Hant"
	runtimeOverrides.Lang = &runtimeLanguage
	if _, err := fixture.settings.Update(groupID, runtimeSettings.Revision(), runtimeOverrides); err != nil {
		t.Fatal(err)
	}
	fixture.updates.SetupCommands(context.Background(), fixture.bot)

	beforeJoin := runtimeMethodCallsForChat(t, fixture.caller, "sendMessage", groupID)
	beforePrivate := runtimeMethodCallsForChat(t, fixture.caller, "sendMessage", userID)
	runRuntimeHandler(t, fixture.bot, fixture.updates.Register, telego.Update{ChatJoinRequest: &telego.ChatJoinRequest{
		Chat:       telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Runtime Group"},
		From:       telego.User{ID: userID, FirstName: "Applicant", LanguageCode: "en"},
		UserChatID: userID,
	}})
	joinHandled := runtimeMethodCallsForChat(t, fixture.caller, "sendMessage", userID) > beforePrivate &&
		runtimeMethodCallsForChat(t, fixture.caller, "sendMessage", groupID) > beforeJoin

	runRuntimeHandler(t, fixture.bot, fixture.updates.Register, telego.Update{Message: &telego.Message{
		MessageID: 2,
		Chat:      telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Runtime Group"},
		From:      &telego.User{ID: ownerID, LanguageCode: "en"},
		Text:      "/stop",
	}})
	if !waitForRuntimeCommandScope(t, fixture.caller, groupID) {
		t.Error("runtime group did not receive its locale-specific command scope")
	}
	if !joinHandled {
		t.Error("runtime group join request did not create the default group and private verification challenges")
	}
	if fixture.verification.IsEnabled(groupID) {
		t.Error("runtime group /stop command did not change verification state")
	}
}

func runtimeMethodCallsForChat(t *testing.T, caller *dispatchCaller, method string, chatID int64) int {
	t.Helper()
	count := 0
	for _, call := range caller.snapshotCalls() {
		if call.method != method {
			continue
		}
		var params struct {
			ChatID int64 `json:"chat_id"`
		}
		if err := json.Unmarshal(call.body, &params); err != nil {
			t.Fatal(err)
		}
		if params.ChatID == chatID {
			count++
		}
	}
	return count
}

func waitForRuntimeCommandScope(t *testing.T, caller *dispatchCaller, groupID int64) bool {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if runtimeHasCommandScope(t, caller, groupID) {
			return true
		}
		select {
		case <-caller.commandMenus:
		case <-deadline.C:
			return false
		}
	}
}

func runtimeHasCommandScope(t *testing.T, caller *dispatchCaller, groupID int64) bool {
	t.Helper()
	for _, call := range caller.snapshotCalls() {
		if call.method != "setMyCommands" {
			continue
		}
		var params struct {
			Scope struct {
				ChatID int64 `json:"chat_id"`
			} `json:"scope"`
		}
		if err := json.Unmarshal(call.body, &params); err != nil {
			t.Fatal(err)
		}
		if params.Scope.ChatID == groupID {
			return true
		}
	}
	return false
}

func TestConfiguredGroupsKeepTenantStateIsolated(t *testing.T) {
	const (
		groupA  = int64(-1009000000811)
		groupB  = int64(-1009000000812)
		adminID = int64(4201)
		botID   = int64(4202)
	)
	ctx := context.Background()
	stateDirectory := t.TempDir()
	configPath := filepath.Join(stateDirectory, "config.json")
	if err := os.WriteFile(configPath,
		[]byte(`{"groups":[{"id":-1009000000811},{"id":-1009000000812}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(ctx, database.Config{StateDirectory: stateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg, state, err := loadRuntimeState(configPath, stateDirectory, database.NewSettingsStore(db))
	if err != nil {
		t.Fatal(err)
	}
	caller := &dispatchCaller{botID: botID, members: make(map[[2]int64]telego.ChatMember)}
	caller.setMember(groupA, adminID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator, User: telego.User{ID: adminID},
	})
	caller.setMember(groupB, adminID, &telego.ChatMemberMember{
		Status: telego.MemberStatusMember, User: telego.User{ID: adminID},
	})
	connector := telegram.NewConnector(testBot(t, caller))
	service, err := verification.New(
		state, telegram.NewVerificationGateway(connector), database.NewVerificationStore(db),
		cfg, &i18n.Messages, nil, verification.Identity{ID: botID, Username: "tenant_test_bot"},
		verificationStateNamespace(stateDirectory), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)

	requireTenantSettingsIsolation(t, state, groupA, groupB)
	requireTenantQueueIsolation(t, service, db, groupA, groupB)
	requireTenantAuthorizationIsolation(t, connector, caller, groupA, groupB, adminID)
	requireTenantRuleIsolation(t, db, groupA, groupB)
}

func requireTenantSettingsIsolation(t *testing.T, state *settings.Store, groupA, groupB int64) {
	t.Helper()
	first, firstOK := state.Settings(groupA)
	second, secondOK := state.Settings(groupB)
	if !firstOK || !secondOK {
		t.Fatalf("tenant settings missing: first=%t second=%t", firstOK, secondOK)
	}
	secondRevision, secondEnabled := second.Revision(), second.Enabled()
	secondOverrides := second.Overrides()
	disabled := false
	next := first.Overrides()
	next.Enabled = &disabled
	if _, err := state.Update(groupA, first.Revision(), next); err != nil {
		t.Fatal(err)
	}
	first, _ = state.Settings(groupA)
	second, _ = state.Settings(groupB)
	if first.Enabled().Value || first.Enabled().Source != settings.SourceChatOverride {
		t.Fatalf("first tenant setting was not updated: %+v", first.Enabled())
	}
	if second.Revision() != secondRevision || second.Enabled() != secondEnabled ||
		!reflect.DeepEqual(second.Overrides(), secondOverrides) {
		t.Fatalf("first tenant setting changed second tenant: revision=%d enabled=%+v overrides=%+v",
			second.Revision(), second.Enabled(), second.Overrides())
	}
}

func requireTenantQueueIsolation(
	t *testing.T,
	service *verification.Service,
	db *database.Database,
	groupA, groupB int64,
) {
	t.Helper()
	state := database.NewVerificationStore(db)
	first := verification.PendingRecord{
		GroupID: groupA, UserID: 4301, Nonce: "tenant-a", Mode: settings.ModeKernel,
		Deadline: time.Now().Add(time.Hour).Unix(),
	}
	second := verification.PendingRecord{
		GroupID: groupB, UserID: 4302, Nonce: "tenant-b", Mode: settings.ModeKernel,
		Deadline: time.Now().Add(time.Hour).Unix(),
	}
	for _, record := range []verification.PendingRecord{first, second} {
		inserted, err := state.InsertPending("database", record)
		if err != nil {
			t.Fatal(err)
		}
		if !inserted {
			t.Fatalf("tenant queue row was not inserted: %+v", record)
		}
	}
	requireTenantQueue(t, service, groupA, 1)
	requireTenantQueue(t, service, groupB, 1)
	changed, err := state.TransitionChallenge("database", verification.ChallengeTransition{
		Expected: first.Ref(), Record: first, From: verification.ChallengePending,
		To: verification.ChallengeApproved, SettledAt: time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first tenant queue row was not settled")
	}
	requireTenantQueue(t, service, groupA, 0)
	requireTenantQueue(t, service, groupB, 1)
}

func requireTenantQueue(t *testing.T, service *verification.Service, groupID int64, want int) {
	t.Helper()
	queue, err := service.ConsoleQueue(context.Background(), groupID)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != want {
		t.Fatalf("tenant %d queue entries = %+v, want %d", groupID, queue, want)
	}
	for _, entry := range queue {
		if entry.GroupID != groupID {
			t.Fatalf("tenant %d queue contains group %d", groupID, entry.GroupID)
		}
	}
}

func requireTenantAuthorizationIsolation(
	t *testing.T,
	connector *telegram.Connector,
	caller *dispatchCaller,
	groupA, groupB, adminID int64,
) {
	t.Helper()
	manager, err := auth.New(auth.Config{BotToken: "1:" + strings.Repeat("a", 35), AdminChecker: connector})
	if err != nil {
		t.Fatal(err)
	}
	session := auth.Session{Principal: auth.Principal{TelegramID: adminID, Role: auth.RoleManager}}
	if err = manager.AuthorizeChat(context.Background(), session, groupA, auth.ReadAccess); err != nil {
		t.Fatalf("first tenant read authorization: %v", err)
	}
	if err = manager.AuthorizeChat(context.Background(), session, groupB, auth.ReadAccess); !errors.Is(err, auth.ErrAccessDenied) {
		t.Fatalf("second tenant inherited first authorization: %v", err)
	}
	caller.setMember(groupA, adminID, &telego.ChatMemberMember{
		Status: telego.MemberStatusMember, User: telego.User{ID: adminID},
	})
	caller.setMember(groupB, adminID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator, User: telego.User{ID: adminID},
	})
	if err = manager.AuthorizeChat(context.Background(), session, groupA, auth.WriteAccess); !errors.Is(err, auth.ErrAccessDenied) {
		t.Fatalf("revoked first tenant write authorization: %v", err)
	}
	if err = manager.AuthorizeChat(context.Background(), session, groupB, auth.WriteAccess); err != nil {
		t.Fatalf("second tenant write authorization inherited first revocation: %v", err)
	}
}

func requireTenantRuleIsolation(t *testing.T, db *database.Database, groupA, groupB int64) {
	t.Helper()
	ctx := context.Background()
	store := database.NewRuleStore(db)
	first := rules.Record{
		ID: "tenant-a-rule", ChatID: groupA, Collection: "tenant_a", Enabled: true,
		Definition: json.RawMessage(`{"message":"first"}`),
	}
	second := rules.Record{
		ID: "tenant-b-rule", ChatID: groupB, Collection: "tenant_b", Enabled: true,
		Definition: json.RawMessage(`{"message":"second"}`),
	}
	if _, changed, err := store.ReplaceRules(ctx, groupA, first.Collection, nil, []rules.Record{first}); err != nil || !changed {
		t.Fatalf("create first tenant rule = %t, %v", changed, err)
	}
	if _, changed, err := store.ReplaceRules(ctx, groupB, second.Collection, nil, []rules.Record{second}); err != nil || !changed {
		t.Fatalf("create second tenant rule = %t, %v", changed, err)
	}
	updated := first
	updated.Enabled = false
	if _, changed, err := store.UpdateRule(ctx, groupA, first, updated); err != nil || !changed {
		t.Fatalf("update first tenant rule = %t, %v", changed, err)
	}
	firstRules, firstErr := store.ListRules(ctx, groupA, "")
	secondRules, secondErr := store.ListRules(ctx, groupB, "")
	if firstErr != nil || secondErr != nil || len(firstRules) != 1 || len(secondRules) != 1 ||
		firstRules[0].ID != first.ID || firstRules[0].Enabled ||
		secondRules[0].ID != second.ID || !secondRules[0].Enabled {
		t.Fatalf("tenant rules after first update: first=%+v/%v second=%+v/%v",
			firstRules, firstErr, secondRules, secondErr)
	}
}
