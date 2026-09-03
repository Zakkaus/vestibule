package verification

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// trustedSourceGateway gives each configured trusted source its own membership answer.
// The production gateway is asked about a chat, not just a user, so a single user can be
// absent from one source and present in another.
type trustedSourceGateway struct {
	*fakeVerifyBot
	memberAtChat map[int64]ChatMember
}

func (b *trustedSourceGateway) Member(_ context.Context, chatID, userID int64) (ChatMember, error) {
	request := memberRequest{UserID: userID}
	request.ChatID.ID = chatID
	b.memberRequests = append(b.memberRequests, request)
	if member, ok := b.memberAtChat[chatID]; ok {
		return member, nil
	}
	return &ChatMemberLeft{Status: MemberStatusLeft}, nil
}

func newTrustedSourceService(groupID int64, sources []int64) *Service {
	return newTestService(&settings.Config{
		GroupIDs: []int64{groupID},
		Groups: []settings.GroupConfig{{
			ID:                    groupID,
			TrustedMemberGroupIDs: sources,
		}},
	})
}

// A blank value and the destination group are invalid trust sources. They must not admit an
// applicant without a challenge, while a later valid source still must be considered.
func TestTrustedBypassIgnoresInvalidSourcesAndContinuesToLaterSource(t *testing.T) {
	const (
		groupID      int64 = -1009000000801
		firstSource  int64 = -1009000000802
		secondSource int64 = -1009000000803
		applicantID  int64 = 801
	)

	member := &ChatMemberMember{Status: MemberStatusMember}
	left := &ChatMemberLeft{Status: MemberStatusLeft}
	for _, tc := range []struct {
		name        string
		sources     []int64
		members     map[int64]ChatMember
		wantTrusted bool
	}{
		{
			name:        "blank source",
			sources:     []int64{0},
			members:     map[int64]ChatMember{0: member},
			wantTrusted: false,
		},
		{
			name:        "destination group source",
			sources:     []int64{groupID},
			members:     map[int64]ChatMember{groupID: member},
			wantTrusted: false,
		},
		{
			name:        "later configured source",
			sources:     []int64{firstSource, secondSource},
			members:     map[int64]ChatMember{firstSource: left, secondSource: member},
			wantTrusted: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := newTrustedSourceService(groupID, tc.sources)
			bot := &trustedSourceGateway{
				fakeVerifyBot: newFakeVerifyBot(),
				memberAtChat:  tc.members,
			}

			handled, trusted := service.tryTrustedBypass(context.Background(), bot, groupID, applicantID)
			if handled != tc.wantTrusted || trusted != tc.wantTrusted {
				t.Fatalf("trusted bypass = handled %v trusted %v, want %v; an invalid source must not auto-approve an applicant and a later valid source must not be skipped", handled, trusted, tc.wantTrusted)
			}
			wantApprovals := 0
			if tc.wantTrusted {
				wantApprovals = 1
			}
			if bot.approves != wantApprovals {
				t.Errorf("approvals = %d, want %d", bot.approves, wantApprovals)
			}
		})
	}
}

// Post-join verification uses the same configured source list but has no join request to
// approve. Invalid entries must start ordinary verification; a later genuine source exempts
// the member instead.
func TestPostJoinTrustIgnoresInvalidSourcesAndContinuesToLaterSource(t *testing.T) {
	const (
		groupID      int64 = -1009000000811
		firstSource  int64 = -1009000000812
		secondSource int64 = -1009000000813
		applicantID  int64 = 811
	)

	member := &ChatMemberMember{Status: MemberStatusMember}
	left := &ChatMemberLeft{Status: MemberStatusLeft}
	for _, tc := range []struct {
		name       string
		sources    []int64
		members    map[int64]ChatMember
		wantExempt bool
	}{
		{
			name:       "blank source",
			sources:    []int64{0},
			members:    map[int64]ChatMember{0: member},
			wantExempt: false,
		},
		{
			name:       "destination group source",
			sources:    []int64{groupID},
			members:    map[int64]ChatMember{groupID: member},
			wantExempt: false,
		},
		{
			name:       "later configured source",
			sources:    []int64{firstSource, secondSource},
			members:    map[int64]ChatMember{firstSource: left, secondSource: member},
			wantExempt: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := newTrustedSourceService(groupID, tc.sources)
			service.botUsername = "bot"
			t.Cleanup(service.stopForShutdown)
			bot := &trustedSourceGateway{
				fakeVerifyBot: newFakeVerifyBot(),
				memberAtChat:  tc.members,
			}

			runFakeHandler(t, bot, service.OnMemberJoined, joinUpdate(groupID, applicantID, ChatTypeSupergroup, nil))

			service.mu.Lock()
			_, pending := service.pend[pkey{gid: groupID, uid: applicantID}]
			service.mu.Unlock()
			if pending == tc.wantExempt {
				t.Fatalf("pending verification = %v, want %v; invalid trust sources must not exempt an arrival and a later valid source must", pending, !tc.wantExempt)
			}
			if exempt := service.recentlyPassed(groupID, applicantID); exempt != tc.wantExempt {
				t.Errorf("recent trusted pass = %v, want %v", exempt, tc.wantExempt)
			}
		})
	}
}

