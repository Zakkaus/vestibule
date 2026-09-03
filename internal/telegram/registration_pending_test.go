package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestExpiredPendingRegistrationBecomesUnknownLeaveInsteadOfGroup(t *testing.T) {
	const (
		actor        = int64(902)
		expiredGroup = int64(-1009000001631)
		currentGroup = int64(-1009000001632)
	)
	now := time.Unix(2_000_000_000, 0)
	expired := settings.PendingRegistration{
		GroupID: expiredGroup, RegisteredBy: actor, Title: "Expired", ExpiresAt: now.Add(-time.Second).Unix(),
	}
	current := settings.PendingRegistration{
		GroupID: currentGroup, RegisteredBy: actor, Title: "Current", ExpiresAt: now.Add(time.Minute).Unix(),
	}
	cfg, store := registrationFixture(t)
	caller := &registrationCaller{
		members: map[[2]int64]telego.ChatMember{
			{expiredGroup, actor}:     adminMember(actor),
			{expiredGroup, testBotID}: adminMember(testBotID),
			{currentGroup, actor}:     adminMember(actor),
			{currentGroup, testBotID}: adminMember(testBotID),
		},
		leaveErrors: map[int64]error{expiredGroup: errors.New("leave unavailable")},
	}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(
		context.Background(), bot, store, cfg, "verify_test_bot", testBotID, nil, nil, nil,
	)
	service.now = func() time.Time { return now }
	commitPendingRegistrations(t, store, expired, current)
	membershipUpdate := func(pending settings.PendingRegistration) telego.Update {
		return telego.Update{MyChatMember: &telego.ChatMemberUpdated{
			Chat:          telego.Chat{ID: pending.GroupID, Type: telego.ChatTypeSupergroup, Title: pending.Title},
			From:          telego.User{ID: actor, LanguageCode: "en"},
			OldChatMember: plainMember(testBotID),
			NewChatMember: adminMember(testBotID),
		}}
	}

	runRegistrationUpdate(t, bot, service, membershipUpdate(expired))
	state := store.Registrations()
	if store.IsGroup(expiredGroup) {
		t.Fatal("expired pending registration became a registered group")
	}
	if _, ok := pendingRegistration(state, expiredGroup); ok {
		t.Fatal("expired pending registration remained pending instead of becoming an unknown-group leave")
	}
	leave, ok := unknownGroupLeave(state, expiredGroup)
	if !ok || leave.ExpiresAt != expired.ExpiresAt {
		t.Fatalf("expired pending registration cleanup evidence = %+v, want expiry %d", leave, expired.ExpiresAt)
	}

	runRegistrationUpdate(t, bot, service, membershipUpdate(current))
	if !store.IsGroup(currentGroup) {
		t.Fatal("current pending registration did not complete")
	}
}

func TestCompletePendingRejectsExpiredRegistrationAndAcceptsCurrentOne(t *testing.T) {
	const (
		actor        = int64(903)
		expiredGroup = int64(-1009000001641)
		currentGroup = int64(-1009000001642)
	)
	now := time.Unix(2_000_000_000, 0)
	expired := settings.PendingRegistration{
		GroupID: expiredGroup, RegisteredBy: actor, Title: "Expired", ExpiresAt: now.Add(-time.Second).Unix(),
	}
	current := settings.PendingRegistration{
		GroupID: currentGroup, RegisteredBy: actor, Title: "Current", ExpiresAt: now.Add(time.Minute).Unix(),
	}
	cfg, store := registrationFixture(t)
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{expiredGroup, testBotID}: adminMember(testBotID),
		{currentGroup, testBotID}: adminMember(testBotID),
	}}
	service := newRegistrationService(
		context.Background(), newRegistrationBot(t, caller), store, cfg,
		"verify_test_bot", testBotID, nil, nil, nil,
	)
	service.now = func() time.Time { return now }
	commitPendingRegistrations(t, store, expired, current)

	status, completed, err := service.completePending(context.Background(), expiredGroup, expired)
	if completed || store.IsGroup(expiredGroup) {
		t.Fatalf("expired pending registration completed with bot membership %d", status)
	}
	if !errors.Is(err, errEnrollmentInvalid) {
		t.Fatalf("expired pending registration error = %v, want %v", err, errEnrollmentInvalid)
	}

	status, completed, err = service.completePending(context.Background(), currentGroup, current)
	if err != nil || status != botMembershipAdmin || !completed || !store.IsGroup(currentGroup) {
		t.Fatalf("current pending registration result = (%d, %v, %v), want admin and completed", status, completed, err)
	}
}

