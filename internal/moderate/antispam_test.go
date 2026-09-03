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

const (
	blockChannelTestGroupID        = int64(-1009000000601)
	blockChannelTestSenderID       = int64(-1009000000602)
	blockChannelTestSenderArgument = "-1009000000602"
	blockChannelTestMessageID      = 61
)

func newBlockChannelTestService(t *testing.T, antispamEnabled bool, whitelist []int64) (*Service, *fakeModBot) {
	t.Helper()
	whitelist = append([]int64(nil), whitelist...)
	cfg := &settings.Config{
		GroupIDs: []int64{blockChannelTestGroupID},
		Groups: []settings.GroupConfig{{
			ID:               blockChannelTestGroupID,
			AntispamEnabled:  &antispamEnabled,
			ChannelWhitelist: &whitelist,
		}},
		NotifyTTLSeconds: -1,
		Lang:             "en",
	}
	telegram := newFakeMod()
	telegram.member = &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}
	return newTestService(t, cfg, telegram, ""), telegram
}

func runBlockChannel(t *testing.T, service *Service, text string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("%q panicked before it could reply: %v", text, recovered)
		}
	}()
	service.BlockChannel(context.Background(), ChannelSenderCommand{
		ChatID:    blockChannelTestGroupID,
		MessageID: blockChannelTestMessageID,
		CallerID:  7,
		Text:      text,
	})
}

func boolPtr(v bool) *bool { return &v }

func TestBCAllowAndDenyRequireAChannelID(t *testing.T) {
	l := i18n.FromStored("en")
	tests := []struct {
		name                 string
		incompleteCommand    string
		completeCommand      string
		initialWhitelisted   bool
		wantWhitelisted      bool
		wantNotice           string
		wantSenderUnbanCalls int
	}{
		{
			name:                 "allow",
			incompleteCommand:    "/bc allow",
			completeCommand:      "/bc allow " + blockChannelTestSenderArgument,
			wantWhitelisted:      true,
			wantNotice:           i18n.Messages.Moderate.Antispam.Allowed.Render(l, blockChannelTestSenderID),
			wantSenderUnbanCalls: 1,
		},
		{
			name:               "deny",
			incompleteCommand:  "/bc deny",
			completeCommand:    "/bc deny " + blockChannelTestSenderArgument,
			initialWhitelisted: true,
			wantNotice:         i18n.Messages.Moderate.Antispam.Removed.Render(l, blockChannelTestSenderID),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initialWhitelist := []int64(nil)
			if test.initialWhitelisted {
				initialWhitelist = []int64{blockChannelTestSenderID}
			}
			service, telegram := newBlockChannelTestService(t, false, initialWhitelist)
			runBlockChannel(t, service, test.incompleteCommand)

			if got := telegram.lastSendText; got != i18n.Messages.Moderate.Antispam.Usage.For(l) {
				t.Fatalf("%s notice = %q, want usage %q", test.incompleteCommand, got, i18n.Messages.Moderate.Antispam.Usage.For(l))
			}
			if got := service.channelWhitelisted(blockChannelTestGroupID, blockChannelTestSenderID); got != test.initialWhitelisted {
				t.Fatalf("%s changed whitelist membership to %t, want %t", test.incompleteCommand, got, test.initialWhitelisted)
			}

			service, telegram = newBlockChannelTestService(t, false, initialWhitelist)
			runBlockChannel(t, service, test.completeCommand)
			if got := service.channelWhitelisted(blockChannelTestGroupID, blockChannelTestSenderID); got != test.wantWhitelisted {
				t.Fatalf("%s left whitelist membership %t, want %t", test.completeCommand, got, test.wantWhitelisted)
			}
			if got := telegram.lastSendText; got != test.wantNotice {
				t.Fatalf("%s notice = %q, want successful notice %q", test.completeCommand, got, test.wantNotice)
			}
			if got := len(telegram.senderUnbans); got != test.wantSenderUnbanCalls {
				t.Fatalf("%s sender unban calls = %d, want %d", test.completeCommand, got, test.wantSenderUnbanCalls)
			}
		})
	}
}

