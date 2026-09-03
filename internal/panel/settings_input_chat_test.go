package panel

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestPanelChatPickersRequestTheExactKindsMetadataAndIdentifiers(t *testing.T) {
	tests := []struct {
		name         string
		kind         inputKind
		wantChannels []bool
	}{
		{name: "channel whitelist", kind: inputChannelWhitelist, wantChannels: []bool{true}},
		{name: "trusted group", kind: inputTrustedGroup, wantChannels: []bool{false}},
		{name: "known chat", kind: inputKnownChat, wantChannels: []bool{false, true}},
		{name: "required channel", kind: inputRequiredChannel, wantChannels: []bool{true}},
		{name: "alert chat", kind: inputAlertChat, wantChannels: []bool{false, true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panel, store, caller, bot := newPanelInputRecordingTest(t)
			session := addPanelSession(t, panel, store, panelTestGroupA, "li")
			if err := panel.armChatInput(context.Background(), bot, session, test.kind, "li"); err != nil {
				t.Fatal(err)
			}
			if len(caller.sends) != 1 || session.pending == nil {
				t.Fatalf("picker sends=%d pending=%+v", len(caller.sends), session.pending)
			}
			if session.pending.promptMessageID != caller.delegate.messageID {
				t.Fatalf("pending picker prompt ID = %d, Telegram returned %d", session.pending.promptMessageID, caller.delegate.messageID)
			}
			assertPanelPickerMarkup(t, caller.sends[0], session.pending, test.wantChannels)
		})
	}
}

func assertPanelPickerMarkup(t *testing.T, sent recordedPanelSend, pending *pendingInput, wantChannels []bool) {
	t.Helper()
	markup := sent.ReplyMarkup
	if !markup.ResizeKeyboard || !markup.OneTime || !markup.Selective {
		t.Fatalf("chat picker keyboard flags = resize %v one-time %v selective %v", markup.ResizeKeyboard, markup.OneTime, markup.Selective)
	}
	if len(markup.Keyboard) != 1 || len(markup.Keyboard[0]) != len(wantChannels) {
		t.Fatalf("chat picker buttons = %+v, want %d", markup.Keyboard, len(wantChannels))
	}
	for index, wantChannel := range wantChannels {
		wantID := pending.requestID
		if index == 1 {
			wantID = pending.requestAltID
		}
		assertPanelPickerButton(t, markup.Keyboard[0][index].RequestChat, index, wantChannel, wantID)
	}
	if len(wantChannels) == 1 && pending.requestAltID != 0 {
		t.Fatalf("single-choice picker retained alternate request ID %d", pending.requestAltID)
	}
	if len(wantChannels) == 2 && pending.requestAltID == 0 {
		t.Fatal("two-choice picker omitted alternate request ID")
	}
}

func assertPanelPickerButton(t *testing.T, request *telego.KeyboardButtonRequestChat, index int, wantChannel bool, wantID int32) {
	t.Helper()
	if request == nil || request.ChatIsChannel != wantChannel {
		t.Fatalf("picker button %d channel restriction = %+v, want %v", index, request, wantChannel)
	}
	if request.RequestTitle == nil || !*request.RequestTitle || request.RequestUsername == nil || !*request.RequestUsername ||
		request.BotIsMember == nil || !*request.BotIsMember {
		t.Fatalf("picker button %d omitted requested title, username, or bot membership: %+v", index, request)
	}
	if request.RequestID != wantID {
		t.Fatalf("picker button %d request ID = %d, pending ID %d", index, request.RequestID, wantID)
	}
}