func TestPendingRegistrationDeadlineDoesNotExpireBeforeWindow(t *testing.T) {
	const groupID = int64(-1009000001651)
	now := time.Unix(2_000_000_000, 0)
	pending := settings.PendingRegistration{
		GroupID: groupID, RegisteredBy: 904, Title: "Still current", ExpiresAt: now.Add(time.Minute).Unix(),
	}
	cfg, store := registrationFixture(t)
	commitPendingRegistrations(t, store, pending)
	caller := &registrationCaller{members: make(map[[2]int64]telego.ChatMember)}
	root, cancel := context.WithCancel(context.Background())
	service := newRegistrationService(
		root, newRegistrationBot(t, caller), store, cfg, "verify_test_bot", testBotID, nil, nil, nil,
	)
	t.Cleanup(func() {
		cancel()
		service.Wait()
	})
	service.now = func() time.Time { return now }

	service.handleUnknownLeaveDeadline(groupID, pending.Title)
	state := store.Registrations()
	if _, ok := pendingRegistration(state, groupID); !ok {
		t.Fatal("pending registration expired before its registration window elapsed")
	}
	if _, ok := unknownGroupLeave(state, groupID); ok {
		t.Fatal("current pending registration became an unknown-group leave early")
	}
	if left := caller.leftChats(); len(left) != 0 {
		t.Fatalf("current pending registration left early: %v", left)
	}

	service.now = func() time.Time { return time.Unix(pending.ExpiresAt, 0) }
	service.handleUnknownLeaveDeadline(groupID, pending.Title)
	if left := caller.leftChats(); len(left) != 1 || left[0] != groupID {
		t.Fatalf("elapsed pending registration leaves = %v, want [%d]", left, groupID)
	}
}

func TestPendingGroupPromotesOnlyAfterBotBecomesAdministrator(t *testing.T) {
	const groupID = int64(-1009000001671)
	now := time.Unix(2_000_000_000, 0)
	pending := settings.PendingRegistration{
		GroupID: groupID, RegisteredBy: 905, Title: "Needs admin", ExpiresAt: now.Add(time.Minute).Unix(),
	}
	cfg, store := registrationFixture(t)
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{groupID, testBotID}: plainMember(testBotID),
	}}
	service := newRegistrationService(
		context.Background(), newRegistrationBot(t, caller), store, cfg,
		"verify_test_bot", testBotID, nil, nil, nil,
	)
	service.now = func() time.Time { return now }
	commitPendingRegistrations(t, store, pending)

	status, completed, err := service.completePending(context.Background(), groupID, pending)
	if err != nil {
		t.Fatal(err)
	}
	if status != botMembershipMember || completed || store.IsGroup(groupID) {
		t.Fatalf("plain-member bot promoted pending group: status=%d completed=%v", status, completed)
	}
	if _, ok := pendingRegistration(store.Registrations(), groupID); !ok {
		t.Fatal("plain-member bot removed the pending registration")
	}

	caller.members[[2]int64{groupID, testBotID}] = adminMember(testBotID)
	status, completed, err = service.completePending(context.Background(), groupID, pending)
	if err != nil || status != botMembershipAdmin || !completed || !store.IsGroup(groupID) {
		t.Fatalf("administrator bot promotion result = (%d, %v, %v), want admin and completed", status, completed, err)
	}
}

func commitPendingRegistrations(t *testing.T, store *settings.Store, pending ...settings.PendingRegistration) {
	t.Helper()
	state := store.Registrations()
	state.PendingRegistrations = append(state.PendingRegistrations, pending...)
	if _, err := store.CommitRegistrations(state.Revision, state); err != nil {
		t.Fatal(err)
	}
}
