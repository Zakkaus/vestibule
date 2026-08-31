package telegram

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
)

func TestOwnerClaimPersistenceFailureUsesClaimMessage(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0)
	caller := &registrationCaller{
		members: make(map[[2]int64]telego.ChatMember),
		events:  make(chan string, 4),
	}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }
	if err := service.EnsureOwnerClaim(); err != nil {
		t.Fatal(err)
	}
	claim := settings.Registrations().OwnerClaimNonce
	if err := os.RemoveAll(stateDirectory); err != nil {
		t.Fatal(err)
	}
	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: testOwner, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: testOwner, LanguageCode: "en"},
		Text: "/start owner_" + claim,
	}})
	waitForRegistrationMethod(t, caller, "sendMessage")
	messages := caller.messagesTo(testOwner)
	if len(messages) != 1 {
		t.Fatalf("owner-claim failure messages = %d, want 1", len(messages))
	}
	want := i18n.Messages.Bot.Registration.OwnerClaimSaveFailed.For(i18n.LangEN)
	if messages[0].Text != want {
		t.Fatalf("owner-claim persistence failure message = %q, want catalogue text %q", messages[0].Text, want)
	}
}

func TestUnknownCleanupRecordCannotAuthorizePromotion(t *testing.T) {
	const (
		actor   = int64(80)
		groupID = int64(-4005)
	)
	cfg, settings := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	state := settings.Registrations()
	state.UnknownGroupLeaves = []store.UnknownGroupLeave{{
		GroupID: groupID, Title: "Cleanup only", ExpiresAt: now.Add(registrationPending).Unix(),
	}}
	if _, err := settings.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, actor}: adminMember(actor),
	}}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }
	runRegistrationUpdate(t, bot, service, telego.Update{MyChatMember: &telego.ChatMemberUpdated{
		Chat:          telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Cleanup only"},
		From:          telego.User{ID: actor, LanguageCode: "en"},
		OldChatMember: plainMember(testBotID),
		NewChatMember: adminMember(testBotID),
	}})
	if settings.IsGroup(groupID) {
		t.Fatal("unknown-group cleanup record authorized registration")
	}
	if left := caller.leftChats(); len(left) != 1 || left[0] != groupID {
		t.Fatalf("cleanup-only promotion leaves = %v, want [%d]", left, groupID)
	}
}

func TestOwnerClaimLifetimePinAndOpenStateLog(t *testing.T) {
	cfg, settings := registrationFixture(t)
	cfg.OwnerClaimLifetimeSeconds = 45
	cfg.OwnerClaimUserID = testOwner
	now := time.Unix(2_000_000_000, 0)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	logs := newSynchronizedLog()
	oldLog := log.Writer()
	log.SetOutput(logs)
	defer log.SetOutput(oldLog)
	if err := service.EnsureOwnerClaim(); err != nil {
		t.Fatal(err)
	}
	state := settings.Registrations()
	if state.OwnerClaimExpiresAt != now.Add(45*time.Second).Unix() {
		t.Fatalf("owner claim expiry = %d, want %d", state.OwnerClaimExpiresAt, now.Add(45*time.Second).Unix())
	}
	if !strings.Contains(logs.String(), "OWNER UNCLAIMED") {
		t.Fatalf("open owner claim was not visible in the journal: %s", logs.String())
	}
	claimUpdate := func(userID int64) telego.Update {
		return telego.Update{Message: &telego.Message{
			Chat: telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
			From: &telego.User{ID: userID, LanguageCode: "en"},
			Text: "/start owner_" + state.OwnerClaimNonce,
		}}
	}
	runRegistrationUpdate(t, bot, service, claimUpdate(testOwner+1))
	if got := settings.Registrations(); got.OwnerID != 0 || got.OwnerClaimNonce == "" {
		t.Fatalf("unmatched pinned claimant changed owner state: %+v", got)
	}
	refused := caller.messagesTo(testOwner + 1)
	wantRefused := i18n.Messages.Bot.Registration.OwnerClaimRefused.For(i18n.LangEN)
	if len(refused) != 1 || refused[0].Text != wantRefused {
		t.Fatalf("pinned claimant refusal = %+v, want catalogue text %q", refused, wantRefused)
	}
	runRegistrationUpdate(t, bot, service, claimUpdate(testOwner))
	if got := settings.Registrations(); got.OwnerID != testOwner || got.OwnerClaimNonce != "" {
		t.Fatalf("pinned owner claim state = %+v", got)
	}
}

