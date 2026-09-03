package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

type applicantMembershipLookupFailureBot struct {
	*fakeVerifyBot
	applicantID int64
	err         error
}

func (b *applicantMembershipLookupFailureBot) Member(ctx context.Context, chatID, userID int64) (ChatMember, error) {
	if userID != b.applicantID {
		return b.fakeVerifyBot.Member(ctx, chatID, userID)
	}
	request := memberRequest{UserID: userID}
	request.ChatID.ID = chatID
	b.memberRequests = append(b.memberRequests, request)
	return nil, b.err
}

func TestKernelPassRefusesApplicantLookupFailureWhenBotCanReadRequiredChannel(t *testing.T) {
	const (
		groupID           int64 = -1009000000841
		requiredChannelID int64 = -1009000000842
		applicantID       int64 = 71
		botID             int64 = 72
	)
	service := newTestService(&settings.Config{
		Groups:   []settings.GroupConfig{{ID: groupID}},
		GroupIDs: []int64{groupID},
	})
	if err := service.updateGroupSettings(groupID, func(_ settings.GroupView, overrides *settings.GroupOverrides) {
		channelID := requiredChannelID
		display := "@required"
		overrides.RequiredChannelID = &channelID
		overrides.ChannelDisplay = &display
	}); err != nil {
		t.Fatal(err)
	}
	key := pkey{gid: groupID, uid: applicantID}
	service.pend[key] = &pending{
		mode: settings.ModeKernel, nonce: "channel-read-failure", lang: i18n.LangZH,
		prompted: true, groupMsgID: 42, deadline: time.Now().Add(time.Hour),
	}
	service.botID = botID
	base := newFakeVerifyBot()
	base.memberByID = map[int64]ChatMember{
		botID: &ChatMemberMember{Status: MemberStatusMember},
	}
	bot := &applicantMembershipLookupFailureBot{
		fakeVerifyBot: base, applicantID: applicantID, err: errors.New("applicant membership lookup failed"),
	}

	service.finishKernelPass(context.Background(), bot, groupID, applicantID, "channel-read-failure", i18n.LangZH, i18n.LangZH)

	if base.approves != 0 {
		t.Errorf("approvals = %d, want 0 after applicant membership lookup failure", base.approves)
	}
	if pending := service.pend[key]; pending == nil || pending.done {
		t.Errorf("pending after unreadable applicant lookup = %#v, want a live record", pending)
	}
	passed := service.voice(gateRequest).Passed.For(i18n.LangZH)
	for _, text := range base.sendTexts {
		if text == passed {
			t.Errorf("applicant result included pass text %q after unreadable membership lookup", passed)
		}
	}
	if len(base.memberRequests) != 2 {
		t.Fatalf("channel member requests = %d, want applicant and bot self-probe", len(base.memberRequests))
	}
	for index, userID := range []int64{applicantID, botID} {
		request := base.memberRequests[index]
		if request.ChatID.ID != requiredChannelID || request.UserID != userID {
			t.Errorf("channel member request %d = chat:%d user:%d, want chat:%d user:%d",
				index, request.ChatID.ID, request.UserID, requiredChannelID, userID)
		}
	}
}

// An applicant lookup can fail even though the bot can still read its own channel membership.
// The applicant is refused until the lookup works, but that operational failure must not become
// a strike or a ban; a confirmed member remains the positive control.
func TestUnreadableApplicantLookupDoesNotChargeTheApplicant(t *testing.T) {
	const (
		groupID           int64 = -1009000000843
		requiredChannelID int64 = -1009000000844
		applicantID       int64 = 843
		botID             int64 = 844
	)
	newService := func() *Service {
		return newTestService(&settings.Config{
			GroupIDs:          []int64{groupID},
			RequiredChannelID: requiredChannelID,
			VerifyMaxFails:    3,
		})
	}

	service := newService()
	service.botID = botID
	key := pkey{gid: groupID, uid: applicantID}
	unreadablePending := &pending{nonce: "unreadable-applicant", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour)}
	service.pend[key] = unreadablePending
	base := newFakeVerifyBot()
	base.memberByID = map[int64]ChatMember{
		botID: &ChatMemberMember{Status: MemberStatusMember},
	}
	unreadable := &applicantMembershipLookupFailureBot{
		fakeVerifyBot: base, applicantID: applicantID, err: errors.New("applicant membership lookup failed"),
	}

	if service.isChannelMember(context.Background(), unreadable, groupID, applicantID, i18n.LangEN) {
		t.Fatal("an unreadable required-channel lookup must not silently admit the applicant")
	}
	if !service.channelWasUnreadable(groupID, applicantID) {
		t.Fatal("an unreadable applicant lookup must be remembered so the applicant is not charged for it")
	}
	if _, banned := service.finishDecline(context.Background(), unreadable, groupID, applicantID, unreadablePending, "timeout"); banned {
		t.Fatal("an unreadable required-channel lookup must not cause an automatic ban")
	}
	service.mu.Lock()
	strikes := len(service.vfail)
	service.mu.Unlock()
	if strikes != 0 {
		t.Errorf("strike records = %d, want 0: the bot could not read the applicant's channel membership", strikes)
	}

	confirmed := newService()
	confirmed.botID = botID
	confirmed.pend[key] = &pending{nonce: "confirmed-member", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour)}
	healthy := newFakeVerifyBot()
	healthy.memberByID = map[int64]ChatMember{
		applicantID: &ChatMemberMember{Status: MemberStatusMember},
	}
	if !confirmed.isChannelMember(context.Background(), healthy, groupID, applicantID, i18n.LangEN) {
		t.Fatal("a confirmed required-channel member must pass the gate")
	}
	if confirmed.channelWasUnreadable(groupID, applicantID) {
		t.Fatal("a readable membership answer must clear the unreadable marker")
	}
}
