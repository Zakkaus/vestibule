package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func TestUnknownGroupGraceExpiry(t *testing.T) {
	newFixture := func(t *testing.T) (*registrationService, *registrationCaller) {
		t.Helper()
		cfg, settings := registrationFixture(t)
		caller := &registrationCaller{
			members: make(map[[2]int64]telego.ChatMember),
			events:  make(chan string, 16),
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		service := newRegistrationService(ctx, newRegistrationBot(t, caller), settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
		return service, caller
	}

	t.Run("expired unknown group leaves exactly once", func(t *testing.T) {
		const groupID = int64(-3001)
		service, caller := newFixture(t)
		service.scheduleUnknownLeave(groupID, "Expired", time.Now().Add(-time.Second))
		waitForRegistrationMethod(t, caller, "leaveChat")
		time.Sleep(20 * time.Millisecond)
		left := caller.leftChats()
		if len(left) != 1 || left[0] != groupID {
			t.Fatalf("expired unknown group leaves = %v, want [%d]", left, groupID)
		}
	})

	t.Run("registered group is retained", func(t *testing.T) {
		const groupID = int64(-3002)
		service, caller := newFixture(t)
		state := service.settings.Registrations()
		state.RegisteredGroups = []store.RegisteredGroup{{ID: groupID, RegisteredBy: testOwner, Title: "Registered"}}
		if _, err := service.settings.CommitRegistrations(state.Revision, state); err != nil {
			t.Fatal(err)
		}
		service.scheduleUnknownLeave(groupID, "Registered", time.Now().Add(-time.Second))
		select {
		case method := <-caller.events:
			t.Fatalf("registered group triggered Telegram method %q", method)
		case <-time.After(20 * time.Millisecond):
		}
		if left := caller.leftChats(); len(left) != 0 {
			t.Fatalf("registered group leaves = %v, want none", left)
		}
	})

	t.Run("later deadline does not double fire", func(t *testing.T) {
		const groupID = int64(-3003)
		service, caller := newFixture(t)
		now := time.Now()
		service.scheduleUnknownLeave(groupID, "Duplicate", now.Add(50*time.Millisecond))
		service.scheduleUnknownLeave(groupID, "Duplicate", now.Add(100*time.Millisecond))
		waitForRegistrationMethod(t, caller, "leaveChat")
		time.Sleep(20 * time.Millisecond)
		left := caller.leftChats()
		if len(left) != 1 || left[0] != groupID {
			t.Fatalf("duplicate deadline leaves = %v, want [%d]", left, groupID)
		}
	})
}

func TestRegistrationAndUnknownLeaveAreSerialized(t *testing.T) {
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	const (
		actor   = int64(77)
		groupID = int64(-4001)
	)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	releaseLeave := make(chan struct{})
	releaseActorLookup := make(chan struct{})
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{groupID, actor}:     adminMember(actor),
			{groupID, testBotID}: adminMember(testBotID),
		},
		memberLookups: make(chan [2]int64, 8),
		lookupBlocks: map[[2]int64]<-chan struct{}{
			{groupID, actor}: releaseActorLookup,
		},
		leaveStarted: make(chan int64, 1),
		releaseLeave: releaseLeave,
		events:       make(chan string, 8),
	}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	leaveDone := make(chan struct{})
	go func() {
		service.leaveUnknown(context.Background(), telego.Chat{
			ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Race",
		}, actor, "test race")
		close(leaveDone)
	}()
	if leftGroup := <-caller.leaveStarted; leftGroup != groupID {
		t.Fatalf("leave started for %d, want %d", leftGroup, groupID)
	}

	updates := make(chan telego.Update, 1)
	handler, err := th.NewBotHandler(bot, updates)
	if err != nil {
		t.Fatal(err)
	}
	processed := make(chan error, 1)
	handler.Use(func(ctx *th.Context, update telego.Update) error {
		err := ctx.Next(update)
		processed <- err
		return err
	})
	service.Register(handler)
	handlerDone := make(chan error, 1)
	go func() { handlerDone <- handler.Start() }()
	updates <- telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Race"},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}}
	if lookup := <-caller.memberLookups; lookup != [2]int64{groupID, actor} {
		t.Fatalf("first membership lookup = %v, want actor lookup", lookup)
	}
	close(releaseLeave)
	<-leaveDone
	close(releaseActorLookup)
	if err := <-processed; err != nil {
		t.Fatal(err)
	}
	close(updates)
	if err := <-handlerDone; err != nil {
		t.Fatal(err)
	}
	if settings.IsGroup(groupID) {
		t.Fatalf("group %d was registered from membership observed before LeaveChat completed", groupID)
	}
}