func TestEnrollmentCommandIssuesDurableOneUseLink(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: testOwner, Type: telego.ChatTypePrivate},
		From: &telego.User{ID: testOwner, LanguageCode: "en"},
		Text: "/enroll",
	}})

	state := registrationStateFromDisk(t, configPath, stateDirectory)
	if state.OwnerID != testOwner || len(state.EnrollmentNonces) != 1 {
		t.Fatalf("enrollment issuance state = %+v, want owner and one nonce", state)
	}
	nonce := state.EnrollmentNonces[0]
	if nonce.Nonce == "" || nonce.IssuedBy != testOwner || nonce.ExpiresAt != now.Add(enrollmentLifetime).Unix() {
		t.Fatalf("issued enrollment nonce = %+v", nonce)
	}
	messages := caller.messagesTo(testOwner)
	if len(messages) != 1 {
		t.Fatalf("enrollment issuance messages = %d, want 1", len(messages))
	}
	link := fmt.Sprintf("https://t.me/%s?startgroup=enroll_%s", "verify_test_bot", nonce.Nonce)
	want := i18n.Messages.Bot.Registration.EnrollmentLink.Render(
		i18n.LangEN, int(enrollmentLifetime/time.Minute), link)
	if messages[0].Text != want {
		t.Fatalf("enrollment issuance message = %q, want catalogue text %q", messages[0].Text, want)
	}
	if strings.Contains(link, "?start=") || !strings.Contains(link, "?startgroup=enroll_") {
		t.Fatalf("enrollment link = %q, want one-use startgroup payload", link)
	}
	if left := caller.leftChats(); len(left) != 0 {
		t.Fatalf("enrollment issuance leaves = %v, want none", left)
	}
}

func TestEnrollmentPayloadRejectsNonAdminAndLeaves(t *testing.T) {
	const (
		actor    = int64(77)
		groupID  = int64(-5001)
		operator = int64(-5002)
		title    = "Non-admin"
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminLogChatID = operator
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	before := settings.Registrations()
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, actor}:     plainMember(actor),
		{groupID, testBotID}: adminMember(testBotID),
	}}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})

	after := settings.Registrations()
	assertRegistrationStateEqual(t, after, before)
	assertRegistrationStateEqual(t, registrationStateFromDisk(t, configPath, stateDirectory), before)
	if settings.IsGroup(groupID) {
		t.Fatal("non-admin enrollment registered a group")
	}
	if left := caller.leftChats(); !reflect.DeepEqual(left, []int64{groupID}) {
		t.Fatalf("non-admin enrollment leaves = %v, want [%d]", left, groupID)
	}
	refusal := caller.messagesTo(groupID)
	wantRefusal := i18n.Messages.Bot.Registration.EnrollmentRefused.For(
		i18n.FromStored(cfg.LangForGroup(groupID)))
	if len(refusal) != 1 || refusal[0].Text != wantRefusal {
		t.Fatalf("non-admin enrollment refusal = %+v, want catalogue text %q", refusal, wantRefusal)
	}
	unauthorized := caller.messagesTo(operator)
	wantUnauthorized := i18n.Messages.Bot.Lifecycle.UnauthorizedChat.Render(
		i18n.FromStored(cfg.LangForGroup(0)), title, groupID, telego.ChatTypeSupergroup)
	if len(unauthorized) != 1 || unauthorized[0].Text != wantUnauthorized {
		t.Fatalf("non-admin enrollment operator result = %+v, want catalogue text %q", unauthorized, wantUnauthorized)
	}
}