func TestPanelChatMatcherRequiresExactPrivateActiveRequest(t *testing.T) {
	base := panelChatUpdate(panelTestUser, 7, -1009000000921)
	for _, test := range []struct {
		name      string
		update    telego.Update
		configure func(*panelSession)
	}{
		{name: "missing message", update: telego.Update{}},
		{name: "missing sender", update: mutatePanelChatUpdate(base, func(message *telego.Message) { message.From = nil })},
		{name: "group chat", update: mutatePanelChatUpdate(base, func(message *telego.Message) { message.Chat.Type = "supergroup" })},
		{name: "missing shared payload", update: mutatePanelChatUpdate(base, func(message *telego.Message) { message.ChatShared = nil })},
		{name: "different sender", update: panelChatUpdate(panelTestUser+1, 7, -1009000000921)},
		{name: "wrong request", update: panelChatUpdate(panelTestUser, 9, -1009000000921)},
		{name: "below int32", update: panelChatUpdate(panelTestUser, int(math.MinInt32)-1, -1009000000921), configure: func(session *panelSession) { session.pending.requestAltID = 0 }},
		{name: "above int32", update: panelChatUpdate(panelTestUser, int(math.MaxInt32)+1, -1009000000921), configure: func(session *panelSession) { session.pending.requestAltID = 0 }},
		{name: "missing pending", update: base, configure: func(session *panelSession) { session.pending = nil }},
		{name: "zero primary", update: panelChatUpdate(panelTestUser, 8, -1009000000921), configure: func(session *panelSession) { session.pending.requestID = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel, session := panelChatMatcherFixture(t)
			if test.configure != nil {
				test.configure(session)
			}
			if panelChatSharedDMWithoutPanic(t, panel, test.update) {
				t.Fatalf("%s update matched an unrelated chat picker", test.name)
			}
		})
	}
	panel, _ := panelChatMatcherFixture(t)
	if !panelChatSharedDMWithoutPanic(t, panel, base) {
		t.Fatal("primary request ID did not match active chat picker")
	}
	if !panelChatSharedDMWithoutPanic(t, panel, panelChatUpdate(panelTestUser, 8, -1009000000921)) {
		t.Fatal("alternate request ID did not match active chat picker")
	}
}

func panelChatMatcherFixture(t *testing.T) (*Panel, *panelSession) {
	t.Helper()
	panel := &Panel{panelState: newSettingsPanelState()}
	session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	session.pending = &pendingInput{kind: inputKnownChat, requestID: 7, requestAltID: 8}
	return panel, session
}

func panelChatUpdate(userID int64, requestID int, chatID int64) telego.Update {
	return telego.Update{Message: &telego.Message{
		MessageID: 1000, Chat: telego.Chat{ID: userID, Type: "private"},
		From:       &telego.User{ID: userID, LanguageCode: "en"},
		ChatShared: &telego.ChatShared{RequestID: requestID, ChatID: chatID},
	}}
}

func mutatePanelChatUpdate(update telego.Update, mutate func(*telego.Message)) telego.Update {
	message := *update.Message
	if update.Message.From != nil {
		from := *update.Message.From
		message.From = &from
	}
	if update.Message.ChatShared != nil {
		shared := *update.Message.ChatShared
		message.ChatShared = &shared
	}
	update.Message = &message
	mutate(update.Message)
	return update
}

func panelChatSharedDMWithoutPanic(t *testing.T, panel *Panel, update telego.Update) (matched bool) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("malformed chat-picker update panicked: %v", recovered)
		}
	}()
	return panel.PanelChatSharedDM(context.Background(), update)
}

func TestPanelChatHandlerAlsoRejectsWrongRequestAndAcceptsExactRequest(t *testing.T) {
	const chatID int64 = -1009000000922
	panel, store, _, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "in")
	session.pending = &pendingInput{kind: inputKnownChat, parent: "li", promptMessageID: 71, requestID: 7, expectedRevision: session.revision}
	before, _ := store.Settings(panelTestGroupA)
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 9, chatID))
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != before.Revision() || len(after.KnownChatIDs().Value) != 0 {
		t.Fatal("wrong request ID reached chat-picker mutation")
	}

	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
	after, _ = store.Settings(panelTestGroupA)
	if !reflect.DeepEqual(after.KnownChatIDs().Value, []int64{chatID}) {
		t.Fatalf("exact request ID did not add known chat: %v", after.KnownChatIDs().Value)
	}
}

func TestPanelSharedChatRequiresLookupAndCurrentUserMembership(t *testing.T) {
	const chatID int64 = -1009000000923
	tests := []struct {
		name   string
		chat   panelChatResult
		member panelMemberResult
	}{
		{name: "chat lookup error", chat: panelChatResult{err: errors.New("get chat failed")}},
		{name: "missing chat", chat: panelChatResult{}},
		{name: "membership lookup error", chat: validPanelChat(chatID), member: panelMemberResult{err: errors.New("get member failed")}},
		{name: "missing member", chat: validPanelChat(chatID), member: panelMemberResult{}},
		{name: "departed user", chat: validPanelChat(chatID), member: panelMemberResult{member: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft}}},
		{name: "banned user", chat: validPanelChat(chatID), member: panelMemberResult{member: &telego.ChatMemberBanned{Status: telego.MemberStatusBanned}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panel, store, caller, bot := newPanelInputRecordingTest(t)
			session := pendingPanelChatSession(t, panel, store, inputKnownChat, 7)
			caller.chats[chatID] = test.chat
			if test.name != "chat lookup error" && test.name != "missing chat" {
				caller.members[[2]int64{chatID, panelTestUser}] = test.member
			}
			if _, valid := panel.validateSharedChat(context.Background(), bot, session, inputKnownChat, chatID); valid {
				t.Fatalf("%s passed shared-chat validation", test.name)
			}
			runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
			group, _ := store.Settings(panelTestGroupA)
			if len(group.KnownChatIDs().Value) != 0 || session.pending == nil {
				t.Fatalf("%s accepted chat or discarded picker: values=%v pending=%+v", test.name, group.KnownChatIDs().Value, session.pending)
			}
			want := i18n.Messages.Panel.Settings.Error.InvalidChat.For(i18n.LangEN)
			if caller.delegate.lastSendText != want {
				t.Fatalf("%s notice = %q, want %q", test.name, caller.delegate.lastSendText, want)
			}
		})
	}
	assertValidKnownChatSelection(t, chatID)
}