func TestBCAllowRefusesAnUnparseableChannelID(t *testing.T) {
	const preservedSenderID = int64(-1009000000603)
	service, telegram := newBlockChannelTestService(t, false, []int64{preservedSenderID})

	runBlockChannel(t, service, "/bc allow notanumber")

	group, ok := service.settings.Settings(blockChannelTestGroupID)
	if !ok {
		t.Fatal("invoking group disappeared from settings")
	}
	if got := group.ChannelWhitelist().Value; len(got) != 1 || got[0] != preservedSenderID {
		t.Fatalf("invalid channel ID changed persisted whitelist to %v; want [%d]", got, preservedSenderID)
	}
	if got := len(telegram.senderUnbans); got != 0 {
		t.Fatalf("invalid channel ID triggered %d sender unban calls; want none", got)
	}
	l := i18n.FromStored("en")
	if got := telegram.lastSendText; got != i18n.Messages.Moderate.Antispam.InvalidChannelID.For(l) {
		t.Fatalf("invalid channel ID notice = %q, want %q", got, i18n.Messages.Moderate.Antispam.InvalidChannelID.For(l))
	}

	runBlockChannel(t, service, "/bc allow "+blockChannelTestSenderArgument)
	if !service.channelWhitelisted(blockChannelTestGroupID, blockChannelTestSenderID) {
		t.Fatal("valid channel ID was refused after invalid input")
	}
	if got := len(telegram.senderUnbans); got != 1 {
		t.Fatalf("valid channel ID sender unban calls = %d, want 1", got)
	}
	if got := telegram.lastSendText; got != i18n.Messages.Moderate.Antispam.Allowed.Render(l, blockChannelTestSenderID) {
		t.Fatalf("valid channel ID notice = %q, want successful notice %q", got, i18n.Messages.Moderate.Antispam.Allowed.Render(l, blockChannelTestSenderID))
	}
}

func TestBareBCTogglesThePersistedAntispamSetting(t *testing.T) {
	for _, test := range []struct {
		name    string
		initial bool
	}{
		{name: "disabled to enabled"},
		{name: "enabled to disabled", initial: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, telegram := newBlockChannelTestService(t, test.initial, nil)
			runBlockChannel(t, service, "/bc")

			group, ok := service.settings.Settings(blockChannelTestGroupID)
			if !ok {
				t.Fatal("invoking group disappeared from settings")
			}
			want := !test.initial
			if got := group.AntispamEnabled().Value; got != want {
				t.Fatalf("bare /bc left antispam enabled=%t after starting enabled=%t; want enabled=%t", got, test.initial, want)
			}
			wantNotice := i18n.Messages.Moderate.Antispam.Disabled.For(i18n.FromStored("en"))
			if want {
				wantNotice = i18n.Messages.Moderate.Antispam.Enabled.For(i18n.FromStored("en"))
			}
			if got := telegram.lastSendText; got != wantNotice {
				t.Fatalf("bare /bc notice = %q, want toggled-state notice %q", got, wantNotice)
			}
		})
	}
}

func TestHandledBCCommandIsDeletedFromTheGroup(t *testing.T) {
	service, telegram := newBlockChannelTestService(t, false, nil)

	runBlockChannel(t, service, "/bc")

	if got := telegram.lastSendText; got != i18n.Messages.Moderate.Antispam.Enabled.For(i18n.FromStored("en")) {
		t.Fatalf("command was not handled: notice = %q", got)
	}
	if telegram.deletes != 1 || telegram.lastDeletedChatID != blockChannelTestGroupID ||
		len(telegram.deletedMessageIDs) != 1 || telegram.deletedMessageIDs[0] != blockChannelTestMessageID {
		t.Fatalf("handled /bc command remained visible: deletes=%d, deleted chat=%d, message IDs=%v; want one deletion from chat %d of message %d",
			telegram.deletes, telegram.lastDeletedChatID, telegram.deletedMessageIDs, blockChannelTestGroupID, blockChannelTestMessageID)
	}
}
