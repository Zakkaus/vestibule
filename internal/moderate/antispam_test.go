package moderate

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestBCAllowUpdatesOnlyInvokingGroup(t *testing.T) {
	const senderID = int64(-1009999900006)
	groups := []int64{-100, -200, -300}
	for _, test := range []struct {
		name      string
		lang      string
		failUnban bool
	}{
		{name: "unban succeeds"},
		{name: "unban failure is reported", failUnban: true},
		{name: "English notice", lang: "en"},
		{name: "Traditional Chinese failure notice", lang: "zh-Hant", failUnban: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := &settings.Config{
				GroupIDs:         groups,
				Groups:           []settings.GroupConfig{{ID: -100}, {ID: -200}, {ID: -300}},
				NotifyTTLSeconds: -1,
				Lang:             test.lang,
			}
			telegram := newFakeMod()
			telegram.member = &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}
			if test.failUnban {
				telegram.senderUnbanErr = map[int64]error{-200: errors.New("no rights")}
			}
			service := newTestService(t, cfg, telegram, "")
			service.BlockChannel(context.Background(), ChannelSenderCommand{
				ChatID:    -200,
				MessageID: 1,
				CallerID:  7,
				Text:      "/bc allow 9999900006",
			})

			if !service.channelWhitelisted(-200, senderID) {
				t.Fatal("successful /bc allow did not update the invoking group")
			}
			if service.channelWhitelisted(-100, senderID) || service.channelWhitelisted(-300, senderID) {
				t.Fatal("/bc allow changed another group")
			}
			if len(telegram.senderUnbans) != 1 {
				t.Fatalf("sender unbans = %d, want 1", len(telegram.senderUnbans))
			}
			call := telegram.senderUnbans[0]
			if call.ChatID.ID != -200 || call.SenderChatID != senderID {
				t.Errorf("unban call = chat %d sender %d, want chat -200 sender %d", call.ChatID.ID, call.SenderChatID, senderID)
			}
			l := i18n.FromStored(test.lang)
			wantNotice := i18n.Messages.Moderate.Antispam.Allowed.Render(l, senderID)
			if test.failUnban {
				wantNotice = i18n.Messages.Moderate.Antispam.AllowedUnbanFailed.Render(l, senderID)
			}
			if telegram.lastSendText != wantNotice {
				t.Errorf("notice = %q, want %q", telegram.lastSendText, wantNotice)
			}
		})
	}
}

func TestBCDenyRemovesWhitelistWithoutUnbanning(t *testing.T) {
	const (
		groupID  int64 = -1009000000503
		senderID int64 = -1009000000504
	)
	telegram := newFakeMod()
	telegram.member = &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}
	service := newTestService(t, &settings.Config{
		GroupIDs: []int64{groupID},
		Groups: []settings.GroupConfig{{
			ID:               groupID,
			ChannelWhitelist: &[]int64{senderID},
		}},
		NotifyTTLSeconds: -1,
		Lang:             "en",
	}, telegram, "")

	service.BlockChannel(context.Background(), ChannelSenderCommand{
		ChatID:    groupID,
		MessageID: 1,
		CallerID:  7,
		Text:      "/bc deny 9000000504",
	})

	if service.channelWhitelisted(groupID, senderID) {
		t.Fatal("/bc deny left the channel whitelisted; later adverts would be treated as trusted")
	}
	if len(telegram.senderUnbans) != 0 {
		t.Fatalf("/bc deny called UnbanSenderChat %d times; it would lift the sender-chat ban used to block that advertiser", len(telegram.senderUnbans))
	}
	l := service.groupLanguage(groupID)
	assertModerationNotifications(t, telegram, fakeModNotification{
		chatID: groupID,
		text:   i18n.Messages.Moderate.Antispam.Removed.Render(l, senderID),
	})
}

func TestFilterChannelSenderUsesTelegramTransport(t *testing.T) {
	const (
		groupID  int64 = -100
		senderID int64 = -1009999900006
	)
	cfg := &settings.Config{
		GroupIDs:            []int64{groupID},
		Groups:              []settings.GroupConfig{{ID: groupID}},
		BlockChannelSenders: boolPtr(true),
		AdminLogChatID:      -200,
		Lang:                "zh",
	}
	telegram := newFakeMod()
	service := newTestService(t, cfg, telegram, "")
	if !service.FilterChannelSender(context.Background(), ChannelSenderMessage{
		ChatID:          groupID,
		MessageID:       3,
		SenderChatID:    senderID,
		SenderChatTitle: "Spam Channel",
	}) {
		t.Fatal("channel sender filter did not consume the spam post")
	}
	if telegram.deletes != 1 || telegram.senderBans != 1 {
		t.Fatalf("filter actions = deletes %d, sender bans %d", telegram.deletes, telegram.senderBans)
	}
	wantAlert := i18n.Messages.Moderate.Antispam.SenderBannedAlert.Render(i18n.LangZH, "Spam Channel", senderID, groupID, senderID)
	if telegram.lastSendChat != cfg.AdminLogChatID || telegram.lastSendText != wantAlert {
		t.Fatalf("operator alert = chat %d text %q, want chat %d text %q", telegram.lastSendChat, telegram.lastSendText, cfg.AdminLogChatID, wantAlert)
	}
}

func boolPtr(v bool) *bool { return &v }
