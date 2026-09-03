package panel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

type recordedPanelSend struct {
	ChatID      int64  `json:"chat_id"`
	Text        string `json:"text"`
	ReplyMarkup struct {
		ForceReply     bool `json:"force_reply"`
		Selective      bool `json:"selective"`
		ResizeKeyboard bool `json:"resize_keyboard"`
		OneTime        bool `json:"one_time_keyboard"`
		RemoveKeyboard bool `json:"remove_keyboard"`
		Keyboard       [][]struct {
			Text        string                            `json:"text"`
			RequestChat *telego.KeyboardButtonRequestChat `json:"request_chat"`
		} `json:"keyboard"`
	} `json:"reply_markup"`
}

type panelChatResult struct {
	chat *telego.ChatFullInfo
	err  error
}

type panelMemberResult struct {
	member telego.ChatMember
	err    error
}

type panelInputRecordingCaller struct {
	delegate       *panelAPICaller
	sendErr        error
	nilSend        bool
	getMeErr       error
	chats          map[int64]panelChatResult
	members        map[[2]int64]panelMemberResult
	sends          []recordedPanelSend
	deletedMessage []telego.DeleteMessageParams
}

func (c *panelInputRecordingCaller) Call(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	switch method {
	case "sendMessage":
		return c.callSendMessage(ctx, endpoint, data)
	case "deleteMessage":
		if err := c.recordDeleteMessage(data); err != nil {
			return nil, err
		}
	case "getMe":
		if c.getMeErr != nil {
			return nil, c.getMeErr
		}
	case "getChat":
		return c.callGetChat(ctx, endpoint, data)
	case "getChatMember":
		return c.callGetChatMember(ctx, endpoint, data)
	}
	return c.delegate.Call(ctx, endpoint, data)
}

func (c *panelInputRecordingCaller) callSendMessage(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	var request recordedPanelSend
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return nil, err
	}
	c.sends = append(c.sends, request)
	if c.sendErr != nil {
		return nil, c.sendErr
	}
	if c.nilSend {
		return panelAPIResponse(nil)
	}
	return c.delegate.Call(ctx, endpoint, data)
}

func (c *panelInputRecordingCaller) recordDeleteMessage(data *ta.RequestData) error {
	var request telego.DeleteMessageParams
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return err
	}
	c.deletedMessage = append(c.deletedMessage, request)
	return nil
}

func (c *panelInputRecordingCaller) callGetChat(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	var request telego.GetChatParams
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return nil, err
	}
	result, ok := c.chats[request.ChatID.ID]
	if !ok {
		return c.delegate.Call(ctx, endpoint, data)
	}
	if result.err != nil {
		return nil, result.err
	}
	return panelAPIResponse(result.chat)
}

func (c *panelInputRecordingCaller) callGetChatMember(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	var request telego.GetChatMemberParams
	if err := json.Unmarshal(data.BodyRaw, &request); err != nil {
		return nil, err
	}
	result, ok := c.members[[2]int64{request.ChatID.ID, request.UserID}]
	if !ok {
		return c.delegate.Call(ctx, endpoint, data)
	}
	if result.err != nil {
		return nil, result.err
	}
	return panelAPIResponse(result.member)
}

func newPanelInputRecordingTest(t *testing.T) (*Panel, *settings.Store, *panelInputRecordingCaller, *telego.Bot) {
	t.Helper()
	caller := &panelInputRecordingCaller{
		delegate: &panelAPICaller{admin: true, messageID: 100},
		chats:    make(map[int64]panelChatResult),
		members:  make(map[[2]int64]panelMemberResult),
	}
	panel, store, bot := newSettingsPanelTestWithCaller(t, "", caller)
	return panel, store, caller, bot
}

func TestPanelTextPromptUsesSelectiveForceReplyAndReturnedMessageID(t *testing.T) {
	panel, store, caller, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "bd", "_")
	if session.pending == nil || len(caller.sends) != 1 {
		t.Fatalf("text prompt state = %+v, sends = %d", session.pending, len(caller.sends))
	}
	sent := caller.sends[0]
	if !sent.ReplyMarkup.ForceReply || !sent.ReplyMarkup.Selective {
		t.Fatalf("text prompt is not selective ForceReply: %+v", sent.ReplyMarkup)
	}
	if session.pending.promptMessageID != caller.delegate.messageID {
		t.Fatalf("pending prompt ID = %d, Telegram returned %d", session.pending.promptMessageID, caller.delegate.messageID)
	}
	want := panel.inputPrompt(i18n.LangEN, inputBanDuration)
	if sent.ChatID != panelTestUser || sent.Text != want {
		t.Fatalf("text prompt destination/text = %d/%q, want %d/%q", sent.ChatID, sent.Text, panelTestUser, want)
	}
}