// Telegram can report that the request vanished while the trusted approval was in flight.
// It is already settled, so starting a normal challenge would only bother an admitted person.
func TestTrustedBypassTreatsVanishedJoinRequestAsSettled(t *testing.T) {
	const (
		groupID     int64 = -1009000000821
		sourceID    int64 = -1009000000822
		applicantID int64 = 821
	)

	service := newTrustedSourceService(groupID, []int64{sourceID})
	gone := newFakeVerifyBot()
	gone.memberByID = map[int64]ChatMember{applicantID: &ChatMemberMember{Status: MemberStatusMember}}
	gone.approveErr = &GatewayError{
		Cause: errors.New("join request disappeared"),
		Kinds: FailureJoinRequestGone,
	}
	if handled, trusted := service.tryTrustedBypass(context.Background(), gone, groupID, applicantID); !handled || !trusted {
		t.Fatalf("vanished join request = handled %v trusted %v, want true true; a settled request must not start a normal challenge", handled, trusted)
	}
	if gone.approves != 1 {
		t.Errorf("vanished request approvals = %d, want 1", gone.approves)
	}

	ordinary := newFakeVerifyBot()
	ordinary.memberByID = map[int64]ChatMember{applicantID: &ChatMemberMember{Status: MemberStatusMember}}
	if handled, trusted := newTrustedSourceService(groupID, []int64{sourceID}).tryTrustedBypass(context.Background(), ordinary, groupID, applicantID); !handled || !trusted {
		t.Fatalf("confirmed trusted member = handled %v trusted %v, want true true", handled, trusted)
	}
}

// A successful trusted admission is an approval in the operator counters and must remain
// remembered long enough to suppress the membership update even if a later trust lookup fails.
func TestTrustedBypassRecordsApprovedAdmission(t *testing.T) {
	const (
		groupID     int64 = -1009000000831
		sourceID    int64 = -1009000000832
		applicantID int64 = 831
	)

	service := newTrustedSourceService(groupID, []int64{sourceID})
	service.botUsername = "bot"
	t.Cleanup(service.stopForShutdown)
	bot := &trustedSourceGateway{
		fakeVerifyBot: newFakeVerifyBot(),
		memberAtChat: map[int64]ChatMember{
			sourceID: &ChatMemberMember{Status: MemberStatusMember},
		},
	}

	if handled, trusted := service.tryTrustedBypass(context.Background(), bot, groupID, applicantID); !handled || !trusted {
		t.Fatalf("trusted admission = handled %v trusted %v, want true true", handled, trusted)
	}
	_, approved, declined := service.Stats()
	if approved != 1 || declined != 0 {
		t.Fatalf("approval counters = approved %d declined %d, want 1 and 0", approved, declined)
	}

	// The join update can arrive after the source lookup becomes unavailable. It is still the
	// same admission, not a reason to start a second challenge.
	bot.memberAtChat[sourceID] = &ChatMemberLeft{Status: MemberStatusLeft}
	runFakeHandler(t, bot, service.OnMemberJoined, joinUpdate(groupID, applicantID, ChatTypeSupergroup, nil))
	service.mu.Lock()
	_, pending := service.pend[pkey{gid: groupID, uid: applicantID}]
	service.mu.Unlock()
	if pending || bot.mutes != 0 {
		t.Errorf("admission update after a trusted bypass started verification: pending=%v mutes=%d", pending, bot.mutes)
	}
}
