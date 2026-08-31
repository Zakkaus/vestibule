package telegram

import (
	"context"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
)

func TestStartupStateAllowsMissingConfigAndNoGroups(t *testing.T) {
	configPath := t.TempDir() + "/config.json"
	cfg, settings, err := loadRuntimeState(configPath, t.TempDir())
	if err != nil {
		t.Fatalf("startup state: %v", err)
	}
	if len(cfg.Groups) != 0 || len(settings.GroupIDs()) != 0 {
		t.Fatalf("startup groups = config %v, settings %v; want none", cfg.GroupIDs, settings.GroupIDs())
	}
	if status := settings.Persistence(); !status.Durable || !status.Writable {
		t.Fatalf("settings persistence = %+v, want durable and writable", status)
	}
	if nonce, created, err := settings.EnsureOwnerClaim(time.Now(), ownerClaimLifetime); err != nil || !created || nonce == "" {
		t.Fatalf("owner claim on zero-group startup = nonce %q, created %t, error %v", nonce, created, err)
	}
}

func TestStartupConfigRemainsRegistrationBaseline(t *testing.T) {
	const groupID int64 = -1009000000601
	configPath := t.TempDir() + "/missing-config.json"
	stateDirectory := t.TempDir()
	_, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	registration := settings.Registrations()
	registration.RegisteredGroups = []store.RegisteredGroup{{ID: groupID, RegisteredBy: 42}}
	if _, err := settings.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	cfg, reloaded, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.IsGroup(groupID) {
		t.Fatal("runtime group was not restored into live settings")
	}
	if cfg.IsGroup(groupID) {
		t.Fatal("runtime group leaked into the immutable config baseline")
	}
}

func TestOwnerClaimRefreshesCommandMenus(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(
		context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil,
	)
	application := NewUpdates(cfg, settings, nil, HandlerSet{})
	service.onOwnerClaimed = func(ctx context.Context) {
		application.SetupCommands(ctx, bot)
	}
	service.now = func() time.Time { return now }
	if err := service.EnsureOwnerClaim(); err != nil {
		t.Fatal(err)
	}
	claim := settings.Registrations().OwnerClaimNonce

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: testOwner, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: testOwner, LanguageCode: "en"},
		Text: "/start owner_" + claim,
	}})

	caller.mu.Lock()
	scopeIDs := append([]int64(nil), caller.commandScopeIDs...)
	caller.mu.Unlock()
	if len(scopeIDs) != 3 {
		t.Fatalf("owner command-menu refresh scopes = %v, want three owner chat scopes", scopeIDs)
	}
	for _, chatID := range scopeIDs {
		if chatID != testOwner {
			t.Fatalf("owner command-menu refresh targeted chat %d, want %d", chatID, testOwner)
		}
	}
}

func TestOwnerClaimIsFirstUserSingleUse(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember), events: make(chan string, 16)}
	bot := newRegistrationBot(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newRegistrationService(ctx, bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	logs := newSynchronizedLog()
	oldLog := log.Writer()
	log.SetOutput(logs)
	defer log.SetOutput(oldLog)
	if err := service.EnsureOwnerClaim(); err != nil {
		t.Fatal(err)
	}
	claim := settings.Registrations().OwnerClaimNonce
	message := func(userID int64) telego.Update {
		return telego.Update{Message: &telego.Message{
			Chat: telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
			From: &telego.User{ID: userID, LanguageCode: "en"},
			Text: "/start owner_" + claim,
		}}
	}
	runRegistrationUpdate(t, bot, service, message(testOwner))
	waitForRegistrationMethod(t, caller, "sendMessage")
	runRegistrationUpdate(t, bot, service, message(testOwner+1))
	waitForRegistrationMethod(t, caller, "sendMessage")
	waitForLog(t, logs, "owner claim refused: user=43")
	state := settings.Registrations()
	if state.OwnerID != testOwner || state.OwnerClaimNonce != "" || state.OwnerClaimExpiresAt != 0 {
		t.Fatalf("owner state = %+v", state)
	}
	if !strings.Contains(logs.String(), "owner claim refused: user=43") {
		t.Fatalf("refused replay was not logged: %s", logs.String())
	}
}

func TestEnrollmentNoncePromotionReplayAndExpiry(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember), events: make(chan string, 32)}
	bot := newRegistrationBot(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newRegistrationService(ctx, bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	const (
		actor  = int64(77)
		groupA = int64(-1001)
		groupB = int64(-1002)
		groupC = int64(-1003)
	)
	caller.members[[2]int64{groupA, actor}] = adminMember(actor)
	caller.members[[2]int64{groupA, testBotID}] = plainMember(testBotID)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupA, Type: telego.ChatTypeSupergroup, Title: "A"},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})
	waitForRegistrationMethod(t, caller, "sendMessage")
	if settings.IsGroup(groupA) || len(settings.Registrations().PendingRegistrations) != 1 {
		t.Fatalf("group should be pending before promotion: %+v", settings.Registrations())
	}
	caller.members[[2]int64{groupA, testBotID}] = adminMember(testBotID)
	runRegistrationUpdate(t, bot, service, telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupA, Type: telego.ChatTypeSupergroup, Title: "A"},
		From:          telego.User{ID: actor, LanguageCode: "en"},
		OldChatMember: &telego.ChatMemberMember{Status: telego.MemberStatusMember, User: telego.User{ID: testBotID}},
		NewChatMember: adminMember(testBotID),
	}})
	waitForRegistrationMethod(t, caller, "sendMessage")
	if !settings.IsGroup(groupA) || len(settings.Registrations().PendingRegistrations) != 0 {
		t.Fatalf("promoted group was not registered: %+v", settings.Registrations())
	}

	caller.members[[2]int64{groupB, actor}] = adminMember(actor)
	caller.members[[2]int64{groupB, testBotID}] = plainMember(testBotID)
	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupB, Type: telego.ChatTypeSupergroup, Title: "B"},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})
	waitForRegistrationMethod(t, caller, "leaveChat")
	if settings.IsGroup(groupB) || len(caller.left) != 1 || caller.left[0] != groupB {
		t.Fatalf("nonce replay result: groups=%v leaves=%v", settings.GroupIDs(), caller.left)
	}

	expired, err := settings.IssueEnrollmentNonce(testOwner, now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.Add(2 * time.Second) }
	caller.members[[2]int64{groupC, actor}] = adminMember(actor)
	caller.members[[2]int64{groupC, testBotID}] = plainMember(testBotID)
	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupC, Type: telego.ChatTypeSupergroup, Title: "C"},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + expired.Nonce,
	}})
	waitForRegistrationMethod(t, caller, "leaveChat")
	if settings.IsGroup(groupC) || len(caller.left) != 2 || caller.left[1] != groupC {
		t.Fatalf("expired nonce result: groups=%v leaves=%v", settings.GroupIDs(), caller.left)
	}
}

