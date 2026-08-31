package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/store"
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
	cfg          *config.Config
	settings     *store.Settings
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
	verification := newTestVerifier(settings, connector, cfg, identity, "")
	verificationGateway := telegram.NewVerificationGateway(connector)
	t.Cleanup(verification.Shutdown)
	moderation := moderate.New(settings, connector, cfg, nil)
	lookups := lookup.New(settings, connector, cfg, "")
	administration := panel.New(
		settings, connector, cfg, &i18n.Messages,
		verification, moderation, lookups, "test", time.Now(),
	)
	updates := telegram.NewUpdates(cfg, settings, connector, telegramHandlers(verification, verificationGateway, administration, moderation, lookups))
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