func TestPanelPromptsRefuseWhileKernelChallengeIsActive(t *testing.T) {
	for _, chatPicker := range []bool{false, true} {
		name := "text"
		if chatPicker {
			name = "chat picker"
		}
		t.Run(name, func(t *testing.T) {
			panel, store, _, bot := newPanelInputRecordingTest(t)
			panel.verifier.(*panelVerifierStub).kernelPending = true
			session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
			var err error
			if chatPicker {
				err = panel.armChatInput(context.Background(), bot, session, inputKnownChat, "li")
			} else {
				err = panel.armTextInput(context.Background(), bot, session, inputTimeout, "vp")
			}
			var notice *panelNoticeError
			if !errors.As(err, &notice) || session.pending != nil {
				t.Fatalf("active kernel challenge armed %s prompt: pending=%+v err=%v", name, session.pending, err)
			}
			panel.verifier.(*panelVerifierStub).kernelPending = false
			if chatPicker {
				err = panel.armChatInput(context.Background(), bot, session, inputKnownChat, "li")
			} else {
				err = panel.armTextInput(context.Background(), bot, session, inputTimeout, "vp")
			}
			if err != nil || session.pending == nil {
				t.Fatalf("inactive kernel challenge refused %s prompt: pending=%+v err=%v", name, session.pending, err)
			}
		})
	}
}

func TestPanelPromptFailuresClearPendingState(t *testing.T) {
	for _, chatPicker := range []bool{false, true} {
		for _, failure := range []string{"render", "send", "nil response"} {
			name := failure + " text"
			if chatPicker {
				name = failure + " chat picker"
			}
			t.Run(name, func(t *testing.T) {
				panel, store, caller, bot := newPanelInputRecordingTest(t)
				session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
				switch failure {
				case "render":
					caller.delegate.editErr = errors.New("render failed")
				case "send":
					caller.sendErr = errors.New("send failed")
				case "nil response":
					caller.nilSend = true
				}
				err := armTestPanelPrompt(panel, bot, session, chatPicker)
				if err == nil || session.pending != nil {
					t.Fatalf("%s failure left usable prompt state: pending=%+v err=%v", failure, session.pending, err)
				}
			})
		}
	}
	panel, store, _, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
	if err := armTestPanelPrompt(panel, bot, session, false); err != nil || session.pending == nil {
		t.Fatalf("successful prompt arm = pending %+v, err %v", session.pending, err)
	}
}

func armTestPanelPrompt(panel *Panel, bot *telego.Bot, session *panelSession, chatPicker bool) error {
	if chatPicker {
		return panel.armChatInput(context.Background(), bot, session, inputKnownChat, "li")
	}
	return panel.armTextInput(context.Background(), bot, session, inputTimeout, "vp")
}

func TestPanelTextMatcherRequiresTheExactPrivateForceReply(t *testing.T) {
	base := panelTextUpdate(panelTestUser, 71, "300")
	for _, test := range []struct {
		name      string
		update    telego.Update
		configure func(*panelSession)
	}{
		{name: "missing message", update: telego.Update{}},
		{name: "missing sender", update: func() telego.Update {
			update := panelTextUpdate(panelTestUser, 71, "300")
			update.Message.From = nil
			return update
		}()},
		{name: "group chat", update: func() telego.Update {
			update := panelTextUpdate(panelTestUser, 71, "300")
			update.Message.Chat.Type = "supergroup"
			return update
		}()},
		{name: "missing reply", update: func() telego.Update {
			update := panelTextUpdate(panelTestUser, 71, "300")
			update.Message.ReplyToMessage = nil
			return update
		}()},
		{name: "different sender", update: panelTextUpdate(panelTestUser+1, 71, "300")},
		{name: "different prompt", update: panelTextUpdate(panelTestUser, 72, "300")},
		{name: "missing pending", update: panelTextUpdate(panelTestUser, 71, "300"), configure: func(session *panelSession) { session.pending = nil }},
		{name: "zero prompt", update: func() telego.Update {
			update := panelTextUpdate(panelTestUser, 71, "300")
			update.Message.ReplyToMessage.MessageID = 0
			return update
		}(), configure: func(session *panelSession) { session.pending.promptMessageID = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel := &Panel{panelState: newSettingsPanelState()}
			session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
			if err != nil {
				t.Fatal(err)
			}
			session.pending = &pendingInput{kind: inputTimeout, promptMessageID: 71}
			if test.configure != nil {
				test.configure(session)
			}
			if panelInputDMWithoutPanic(t, panel, test.update) {
				t.Fatalf("%s update matched an unrelated ForceReply prompt", test.name)
			}
		})
	}
	panel := &Panel{panelState: newSettingsPanelState()}
	session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
	if err != nil {
		t.Fatal(err)
	}
	session.pending = &pendingInput{kind: inputTimeout, promptMessageID: 71}
	if !panelInputDMWithoutPanic(t, panel, base) {
		t.Fatal("exact private ForceReply did not match its live prompt")
	}
}