func TestEnrollmentCommitFailureRetainsNonceAndReportsPersistence(t *testing.T) {
	const (
		actor    = int64(78)
		groupID  = int64(-5003)
		operator = int64(-5004)
		title    = "Commit failure"
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminLogChatID = operator
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	before := settings.Registrations()
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, actor}:     adminMember(actor),
		{groupID, testBotID}: adminMember(testBotID),
	}}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }
	if err := os.RemoveAll(stateDirectory); err != nil {
		t.Fatal(err)
	}

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})

	after := settings.Registrations()
	assertRegistrationStateEqual(t, after, before)
	if settings.IsGroup(groupID) {
		t.Fatal("failed enrollment commit registered a group")
	}
	if status := settings.Persistence(); status.LastError == nil {
		t.Fatalf("failed enrollment commit persistence = %+v, want retained write error", status)
	}
	if left := caller.leftChats(); !reflect.DeepEqual(left, []int64{groupID}) {
		t.Fatalf("failed enrollment commit leaves = %v, want [%d]", left, groupID)
	}
	failure := caller.messagesTo(groupID)
	wantFailure := i18n.Messages.Bot.Registration.RegistrationSaveFailed.For(i18n.LangEN)
	if len(failure) != 1 || failure[0].Text != wantFailure {
		t.Fatalf("failed enrollment commit response = %+v, want catalogue text %q", failure, wantFailure)
	}
	unauthorized := caller.messagesTo(operator)
	wantUnauthorized := i18n.Messages.Bot.Lifecycle.UnauthorizedChat.Render(
		i18n.FromStored(cfg.LangForGroup(0)), title, groupID, telego.ChatTypeSupergroup)
	if len(unauthorized) != 1 || unauthorized[0].Text != wantUnauthorized {
		t.Fatalf("failed enrollment commit operator result = %+v, want catalogue text %q", unauthorized, wantUnauthorized)
	}
}

func TestEnrollmentTransportFailuresRetainDurableState(t *testing.T) {
	t.Run("membership lookup", testEnrollmentMembershipLookupFailure)
	t.Run("leave", testEnrollmentLeaveFailure)
	t.Run("response send", testEnrollmentResponseSendFailure)
}

func testEnrollmentMembershipLookupFailure(t *testing.T) {
	const (
		actor    = int64(79)
		groupID  = int64(-5005)
		operator = int64(-5006)
		title    = "Membership lookup"
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminLogChatID = operator
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	before := settings.Registrations()
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{groupID, actor}: adminMember(actor),
		},
		memberErrors: map[[2]int64]error{
			{groupID, testBotID}: fmt.Errorf("getChatMember unavailable"),
		},
	}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})

	if requests := caller.membershipRequestsForTest(); !reflect.DeepEqual(requests,
		[][2]int64{{groupID, actor}, {groupID, testBotID}}) {
		t.Fatalf("enrollment membership requests = %v, want actor then bot", requests)
	}
	after := settings.Registrations()
	assertRegistrationStateEqual(t, after, before)
	assertRegistrationStateEqual(t, registrationStateFromDisk(t, configPath, stateDirectory), before)
	if settings.IsGroup(groupID) {
		t.Fatal("unreadable bot membership registered a group")
	}
	if left := caller.leftChats(); !reflect.DeepEqual(left, []int64{groupID}) {
		t.Fatalf("unreadable bot membership leaves = %v, want [%d]", left, groupID)
	}
	refusal := caller.messagesTo(groupID)
	wantRefusal := i18n.Messages.Bot.Registration.EnrollmentRefused.For(
		i18n.FromStored(cfg.LangForGroup(groupID)))
	if len(refusal) != 1 || refusal[0].Text != wantRefusal {
		t.Fatalf("unreadable bot membership response = %+v, want catalogue text %q", refusal, wantRefusal)
	}
	unauthorized := caller.messagesTo(operator)
	wantUnauthorized := i18n.Messages.Bot.Lifecycle.UnauthorizedChat.Render(
		i18n.FromStored(cfg.LangForGroup(0)), title, groupID, telego.ChatTypeSupergroup)
	if len(unauthorized) != 1 || unauthorized[0].Text != wantUnauthorized {
		t.Fatalf("unreadable bot membership operator result = %+v, want catalogue text %q", unauthorized, wantUnauthorized)
	}
}