func validPanelChat(chatID int64) panelChatResult {
	return panelChatResult{chat: &telego.ChatFullInfo{ID: chatID, Type: "supergroup", Title: "Selected chat"}}
}

func pendingPanelChatSession(t *testing.T, panel *Panel, store *settings.Store, kind inputKind, requestID int32) *panelSession {
	t.Helper()
	session := addPanelSession(t, panel, store, panelTestGroupA, "in")
	session.pending = &pendingInput{
		kind: kind, parent: "li", promptMessageID: 71, requestID: requestID, expectedRevision: session.revision,
	}
	return session
}

func assertValidKnownChatSelection(t *testing.T, chatID int64) {
	t.Helper()
	panel, store, _, bot := newPanelInputRecordingTest(t)
	pendingPanelChatSession(t, panel, store, inputKnownChat, 7)
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
	group, _ := store.Settings(panelTestGroupA)
	if !reflect.DeepEqual(group.KnownChatIDs().Value, []int64{chatID}) {
		t.Fatalf("valid member chat was refused: %v", group.KnownChatIDs().Value)
	}
}

func TestPanelRequiredChannelRequiresBotIdentityAndMembership(t *testing.T) {
	const chatID int64 = -1009000000924
	tests := []struct {
		name      string
		getMeErr  error
		botMember panelMemberResult
	}{
		{name: "identity lookup error", getMeErr: errors.New("get me failed")},
		{name: "membership lookup error", botMember: panelMemberResult{err: errors.New("get member failed")}},
		{name: "missing member", botMember: panelMemberResult{}},
		{name: "departed bot", botMember: panelMemberResult{member: &telego.ChatMemberLeft{Status: telego.MemberStatusLeft}}},
		{name: "banned bot", botMember: panelMemberResult{member: &telego.ChatMemberBanned{Status: telego.MemberStatusBanned}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			panel, store, caller, bot := newPanelInputRecordingTest(t)
			session := pendingPanelChatSession(t, panel, store, inputRequiredChannel, 7)
			caller.getMeErr = test.getMeErr
			caller.chats[chatID] = panelChatResult{chat: &telego.ChatFullInfo{ID: chatID, Type: "channel", Title: "Channel", Username: "channel"}}
			if test.getMeErr == nil {
				caller.members[[2]int64{chatID, 500}] = test.botMember
			}
			if _, valid := panel.validateSharedChat(context.Background(), bot, session, inputRequiredChannel, chatID); valid {
				t.Fatalf("%s passed required-channel validation", test.name)
			}
			runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
			group, _ := store.Settings(panelTestGroupA)
			if group.RequiredChannelID().Value != 0 || session.pending == nil {
				t.Fatalf("%s accepted required channel or discarded picker", test.name)
			}
		})
	}
	assertValidRequiredChannel(t, chatID)
}

func assertValidRequiredChannel(t *testing.T, chatID int64) {
	t.Helper()
	panel, store, caller, bot := newPanelInputRecordingTest(t)
	pendingPanelChatSession(t, panel, store, inputRequiredChannel, 7)
	caller.chats[chatID] = panelChatResult{chat: &telego.ChatFullInfo{ID: chatID, Type: "channel", Title: "Channel", Username: "channel"}}
	caller.members[[2]int64{chatID, 500}] = panelMemberResult{member: &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator}}
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
	group, _ := store.Settings(panelTestGroupA)
	if group.RequiredChannelID().Value != chatID {
		t.Fatalf("valid required channel was refused: %d", group.RequiredChannelID().Value)
	}
}