func panelInputDMWithoutPanic(t *testing.T, panel *Panel, update telego.Update) (matched bool) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("malformed ForceReply update panicked: %v", recovered)
		}
	}()
	return panel.PanelInputDM(context.Background(), update)
}

func panelTextUpdate(userID int64, replyID int, text string) telego.Update {
	message := &telego.Message{
		MessageID: 1000, Chat: telego.Chat{ID: userID, Type: "private"},
		From: &telego.User{ID: userID, LanguageCode: "en"}, Text: text,
	}
	if replyID != 0 {
		message.ReplyToMessage = &telego.Message{MessageID: replyID}
	}
	return telego.Update{Message: message}
}

func TestPanelCanceledForceReplyMatchesOnceAndCannotReachAnotherHandler(t *testing.T) {
	panel, store, caller, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "bd", "_")
	promptID := session.pending.promptMessageID
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "cn", "_")
	if session.pending != nil || !caller.delegate.replyKeyboardRemoved {
		t.Fatalf("cancel cleanup = pending %+v, keyboard removed %v", session.pending, caller.delegate.replyKeyboardRemoved)
	}
	if !deletedPanelMessage(caller.deletedMessage, panelTestUser, promptID) {
		t.Fatalf("cancel did not delete ForceReply prompt %d: %+v", promptID, caller.deletedMessage)
	}
	if panel.PanelInputDM(context.Background(), panelTextUpdate(panelTestUser+1, promptID, "answer")) {
		t.Fatal("canceled ForceReply tombstone matched a different sender")
	}
	if panel.PanelInputDM(context.Background(), panelTextUpdate(panelTestUser, promptID+1, "answer")) {
		t.Fatal("canceled ForceReply tombstone matched a different prompt")
	}
	update := panelTextUpdate(panelTestUser, promptID, "answer")
	if !panel.PanelInputDM(context.Background(), update) {
		t.Fatal("canceled ForceReply did not match its one-shot tombstone")
	}
	runFakeHandler(t, bot, panel.OnPanelInput, update)
	want := i18n.Messages.Panel.Settings.Error.InputCanceledVerification.For(i18n.LangEN)
	if caller.delegate.lastSendText != want {
		t.Fatalf("canceled ForceReply notice = %q, want %q", caller.delegate.lastSendText, want)
	}
	if panel.PanelInputDM(context.Background(), update) {
		t.Fatal("consumed ForceReply tombstone matched a second time")
	}
}

func deletedPanelMessage(deleted []telego.DeleteMessageParams, chatID int64, messageID int) bool {
	for _, request := range deleted {
		if request.ChatID.ID == chatID && request.MessageID == messageID {
			return true
		}
	}
	return false
}

func TestPanelKernelCancellationStillMatchesOnlyItsForceReply(t *testing.T) {
	newPanel := func() (*Panel, *panelSession) {
		panel := &Panel{verifier: &panelVerifierStub{kernelPending: true}, panelState: newSettingsPanelState()}
		session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
		if err != nil {
			t.Fatal(err)
		}
		session.pending = &pendingInput{kind: inputTimeout, promptMessageID: 71}
		return panel, session
	}
	panel, session := newPanel()
	if panel.PanelInputDM(context.Background(), panelTextUpdate(panelTestUser, 72, "answer")) {
		t.Fatal("kernel activation made an unrelated reply match the panel prompt")
	}
	if session.pending != nil {
		t.Fatal("kernel activation retained the superseded panel prompt")
	}
	panel, _ = newPanel()
	if !panel.PanelInputDM(context.Background(), panelTextUpdate(panelTestUser, 71, "answer")) {
		t.Fatal("kernel activation did not match the superseded prompt itself")
	}
}

func TestPanelBlankTextKeepsPromptAndValidTextCompletesIt(t *testing.T) {
	panel, store, caller, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "vp")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "to", "_")
	before, _ := store.Settings(panelTestGroupA)
	submitPanelText(t, panel, bot, session, "   ")
	after, _ := store.Settings(panelTestGroupA)
	if session.pending == nil || after.Revision() != before.Revision() {
		t.Fatalf("blank input changed state or discarded prompt: revision=%d pending=%+v", after.Revision(), session.pending)
	}
	want := i18n.Messages.Panel.Settings.Error.InvalidInput.For(i18n.LangEN)
	if caller.delegate.lastSendText != want {
		t.Fatalf("blank input notice = %q, want %q", caller.delegate.lastSendText, want)
	}
	promptID := session.pending.promptMessageID
	submitPanelText(t, panel, bot, session, "300")
	after, _ = store.Settings(panelTestGroupA)
	if session.pending != nil || after.TimeoutSeconds().Value != 300 {
		t.Fatalf("valid correction = timeout %d pending %+v", after.TimeoutSeconds().Value, session.pending)
	}
	if !caller.delegate.replyKeyboardRemoved || !deletedPanelMessage(caller.deletedMessage, panelTestUser, promptID) {
		t.Fatalf("successful input did not remove keyboard and prompt: removed=%v deleted=%+v", caller.delegate.replyKeyboardRemoved, caller.deletedMessage)
	}
}

