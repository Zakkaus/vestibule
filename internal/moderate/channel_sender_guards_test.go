package moderate

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// FilterChannelSender deletes a post and bans the channel that sent it. Everything before that
// is a reason not to: the feature being switched off, an anonymous administrator posting as the
// group, Telegram's own forward of a linked channel, a chat this bot already knows, and a
// channel an operator allowed by hand. Each was removable with every test in the repository
// still passing, in a code path whose only job is to punish.
func TestFilterChannelSenderLeavesLegitimatePostersAlone(t *testing.T) {
	const (
		groupID     int64 = -1009000000701
		senderID    int64 = -1009000000702
		knownID     int64 = -1009000000703
		whitelisted int64 = -1009000000704
	)
	for _, tc := range []struct {
		name     string
		antispam bool
		message  ChannelSenderMessage
	}{
		{
			name:     "the group turned antispam off",
			antispam: false,
			message:  ChannelSenderMessage{ChatID: groupID, MessageID: 3, SenderChatID: senderID},
		},
		{
			// Held twice over: the explicit comparison, and the group counting as a chat this
			// bot knows. Removing either alone leaves this green; removing both has the group
			// delete its own administrator's post and ban itself.
			name:     "an anonymous administrator posting as the group itself",
			antispam: true,
			message:  ChannelSenderMessage{ChatID: groupID, MessageID: 3, SenderChatID: groupID},
		},
		{
			name:     "Telegram forwarding a linked channel's post",
			antispam: true,
			message: ChannelSenderMessage{ChatID: groupID, MessageID: 3, SenderChatID: senderID,
				AutomaticForward: true},
		},
		{
			name:     "a chat this bot already knows",
			antispam: true,
			message:  ChannelSenderMessage{ChatID: groupID, MessageID: 3, SenderChatID: knownID},
		},
		{
			name:     "a channel an operator allowed by hand",
			antispam: true,
			message:  ChannelSenderMessage{ChatID: groupID, MessageID: 3, SenderChatID: whitelisted},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &settings.Config{
				GroupIDs: []int64{groupID},
				Groups: []settings.GroupConfig{{
					ID:               groupID,
					ChannelWhitelist: &[]int64{whitelisted},
				}},
				BlockChannelSenders: boolPtr(tc.antispam),
				KnownChatIDs:        []int64{knownID},
				AdminLogChatID:      -1009000000705,
				Lang:                "zh",
			}
			telegram := newFakeMod()
			service := newTestService(t, cfg, telegram, "")

			if service.FilterChannelSender(context.Background(), tc.message) {
				t.Error("the update was consumed; a legitimate post must keep travelling")
			}
			if telegram.deletes != 0 || telegram.senderBans != 0 {
				t.Errorf("deletes = %d, sender bans = %d, want none: this post had a reason "+
					"not to be punished", telegram.deletes, telegram.senderBans)
			}
		})
	}
}