func TestUnknownGroupLeaveSurvivesRestart(t *testing.T) {
	const (
		actor   = int64(78)
		groupID = int64(-4002)
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	bindTestOwner(t, settings, now)
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, actor}:     adminMember(actor),
		{groupID, testBotID}: plainMember(testBotID),
	}}
	firstRoot, cancelFirst := context.WithCancel(context.Background())
	first := newRegistrationService(firstRoot, newRegistrationBot(t, caller), settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	t.Cleanup(func() {
		cancelFirst()
		first.Wait()
	})
	first.now = func() time.Time { return now }
	runRegistrationUpdate(t, first.bot, first, telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Restart"},
		From:          telego.User{ID: actor, LanguageCode: "en"},
		OldChatMember: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: telego.User{ID: testBotID}},
		NewChatMember: plainMember(testBotID),
	}})
	cancelFirst()
	first.Wait()

	settingsPath := filepath.Join(stateDirectory, "settings.json")
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	leaves, ok := state["unknown_group_leaves"].([]any)
	if !ok || len(leaves) != 1 {
		t.Fatalf("persisted unknown-group leaves = %#v, want one cleanup record", state["unknown_group_leaves"])
	}
	record, ok := leaves[0].(map[string]any)
	if !ok {
		t.Fatalf("unknown-group leave record = %#v", leaves[0])
	}
	record["expires_at"] = time.Now().Add(-time.Second).Unix()
	raw, err = json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	reloadedCfg, reloadedSettings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	restartedCaller := &registrationCaller{
		members: make(map[[2]int64]telego.ChatMember),
		events:  make(chan string, 4),
	}
	secondRoot, cancelSecond := context.WithCancel(context.Background())
	second := newRegistrationService(secondRoot, newRegistrationBot(t, restartedCaller), reloadedSettings, reloadedCfg, "verify_test_bot", testBotID, nil, nil, nil)
	t.Cleanup(func() {
		cancelSecond()
		second.Wait()
	})
	waitForRegistrationMethod(t, restartedCaller, "leaveChat")
	if left := restartedCaller.leftChats(); len(left) != 1 || left[0] != groupID {
		t.Fatalf("restart leaves = %v, want [%d]", left, groupID)
	}
	cancelSecond()
	second.Wait()
}

func TestEffectiveGroupDemotionRunsOneSetupReport(t *testing.T) {
	cfg, settings := registrationFixture(t)
	const groupID = int64(-4003)
	state := settings.Registrations()
	state.RegisteredGroups = []store.RegisteredGroup{{ID: groupID, RegisteredBy: testOwner}}
	if _, err := settings.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}
	reports := make(chan int64, 2)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, func(_ context.Context, reportedGroupID int64) { reports <- reportedGroupID }, nil)
	update := telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Demoted"},
		From:          telego.User{ID: 79, LanguageCode: "en"},
		OldChatMember: adminMember(testBotID),
		NewChatMember: plainMember(testBotID),
	}}
	runRegistrationUpdate(t, bot, service, update)
	select {
	case reportedGroupID := <-reports:
		if reportedGroupID != groupID {
			t.Fatalf("setup report group = %d, want %d", reportedGroupID, groupID)
		}
	default:
		t.Fatal("effective-group demotion did not run the setup report")
	}
	runRegistrationUpdate(t, bot, service, update)
	select {
	case reportedGroupID := <-reports:
		t.Fatalf("flapping group produced an unthrottled second report for %d", reportedGroupID)
	default:
	}
	if !settings.IsGroup(groupID) {
		t.Fatal("membership loss automatically erased an owner registration")
	}
}

type removalEvent struct {
	groupID int64
	durable bool
}

type unregisterFixture struct {
	groupID        int64
	stateDirectory string
	settings       *store.Settings
	caller         *registrationCaller
	bot            *telego.Bot
	service        *registrationService
	removed        chan removalEvent
	releaseLeave   chan struct{}
}