func TestPanelSharedChatRechecksAuthorizationAndKernelState(t *testing.T) {
	const chatID int64 = -1009000000925
	for _, test := range []struct {
		name   string
		member panelMemberResult
		kernel bool
	}{
		{name: "lookup error", member: panelMemberResult{err: errors.New("authorization failed")}},
		{name: "demoted", member: panelMemberResult{member: &telego.ChatMemberMember{Status: telego.MemberStatusMember}}},
		{name: "kernel active", kernel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel, store, caller, bot := newPanelInputRecordingTest(t)
			session := pendingPanelChatSession(t, panel, store, inputKnownChat, 7)
			if test.kernel {
				panel.verifier.(*panelVerifierStub).kernelPending = true
			} else {
				caller.members[[2]int64{panelTestGroupA, panelTestUser}] = test.member
			}
			runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
			group, _ := store.Settings(panelTestGroupA)
			if len(group.KnownChatIDs().Value) != 0 {
				t.Fatalf("%s allowed chat-picker mutation", test.name)
			}
			if test.kernel && session.pending != nil {
				t.Fatal("kernel activation retained chat picker")
			}
		})
	}
	assertValidKnownChatSelection(t, chatID)
}

func TestPanelSharedChatAppliesAlertRejectsSelfAndKeepsListsUnique(t *testing.T) {
	const (
		alertID = int64(-1009000000926)
		knownID = int64(-1009000000927)
	)
	panel, store, _, bot := newPanelInputRecordingTest(t)
	pendingPanelChatSession(t, panel, store, inputAlertChat, 7)
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, alertID))
	group, _ := store.Settings(panelTestGroupA)
	if group.AdminLogChatID().Value != alertID {
		t.Fatalf("alert destination = %d, want %d", group.AdminLogChatID().Value, alertID)
	}

	panel, store, caller, bot := newPanelInputRecordingTest(t)
	session := pendingPanelChatSession(t, panel, store, inputTrustedGroup, 7)
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, panelTestGroupA))
	group, _ = store.Settings(panelTestGroupA)
	if len(group.TrustedMemberGroupIDs().Value) != 0 || session.pending == nil {
		t.Fatalf("current group entered trusted list or picker was lost: %v", group.TrustedMemberGroupIDs().Value)
	}
	want := i18n.Messages.Panel.Settings.Error.InvalidChat.For(i18n.LangEN)
	if caller.delegate.lastSendText != want {
		t.Fatalf("self-trust notice = %q, want %q", caller.delegate.lastSendText, want)
	}

	assertValidTrustedGroupSelection(t, -1009000000928)

	assertPanelDuplicateKnownChat(t, knownID)
}

func assertPanelDuplicateKnownChat(t *testing.T, chatID int64) {
	t.Helper()
	panel, store, _, bot := newPanelInputRecordingTest(t)
	group, _ := store.Settings(panelTestGroupA)
	known := []int64{chatID}
	next := group.Overrides()
	next.KnownChatIDs = &known
	result, err := store.Update(panelTestGroupA, group.Revision(), next)
	if err != nil {
		t.Fatal(err)
	}
	session := pendingPanelChatSession(t, panel, store, inputKnownChat, 7)
	session.listKind = inputKnownChat
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
	group, _ = store.Settings(panelTestGroupA)
	if group.Revision() != result.Revision || !reflect.DeepEqual(group.KnownChatIDs().Value, known) {
		t.Fatalf("duplicate known chat changed list/revision: revision %d values %v", group.Revision(), group.KnownChatIDs().Value)
	}
	if panel.sessionByUser(panelTestUser) != session {
		t.Fatal("duplicate known chat ended the settings session instead of acting idempotently")
	}
}

func assertValidTrustedGroupSelection(t *testing.T, chatID int64) {
	t.Helper()
	panel, store, _, bot := newPanelInputRecordingTest(t)
	pendingPanelChatSession(t, panel, store, inputTrustedGroup, 7)
	runFakeHandler(t, bot, panel.OnPanelChatShared, panelChatUpdate(panelTestUser, 7, chatID))
	group, _ := store.Settings(panelTestGroupA)
	if !reflect.DeepEqual(group.TrustedMemberGroupIDs().Value, []int64{chatID}) {
		t.Fatalf("valid trusted group was refused: %v", group.TrustedMemberGroupIDs().Value)
	}
}

func TestPanelSharedRequestIdentifiersStayInsideSignedThirtyTwoBits(t *testing.T) {
	for _, value := range []int{math.MinInt32, 0, math.MaxInt32} {
		got, ok := sharedRequestID(value)
		if !ok || got != int32(value) {
			t.Errorf("in-range request ID %d = %d, %v", value, got, ok)
		}
	}
	for _, value := range []int{int(math.MinInt32) - 1, int(math.MaxInt32) + 1} {
		if got, ok := sharedRequestID(value); ok {
			t.Errorf("out-of-range request ID %d was narrowed to %d", value, got)
		}
	}
}