func testEnrollmentLeaveFailure(t *testing.T) {
	const (
		actor    = int64(80)
		groupID  = int64(-5007)
		operator = int64(-5008)
		title    = "Leave failure"
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AdminLogChatID = operator
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	state := settings.Registrations()
	state.UnknownGroupLeaves = []store.UnknownGroupLeave{{
		GroupID: groupID, Title: title, ExpiresAt: now.Add(registrationPending).Unix(),
	}}
	if _, err := settings.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}
	before := settings.Registrations()
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{groupID, actor}: plainMember(actor),
		},
		leaveErrors: map[int64]error{
			groupID: fmt.Errorf("leaveChat unavailable"),
		},
	}
	bot := newRegistrationBot(t, caller)
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newRegistrationService(root, bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})

	after := settings.Registrations()
	assertRegistrationStateEqual(t, after, before)
	assertRegistrationStateEqual(t, registrationStateFromDisk(t, configPath, stateDirectory), before)
	if len(after.UnknownGroupLeaves) != 1 || after.UnknownGroupLeaves[0].GroupID != groupID {
		t.Fatalf("failed leave cleanup evidence = %+v, want group %d record", after.UnknownGroupLeaves, groupID)
	}
	if attempts := caller.leaveAttemptsForTest(); !reflect.DeepEqual(attempts, []int64{groupID}) {
		t.Fatalf("failed leave attempts = %v, want [%d]", attempts, groupID)
	}
	if left := caller.leftChats(); len(left) != 0 {
		t.Fatalf("failed leave completed leaves = %v, want none", left)
	}
	refusal := caller.messagesTo(groupID)
	wantRefusal := i18n.Messages.Bot.Registration.EnrollmentRefused.For(
		i18n.FromStored(cfg.LangForGroup(groupID)))
	if len(refusal) != 1 || refusal[0].Text != wantRefusal {
		t.Fatalf("failed leave response = %+v, want catalogue text %q", refusal, wantRefusal)
	}
	if unauthorized := caller.messagesTo(operator); len(unauthorized) != 0 {
		t.Fatalf("failed leave operator result = %+v, want none before leaving", unauthorized)
	}
}

func testEnrollmentResponseSendFailure(t *testing.T) {
	const (
		actor   = int64(81)
		groupID = int64(-5009)
		title   = "Response failure"
	)
	configPath := filepath.Join(t.TempDir(), "missing-config.json")
	stateDirectory := t.TempDir()
	cfg, settings, err := loadRuntimeState(configPath, stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, settings, now)
	nonce, err := settings.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	before := settings.Registrations()
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{groupID, actor}:     adminMember(actor),
			{groupID, testBotID}: adminMember(testBotID),
		},
		sendErrors: map[int64]error{
			actor: fmt.Errorf("sendMessage unavailable"),
		},
	}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(context.Background(), bot, settings, cfg, "verify_test_bot", testBotID, nil, nil, nil)
	service.now = func() time.Time { return now }

	runRegistrationUpdate(t, bot, service, telego.Update{Message: &telego.Message{
		Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		From: &telego.User{ID: actor, LanguageCode: "en"},
		Text: "/start enroll_" + nonce.Nonce,
	}})

	after := settings.Registrations()
	assertResponseFailureRegistration(
		t, settings, before, after, configPath, stateDirectory, groupID, actor, title,
	)
	assertResponseFailureMessages(t, caller, groupID, actor, title)
}

func assertResponseFailureRegistration(
	t *testing.T,
	settings *store.Settings,
	before store.RegistrationState,
	after store.RegistrationState,
	configPath string,
	stateDirectory string,
	groupID int64,
	actor int64,
	title string,
) {
	t.Helper()
	if after.Revision != before.Revision+1 || !settings.IsGroup(groupID) ||
		len(after.RegisteredGroups) != 1 || len(after.EnrollmentNonces) != 0 ||
		len(after.PendingRegistrations) != 0 || len(after.UnknownGroupLeaves) != 0 {
		t.Fatalf("response failure registration state = %+v, want durable completed registration", after)
	}
	registered := after.RegisteredGroups[0]
	if registered.ID != groupID || registered.RegisteredBy != actor || registered.Title != title {
		t.Fatalf("response failure registered group = %+v", registered)
	}
	assertRegistrationStateEqual(t, registrationStateFromDisk(t, configPath, stateDirectory), after)
}

func assertResponseFailureMessages(
	t *testing.T,
	caller *registrationCaller,
	groupID int64,
	actor int64,
	title string,
) {
	t.Helper()
	want := i18n.Messages.Bot.Registration.GroupRegistered.Render(i18n.LangEN, title)
	attempts := caller.sendAttemptsTo(actor)
	if len(attempts) != 1 || attempts[0].Text != want {
		t.Fatalf("response failure direct attempt = %+v, want catalogue text %q", attempts, want)
	}
	if direct := caller.messagesTo(actor); len(direct) != 0 {
		t.Fatalf("response failure delivered direct messages = %+v, want none", direct)
	}
	fallback := caller.messagesTo(groupID)
	if len(fallback) != 1 || fallback[0].Text != want {
		t.Fatalf("response failure fallback = %+v, want catalogue text %q", fallback, want)
	}
	if left := caller.leftChats(); len(left) != 0 {
		t.Fatalf("response failure leaves = %v, want none", left)
	}
}