func newUnregisterFixture(t *testing.T) *unregisterFixture {
	t.Helper()
	const groupID = int64(-4004)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	bindTestOwner(t, settings, time.Unix(2_000_000_000, 0))
	state := settings.Registrations()
	state.RegisteredGroups = []store.RegisteredGroup{{ID: groupID, RegisteredBy: testOwner, Title: "Remove"}}
	state.ControlGroupID = groupID
	if _, err := settings.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}
	group, ok := settings.Group(groupID)
	if !ok {
		t.Fatal("registered group is not effective")
	}
	overrides := group.Overrides()
	disabled := false
	overrides.Enabled = &disabled
	if _, err := settings.CommitGroup(groupID, group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	releaseLeave := make(chan struct{}, 1)
	caller := &registrationCaller{
		members:      make(map[[2]int64]telego.ChatMember),
		events:       make(chan string, 8),
		leaveStarted: make(chan int64, 1),
		releaseLeave: releaseLeave,
	}
	bot := newRegistrationBot(t, caller)
	removed := make(chan removalEvent, 1)
	onRemoved := func(removedGroupID int64) {
		removed <- removalEvent{groupID: removedGroupID, durable: !settings.IsGroup(removedGroupID)}
	}
	service := newRegistrationService(
		context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, onRemoved,
	)
	return &unregisterFixture{
		groupID: groupID, stateDirectory: stateDirectory, settings: settings,
		caller: caller, bot: bot, service: service, removed: removed, releaseLeave: releaseLeave,
	}
}

func (f *unregisterFixture) command(userID int64) telego.Update {
	return telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: userID, LanguageCode: "en"},
		Text: fmt.Sprintf("/unregister %d", f.groupID),
	}}
}

func TestOwnerCanUnregisterRuntimeGroupAndDropOverrides(t *testing.T) {
	fixture := newUnregisterFixture(t)
	assertNonOwnerCannotUnregister(t, fixture)
	ownerDone := make(chan struct{})
	go func() {
		runRegistrationUpdate(t, fixture.bot, fixture.service, fixture.command(testOwner))
		close(ownerDone)
	}()
	assertRemovalPrecedesLeave(t, fixture)
	fixture.releaseLeave <- struct{}{}
	select {
	case <-ownerDone:
	case <-time.After(time.Second):
		t.Fatal("owner unregister did not finish after LeaveChat release")
	}
	assertUnregisterResult(t, fixture)
}

func assertNonOwnerCannotUnregister(t *testing.T, fixture *unregisterFixture) {
	t.Helper()
	runRegistrationUpdate(t, fixture.bot, fixture.service, fixture.command(testOwner+1))
	if !fixture.settings.IsGroup(fixture.groupID) {
		t.Fatal("non-owner unregistered a runtime group")
	}
	messages := fixture.caller.messagesTo(testOwner + 1)
	if len(messages) != 1 {
		t.Fatalf("non-owner unregister messages = %d, want 1", len(messages))
	}
	want := i18n.Messages.Bot.Registration.UnregisterOwnerOnly.For(i18n.LangEN)
	if messages[0].Text != want {
		t.Fatalf("non-owner unregister message = %q, want catalogue text %q", messages[0].Text, want)
	}
}

func assertRemovalPrecedesLeave(t *testing.T, fixture *unregisterFixture) {
	t.Helper()
	select {
	case groupID := <-fixture.caller.leaveStarted:
		if groupID != fixture.groupID {
			t.Errorf("LeaveChat started for %d, want %d", groupID, fixture.groupID)
		}
	case <-time.After(time.Second):
		t.Fatal("owner unregister did not reach LeaveChat")
	}
	select {
	case event := <-fixture.removed:
		if event.groupID != fixture.groupID || !event.durable {
			t.Errorf("group-removal transition before LeaveChat = %+v, want group %d after durable removal", event, fixture.groupID)
		}
	default:
		t.Error("LeaveChat started before the group-removal transition")
	}
}

func assertUnregisterResult(t *testing.T, fixture *unregisterFixture) {
	t.Helper()
	if fixture.settings.IsGroup(fixture.groupID) {
		t.Fatal("owner unregister did not remove the runtime group")
	}
	if left := fixture.caller.leftChats(); len(left) != 1 || left[0] != fixture.groupID {
		t.Fatalf("unregister leaves = %v, want [%d]", left, fixture.groupID)
	}
	messages := fixture.caller.messagesTo(testOwner)
	want := i18n.Messages.Bot.Registration.GroupUnregistered.Render(i18n.LangEN, "Remove")
	if len(messages) != 1 || messages[0].Text != want {
		t.Fatalf("owner unregister messages = %+v, want catalogue text %q", messages, want)
	}
	registration := fixture.settings.Registrations()
	if registration.ControlGroupID != 0 || len(registration.RegisteredGroups) != 0 ||
		len(registration.UnknownGroupLeaves) != 0 {
		t.Fatalf("registration state after unregister = %+v", registration)
	}
	assertNoPersistedGroupOverride(t, fixture)
}

func assertNoPersistedGroupOverride(t *testing.T, fixture *unregisterFixture) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture.stateDirectory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Groups map[string]json.RawMessage `json:"groups"`
	}
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted.Groups[fmt.Sprint(fixture.groupID)]; ok {
		t.Fatalf("orphaned override for group %d remained in settings.json", fixture.groupID)
	}
}