func TestOwnerPromotionRegistersFirstControlGroup(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	const groupID = int64(-1901)
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{groupID, testOwner}: adminMember(testOwner),
			{groupID, testBotID}: adminMember(testBotID),
		},
		events: make(chan string, 16),
	}
	bot := newRegistrationBot(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newRegistrationService(ctx, bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }
	runRegistrationUpdate(t, bot, service, telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Owner Group"},
		From:          telego.User{ID: testOwner, LanguageCode: "en"},
		OldChatMember: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: telego.User{ID: testBotID}},
		NewChatMember: adminMember(testBotID),
	}})
	waitForRegistrationMethod(t, caller, "sendMessage")
	state := settings.Registrations()
	if !settings.IsGroup(groupID) || state.ControlGroupID != groupID || len(state.RegisteredGroups) != 1 {
		t.Fatalf("owner registration = %+v, groups=%v", state, settings.GroupIDs())
	}
	if len(caller.left) != 0 || len(caller.sent) != 1 {
		t.Fatalf("owner registration Telegram calls: sent=%+v left=%v", caller.sent, caller.left)
	}
	want := i18n.Messages.Bot.Registration.GroupRegistered.Render(i18n.LangEN, "Owner Group")
	if caller.sent[0].Text != want {
		t.Fatalf("owner registration message = %q, want catalogue text %q", caller.sent[0].Text, want)
	}
	if strings.Contains(caller.sent[0].Text, "?start=") {
		t.Fatalf("owner registration returned an unroutable start payload: %q", caller.sent[0].Text)
	}
}

func TestRegistrationCompletedMessageLocales(t *testing.T) {
	const (
		groupID = int64(-1902)
		title   = "Runtime Group"
	)
	for _, test := range []struct {
		name string
		code string
		lang i18n.Lang
	}{
		{name: "zh", code: "zh-CN", lang: i18n.LangZH},
		{name: "zh-Hant", code: "zh-TW", lang: i18n.LangZHHant},
		{name: "en", code: "en", lang: i18n.LangEN},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, settings := registrationFixture(t)
			caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
			bot := newRegistrationBot(t, caller)
			service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
			service.registrationCompleted(context.Background(),
				telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
				telego.User{ID: testOwner, LanguageCode: test.code})
			if len(caller.sent) != 1 {
				t.Fatalf("registration messages = %d, want 1", len(caller.sent))
			}
			want := i18n.Messages.Bot.Registration.GroupRegistered.Render(test.lang, title)
			if caller.sent[0].Text != want {
				t.Errorf("registration message = %q, want catalogue text %q", caller.sent[0].Text, want)
			}
			if strings.Contains(caller.sent[0].Text, "?start=") {
				t.Errorf("registration message returned an unroutable start payload: %q", caller.sent[0].Text)
			}
		})
	}
}

func TestNonOwnerPromotionAttemptLeaves(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	const (
		actor   = int64(77)
		groupID = int64(-2001)
	)
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, actor}: adminMember(actor),
	}}
	caller.events = make(chan string, 16)
	bot := newRegistrationBot(t, caller)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newRegistrationService(ctx, bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }
	runRegistrationUpdate(t, bot, service, telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Unauthorized"},
		From:          telego.User{ID: actor, LanguageCode: "en"},
		OldChatMember: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: telego.User{ID: testBotID}},
		NewChatMember: adminMember(testBotID),
	}})
	waitForRegistrationMethod(t, caller, "leaveChat")
	if settings.IsGroup(groupID) || len(caller.left) != 1 || caller.left[0] != groupID {
		t.Fatalf("non-owner attempt: groups=%v leaves=%v", settings.GroupIDs(), caller.left)
	}
}
