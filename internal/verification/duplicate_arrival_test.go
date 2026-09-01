package verification

import (
	"context"
	"github.com/Zakkaus/vestibule/internal/settings"
	"testing"
)

// Telegram redelivered eight join requests in one day, each about five seconds after the first.
// Each repeat posted a second challenge and left the first to be deleted, so a delete that failed
// left an orphan challenge in the group that nobody could answer.
func TestRedeliveredJoinRequestKeepsTheChallengeOnScreen(t *testing.T) {
	const (
		groupID int64 = -1009000000920
		userID  int64 = 920
	)
	v := newTestService(&settings.Config{
		Groups:         []settings.GroupConfig{{ID: groupID}},
		GroupIDs:       []int64{groupID},
		Lang:           "en",
		VerifyMode:     settings.ModeKernel,
		TimeoutSeconds: 240,
		DeliveryMode:   settings.DeliveryBoth,
	})
	bot := newFakeVerifyBot()
	update := Update{ChatJoinRequest: &ChatJoinRequest{
		Chat: Chat{ID: groupID},
		From: User{ID: userID, FirstName: "Applicant"},
	}}

	runFakeHandler(t, newAPITestBot(t, bot), v.OnJoinRequest, update)
	firstSends, firstDeletes := bot.sends, bot.deletes
	if firstSends == 0 {
		t.Fatal("the first join request posted no challenge")
	}

	runFakeHandler(t, newAPITestBot(t, bot), v.OnJoinRequest, update)
	if bot.sends != firstSends {
		t.Errorf("a redelivered request posted %d more message(s); the challenge already on screen stands",
			bot.sends-firstSends)
	}
	if bot.deletes != firstDeletes {
		t.Errorf("a redelivered request deleted %d message(s); nothing needed replacing",
			bot.deletes-firstDeletes)
	}
}

// A new challenge may open only after the previous row has left pending state.
func TestReapplicationAfterSettlementStartsFreshChallenge(t *testing.T) {
	const (
		groupID int64 = -1009000000921
		userID  int64 = 921
	)
	v := newTestService(&settings.Config{
		Groups:         []settings.GroupConfig{{ID: groupID}},
		GroupIDs:       []int64{groupID},
		Lang:           "en",
		VerifyMode:     settings.ModeKernel,
		TimeoutSeconds: 240,
		DeliveryMode:   settings.DeliveryBoth,
	})
	bot := newFakeVerifyBot()
	update := Update{ChatJoinRequest: &ChatJoinRequest{
		Chat: Chat{ID: groupID},
		From: User{ID: userID, FirstName: "Applicant"},
	}}

	runFakeHandler(t, newAPITestBot(t, bot), v.OnJoinRequest, update)
	if !v.approve(context.Background(), bot, groupID, userID) {
		t.Fatal("first challenge did not settle")
	}
	sendsBeforeReapply := bot.sends

	runFakeHandler(t, newAPITestBot(t, bot), v.OnJoinRequest, update)
	if bot.sends <= sendsBeforeReapply {
		t.Error("a re-application after settlement posted no fresh challenge")
	}
}