func TestPanelTextInputRechecksAdministratorAndAcceptsCurrentAdmin(t *testing.T) {
	for _, test := range []struct {
		name   string
		result panelMemberResult
	}{
		{name: "lookup error", result: panelMemberResult{err: errors.New("membership failed")}},
		{name: "demoted", result: panelMemberResult{member: &telego.ChatMemberMember{Status: telego.MemberStatusMember}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			panel, store, caller, bot := newPanelInputRecordingTest(t)
			session := addPanelSession(t, panel, store, panelTestGroupA, "in")
			session.pending = &pendingInput{kind: inputTimeout, parent: "vp", promptMessageID: 71, expectedRevision: session.revision}
			caller.members[[2]int64{panelTestGroupA, panelTestUser}] = test.result
			before, _ := store.Settings(panelTestGroupA)
			runFakeHandler(t, bot, panel.OnPanelInput, panelTextUpdate(panelTestUser, 71, "300"))
			after, _ := store.Settings(panelTestGroupA)
			if after.Revision() != before.Revision() || after.TimeoutSeconds().Value != before.TimeoutSeconds().Value {
				t.Fatalf("%s authorization failure changed timeout", test.name)
			}
			if test.name == "lookup error" {
				want := i18n.Messages.Panel.Settings.Error.AuthorizationCheckFailed.For(i18n.LangEN)
				if caller.delegate.lastSendText != want || panel.sessionByUser(panelTestUser) == nil {
					t.Fatalf("authorization lookup error notice/session = %q/%v, want %q/retained", caller.delegate.lastSendText, panel.sessionByUser(panelTestUser) != nil, want)
				}
			} else {
				want := i18n.Messages.Panel.Settings.Error.AuthorizationLost.For(i18n.LangEN)
				if caller.delegate.lastEditText != want || panel.sessionByUser(panelTestUser) != nil {
					t.Fatalf("demotion notice/session = %q/%v, want %q/removed", caller.delegate.lastEditText, panel.sessionByUser(panelTestUser) != nil, want)
				}
			}
		})
	}
	panel, store, _, bot := newPanelInputRecordingTest(t)
	session := addPanelSession(t, panel, store, panelTestGroupA, "in")
	session.pending = &pendingInput{kind: inputTimeout, parent: "vp", promptMessageID: 71, expectedRevision: session.revision}
	runFakeHandler(t, bot, panel.OnPanelInput, panelTextUpdate(panelTestUser, 71, "300"))
	group, _ := store.Settings(panelTestGroupA)
	if group.TimeoutSeconds().Value != 300 {
		t.Fatalf("current administrator could not set timeout: %d", group.TimeoutSeconds().Value)
	}
}

func TestPanelInputMatchersRejectASessionReplacedAfterLookup(t *testing.T) {
	for _, chatPicker := range []bool{false, true} {
		name := "ForceReply"
		if chatPicker {
			name = "chat picker"
		}
		t.Run(name, func(t *testing.T) {
			panel := &Panel{panelState: newSettingsPanelState()}
			session, err := panel.newSettingsSession(panelTestUser, panelTestGroupA, i18n.LangEN)
			if err != nil {
				t.Fatal(err)
			}
			session.pending = &pendingInput{kind: inputKnownChat, promptMessageID: 71, requestID: 7}
			session.mu.Lock()
			marker := promptKey{userID: -1, messageID: -1}
			panel.panelState.mu.Lock()
			panel.panelState.tombstones[marker] = inputTombstone{expiresAt: time.Now().Add(-time.Second)}
			panel.panelState.mu.Unlock()
			matched := make(chan bool, 1)
			go func() {
				if chatPicker {
					matched <- panel.PanelChatSharedDM(context.Background(), panelChatUpdate(panelTestUser, 7, -1009000000929))
					return
				}
				matched <- panel.PanelInputDM(context.Background(), panelTextUpdate(panelTestUser, 71, "answer"))
			}()
			waitForExpiredTombstonePrune(t, panel, marker)
			replacement := &panelSession{ownerID: panelTestUser, expiresAt: time.Now().Add(time.Hour)}
			panel.panelState.mu.Lock()
			panel.panelState.byUser[panelTestUser] = replacement
			panel.panelState.mu.Unlock()
			session.mu.Unlock()
			if <-matched {
				t.Fatalf("%s matched a prompt owned by the replaced session", name)
			}
		})
	}
}
