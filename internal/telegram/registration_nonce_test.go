package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/mymmrac/telego"
)

func TestEnrollmentSecretMustMatchBeforeGroupRegistration(t *testing.T) {
	const (
		actor         = int64(901)
		rejectedGroup = int64(-1009000001601)
		acceptedGroup = int64(-1009000001602)
	)
	cfg, store := registrationFixture(t)
	now := time.Unix(2_000_000_000, 0)
	bindTestOwner(t, store, now)
	nonce, err := store.IssueEnrollmentNonce(testOwner, now, enrollmentLifetime)
	if err != nil {
		t.Fatal(err)
	}
	caller := &registrationCaller{members: map[[2]int64]telego.ChatMember{
		{rejectedGroup, actor}:     adminMember(actor),
		{rejectedGroup, testBotID}: adminMember(testBotID),
		{acceptedGroup, actor}:     adminMember(actor),
		{acceptedGroup, testBotID}: adminMember(testBotID),
	}}
	bot := newRegistrationBot(t, caller)
	service := newRegistrationService(
		context.Background(), bot, store, cfg, "verify_test_bot", testBotID, nil, nil, nil,
	)
	service.now = func() time.Time { return now }
	enrollment := func(groupID int64, title, secret string) telego.Update {
		return telego.Update{Message: &telego.Message{
			Chat: telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
			From: &telego.User{ID: actor, LanguageCode: "en"},
			Text: "/start enroll_" + secret,
		}}
	}

	runRegistrationUpdate(t, bot, service, enrollment(rejectedGroup, "Rejected", "wrong-secret"))
	if store.IsGroup(rejectedGroup) {
		t.Fatal("mismatched enrollment secret registered a group")
	}
	state := store.Registrations()
	if len(state.EnrollmentNonces) != 1 || state.EnrollmentNonces[0].Nonce != nonce.Nonce {
		t.Fatalf("mismatched enrollment secret consumed the owner's nonce: %+v", state.EnrollmentNonces)
	}

	runRegistrationUpdate(t, bot, service, enrollment(acceptedGroup, "Accepted", nonce.Nonce))
	if !store.IsGroup(acceptedGroup) {
		t.Fatal("matching enrollment secret did not register the group")
	}
	if remaining := store.Registrations().EnrollmentNonces; len(remaining) != 0 {
		t.Fatalf("matching enrollment secret left nonce unconsumed: %+v", remaining)
	}
}
