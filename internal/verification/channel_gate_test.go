package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// An applicant stuck behind a channel gate the bot cannot read is stuck through no fault of
// their own. Refusing entry is right; charging them a strike toward an automatic ban is not.
func TestUnreadableChannelGateDoesNotStrike(t *testing.T) {
	cases := []struct {
		name     string
		failOpen *bool
	}{
		{name: "strict deployment", failOpen: boolPtr(false)},
		{name: "default deployment", failOpen: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &settings.Config{RequiredChannelID: -400, VerifyMaxFails: 3, RequiredChannelFailOpen: tc.failOpen}
			v := newTestService(cfg)
			v.botID = 42
			gid, uid := int64(-100), int64(5)
			fb := &fakeVerifyBot{memberErr: errors.New("CHAT_ADMIN_REQUIRED")}

			for round := 1; round <= 3; round++ {
				p := &pending{nonce: "n", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour)}
				v.pend[pkey{gid, uid}] = p
				v.isChannelMember(context.Background(), fb, gid, uid, i18n.LangEN)
				if _, banned := v.finishDecline(context.Background(), fb, gid, uid, p, "timeout"); banned {
					t.Fatalf("round %d: an unreadable gate must never reach an automatic ban", round)
				}
			}
			v.mu.Lock()
			strikes := len(v.vfail)
			v.mu.Unlock()
			if strikes != 0 {
				t.Errorf("strike records = %d, want 0: the bot could not read the gate", strikes)
			}
			if fb.bans != 0 {
				t.Errorf("bans = %d, want 0", fb.bans)
			}
			if fb.declines != 3 {
				t.Errorf("declines = %d, want 3: the applicant is still refused entry", fb.declines)
			}
		})
	}
}

// A readable gate clears the marker, so someone who simply never joined still carries the strike.
func TestReadableChannelGateStillStrikes(t *testing.T) {
	cfg := &settings.Config{RequiredChannelID: -400, VerifyMaxFails: 3}
	v := newTestService(cfg)
	v.botID = 42
	gid, uid := int64(-100), int64(6)
	p := &pending{nonce: "n", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour), channelUnreadable: true}
	v.pend[pkey{gid, uid}] = p
	fb := &fakeVerifyBot{member: &ChatMemberLeft{Status: MemberStatusLeft}}

	if v.isChannelMember(context.Background(), fb, gid, uid, i18n.LangEN) {
		t.Fatal("a confirmed non-member does not pass the gate")
	}
	if _, _ = v.finishDecline(context.Background(), fb, gid, uid, p, "timeout"); true {
		v.mu.Lock()
		strikes := len(v.vfail)
		v.mu.Unlock()
		if strikes != 1 {
			t.Errorf("strike records = %d, want 1: a readable gate clears any earlier failure marker", strikes)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

// When the bot cannot read the required channel, telling the applicant to join it is
// misleading. The prompt must explain the access problem; a confirmed member is the control.
func TestUnreadableChannelGateExplainsTheAccessProblem(t *testing.T) {
	const (
		groupID           int64 = -1009000000851
		requiredChannelID int64 = -1009000000852
		applicantID       int64 = 851
		botID             int64 = 852
	)
	cfg := &settings.Config{
		GroupIDs:                []int64{groupID},
		RequiredChannelID:       requiredChannelID,
		RequiredChannelFailOpen: boolPtr(false),
	}
	service := newTestService(cfg)
	service.botID = botID
	service.pend[pkey{gid: groupID, uid: applicantID}] = &pending{
		nonce: "unreadable-prompt", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour),
	}
	unreadable := &fakeVerifyBot{memberErr: errors.New("channel lookup unavailable")}

	if service.isChannelMember(context.Background(), unreadable, groupID, applicantID, i18n.LangEN) {
		t.Fatal("an unreadable required channel must not silently admit the applicant in a fail-closed group")
	}
	if _, err := service.sendChannelPrompt(context.Background(), unreadable, groupID, applicantID, i18n.LangEN); err != nil {
		t.Fatalf("send unreadable-channel prompt: %v", err)
	}
	want := service.messages.Verification.Channel.Unreadable.For(i18n.LangEN)
	if unreadable.lastSendText != want {
		t.Errorf("unreadable-channel prompt = %q, want %q; the applicant must not be told to join a channel the bot cannot read", unreadable.lastSendText, want)
	}

	confirmed := newTestService(cfg)
	confirmed.botID = botID
	confirmed.pend[pkey{gid: groupID, uid: applicantID}] = &pending{
		nonce: "confirmed-prompt", lang: i18n.LangEN, deadline: time.Now().Add(time.Hour),
	}
	healthy := &fakeVerifyBot{member: &ChatMemberMember{Status: MemberStatusMember}}
	if !confirmed.isChannelMember(context.Background(), healthy, groupID, applicantID, i18n.LangEN) {
		t.Fatal("a confirmed required-channel member must pass the same gate")
	}
}

// The unreadable marker must survive a restart: otherwise a later timeout turns a channel
// outage into the applicant's strike and eventually an automatic ban.
func TestUnreadableChannelGateSurvivesRestart(t *testing.T) {
	const (
		groupID           int64 = -1009000000861
		requiredChannelID int64 = -1009000000862
		applicantID       int64 = 861
	)
	cfg := &settings.Config{
		GroupIDs:          []int64{groupID},
		RequiredChannelID: requiredChannelID,
		VerifyMaxFails:    3,
	}
	dir := t.TempDir()
	key := pkey{gid: groupID, uid: applicantID}
	seed := newTestService(cfg)
	seed.statePath = dir + "/pending.json"
	seed.pend[key] = &pending{
		nonce: "unreadable-restart", mode: settings.ModeKernel, lang: i18n.LangEN, deadline: time.Now().Add(time.Hour), channelUnreadable: true,
	}
	seed.save()

	restored := newTestService(cfg)
	restored.statePath = dir + "/pending.json"
	restored.load(&fakeVerifyBot{})
	pending, ok := restored.pend[key]
	if !ok {
		t.Fatal("the pending verification must survive the restart")
	}
	if !restored.channelWasUnreadable(groupID, applicantID) {
		t.Fatal("an unreadable channel gate must remain unreadable after restart")
	}
	if _, banned := restored.finishDecline(context.Background(), &fakeVerifyBot{}, groupID, applicantID, pending, "timeout"); banned {
		t.Fatal("a restored unreadable channel gate must not cause an automatic ban")
	}
	restored.mu.Lock()
	strikes := len(restored.vfail)
	restored.mu.Unlock()
	if strikes != 0 {
		t.Errorf("restored unreadable gate created %d strike records, want 0", strikes)
	}
}
