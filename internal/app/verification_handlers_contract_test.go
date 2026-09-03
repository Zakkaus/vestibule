package app

import (
	"fmt"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func TestVerificationAnswerHandlerReportsSettledChallenges(t *testing.T) {
	const applicantID int64 = 4301
	fixture := newDispatchFixture(t, 0)
	handlers := telegram.NewVerificationHandlers(fixture.verification, fixture.verificationGateway)
	before := len(fixture.caller.callbackAnswers())
	update := telego.Update{CallbackQuery: &telego.CallbackQuery{
		ID:   "answer-contract",
		From: telego.User{ID: applicantID, LanguageCode: "en"},
		Data: fmt.Sprintf(
			"%s%d:%d:missing:0", verification.AnswerCallbackPrefix, fixture.groupID, applicantID,
		),
	}}

	runDirectHandler(t, fixture.bot, handlers.Answer, update)

	answers := fixture.caller.callbackAnswers()
	if len(answers) != before+1 {
		t.Fatalf("answer handler acknowledgements = %d, want one new result", len(answers)-before)
	}
	got := answers[len(answers)-1]
	want := i18n.Messages.Verification.Result.AlreadyHandled.For(i18n.LangEN)
	if got.CallbackQueryID != "answer-contract" || got.Text != want || got.ShowAlert {
		t.Fatalf("answer handler acknowledgement = %#v, want settled result %q", got, want)
	}
}

func TestVerificationAdminHandlerReportsSettledChallenges(t *testing.T) {
	const (
		adminID     int64 = 4302
		applicantID int64 = 4303
	)
	fixture := newDispatchFixture(t, 0)
	handlers := telegram.NewVerificationHandlers(fixture.verification, fixture.verificationGateway)
	before := len(fixture.caller.callbackAnswers())
	update := telego.Update{CallbackQuery: &telego.CallbackQuery{
		ID:   "admin-contract",
		From: telego.User{ID: adminID, LanguageCode: "en"},
		Data: fmt.Sprintf(
			"%spass:%d:%d:missing", verification.AdminCallbackPrefix, fixture.groupID, applicantID,
		),
	}}

	runDirectHandler(t, fixture.bot, handlers.AdminAction, update)

	answers := fixture.caller.callbackAnswers()
	if len(answers) != before+1 {
		t.Fatalf("administrator handler acknowledgements = %d, want one new result", len(answers)-before)
	}
	got := answers[len(answers)-1]
	want := i18n.Messages.Verification.Admin.AlreadyHandled.For(i18n.LangEN)
	if got.CallbackQueryID != "admin-contract" || got.Text != want || !got.ShowAlert {
		t.Fatalf("administrator handler acknowledgement = %#v, want settled alert %q", got, want)
	}
}

func TestVerificationMemberHandlerStartsPostJoinVerification(t *testing.T) {
	const applicantID int64 = 4304
	fixture := newDispatchFixture(t, 0)
	fixture.caller.setMember(fixture.groupID, applicantID, &telego.ChatMemberMember{
		Status: telego.MemberStatusMember,
		User:   telego.User{ID: applicantID, FirstName: "Applicant", LanguageCode: "en"},
	})
	handlers := telegram.NewVerificationHandlers(fixture.verification, fixture.verificationGateway)
	beforeRestrictions := fixture.caller.methodCount("restrictChatMember")
	beforeMessages := len(fixture.caller.sentTexts())
	update := telego.Update{ChatMember: &telego.ChatMemberUpdated{
		Chat: telego.Chat{ID: fixture.groupID, Type: telego.ChatTypeSupergroup},
		From: telego.User{ID: applicantID},
		OldChatMember: &telego.ChatMemberLeft{
			Status: telego.MemberStatusLeft,
			User:   telego.User{ID: applicantID},
		},
		NewChatMember: &telego.ChatMemberMember{
			Status: telego.MemberStatusMember,
			User:   telego.User{ID: applicantID, FirstName: "Applicant", LanguageCode: "en"},
		},
	}}

	runDirectHandler(t, fixture.bot, handlers.MemberJoined, update)

	if got := fixture.caller.methodCount("restrictChatMember") - beforeRestrictions; got != 1 {
		t.Fatalf("member-joined restrictions = %d, want one verification hold", got)
	}
	if got := len(fixture.caller.sentTexts()) - beforeMessages; got == 0 {
		t.Fatal("member-joined handler sent no verification challenge")
	}
}
