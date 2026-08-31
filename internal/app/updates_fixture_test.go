package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

func apiResponse(value any) (*ta.Response, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &ta.Response{Ok: true, Result: raw}, nil
}

func testBot(t *testing.T, caller ta.Caller) *telego.Bot {
	t.Helper()
	bot, err := telego.NewBot(
		"1:"+strings.Repeat("a", 35),
		telego.WithAPICaller(caller),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return bot
}

type dispatchAPICall struct {
	method    string
	body      []byte
	messageID int
}

type dispatchCaller struct {
	mu            sync.Mutex
	botID         int64
	nextMessageID int
	members       map[[2]int64]telego.ChatMember
	calls         []dispatchAPICall
	commandMenus  chan struct{}
}

func (c *dispatchCaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	method := endpoint[strings.LastIndexByte(endpoint, '/')+1:]
	body := append([]byte(nil), data.BodyRaw...)
	switch method {
	case "getMe":
		c.record(dispatchAPICall{method: method, body: body})
		return apiResponse(&telego.User{ID: c.botID, IsBot: true, Username: "dispatch_bot"})
	case "getChat":
		var params struct {
			ChatID int64 `json:"chat_id"`
		}
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		c.record(dispatchAPICall{method: method, body: body})
		return apiResponse(&telego.ChatFullInfo{ID: params.ChatID, Type: telego.ChatTypeSupergroup})
	case "getChatMember":
		var params struct {
			ChatID int64 `json:"chat_id"`
			UserID int64 `json:"user_id"`
		}
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		c.mu.Lock()
		member := c.members[[2]int64{params.ChatID, params.UserID}]
		c.mu.Unlock()
		if member == nil {
			return nil, fmt.Errorf("no member response for chat %d user %d", params.ChatID, params.UserID)
		}
		c.record(dispatchAPICall{method: method, body: body})
		return apiResponse(member)
	case "sendMessage":
		var params struct {
			ChatID int64  `json:"chat_id"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.nextMessageID++
		messageID := c.nextMessageID
		c.calls = append(c.calls, dispatchAPICall{method: method, body: body, messageID: messageID})
		c.mu.Unlock()
		chatType := telego.ChatTypePrivate
		if params.ChatID < 0 {
			chatType = telego.ChatTypeSupergroup
		}
		return apiResponse(&telego.Message{
			MessageID: messageID,
			Chat:      telego.Chat{ID: params.ChatID, Type: chatType},
			Text:      params.Text,
		})
	case "editMessageText":
		var params struct {
			ChatID    int64  `json:"chat_id"`
			MessageID int    `json:"message_id"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal(body, &params); err != nil {
			return nil, err
		}
		c.record(dispatchAPICall{method: method, body: body, messageID: params.MessageID})
		return apiResponse(&telego.Message{
			MessageID: params.MessageID,
			Chat:      telego.Chat{ID: params.ChatID, Type: telego.ChatTypePrivate},
			Text:      params.Text,
		})
	case "answerCallbackQuery", "deleteMessage", "banChatSenderChat",
		"approveChatJoinRequest", "declineChatJoinRequest":
		c.record(dispatchAPICall{method: method, body: body})
		return apiResponse(true)
	case "setMyCommands":
		return c.setMyCommands(method, body)
	default:
		return nil, fmt.Errorf("unexpected Telegram method %q", method)
	}
}

func (c *dispatchCaller) setMyCommands(method string, body []byte) (*ta.Response, error) {
	c.record(dispatchAPICall{method: method, body: body})
	c.signalCommandMenu()
	return apiResponse(true)
}

func (c *dispatchCaller) record(call dispatchAPICall) {
	c.mu.Lock()
	c.calls = append(c.calls, call)
	c.mu.Unlock()
}

func (c *dispatchCaller) signalCommandMenu() {
	if c.commandMenus == nil {
		return
	}
	select {
	case c.commandMenus <- struct{}{}:
	default:
	}
}

func (c *dispatchCaller) setMember(chatID, userID int64, member telego.ChatMember) {
	c.mu.Lock()
	c.members[[2]int64{chatID, userID}] = member
	c.mu.Unlock()
}

func (c *dispatchCaller) snapshotCalls() []dispatchAPICall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]dispatchAPICall(nil), c.calls...)
}

func (c *dispatchCaller) methodCount(method string) int {
	count := 0
	for _, call := range c.snapshotCalls() {
		if call.method == method {
			count++
		}
	}
	return count
}

func (c *dispatchCaller) latestLaunchToken(t *testing.T) string {
	t.Helper()
	calls := c.snapshotCalls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method != "sendMessage" {
			continue
		}
		var params struct {
			ReplyMarkup struct {
				InlineKeyboard [][]struct {
					URL string `json:"url"`
				} `json:"inline_keyboard"`
			} `json:"reply_markup"`
		}
		if json.Unmarshal(calls[i].body, &params) != nil {
			continue
		}
		for _, row := range params.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				if index := strings.Index(button.URL, "?start=panel_"); index >= 0 {
					return button.URL[index+len("?start=panel_"):]
				}
			}
		}
	}
	t.Fatal("settings launch token was not sent")
	return ""
}

func (c *dispatchCaller) latestPanelCallback(t *testing.T, screen, field, value string) (string, int) {
	t.Helper()
	calls := c.snapshotCalls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method != "sendMessage" && calls[i].method != "editMessageText" {
			continue
		}
		var params struct {
			ReplyMarkup struct {
				InlineKeyboard [][]struct {
					CallbackData string `json:"callback_data"`
				} `json:"inline_keyboard"`
			} `json:"reply_markup"`
		}
		if json.Unmarshal(calls[i].body, &params) != nil {
			continue
		}
		for _, row := range params.ReplyMarkup.InlineKeyboard {
			for _, button := range row {
				parts := strings.Split(button.CallbackData, ":")
				if len(parts) == 6 && parts[2] == screen && parts[4] == field && parts[5] == value {
					return button.CallbackData, calls[i].messageID
				}
			}
		}
	}
	t.Fatalf("panel callback screen=%q field=%q value=%q was not sent", screen, field, value)
	return "", 0
}

func (c *dispatchCaller) latestForceReplyMessageID(t *testing.T) int {
	t.Helper()
	calls := c.snapshotCalls()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method != "sendMessage" {
			continue
		}
		var params struct {
			ReplyMarkup struct {
				ForceReply bool `json:"force_reply"`
			} `json:"reply_markup"`
		}
		if json.Unmarshal(calls[i].body, &params) == nil && params.ReplyMarkup.ForceReply {
			return calls[i].messageID
		}
	}
	t.Fatal("panel ForceReply prompt was not sent")
	return 0
}

func (c *dispatchCaller) sentTexts() []string {
	var texts []string
	for _, call := range c.snapshotCalls() {
		if call.method != "sendMessage" {
			continue
		}
		var params struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(call.body, &params) == nil {
			texts = append(texts, params.Text)
		}
	}
	return texts
}

func (c *dispatchCaller) callbackAnswers() []telego.AnswerCallbackQueryParams {
	var answers []telego.AnswerCallbackQueryParams
	for _, call := range c.snapshotCalls() {
		if call.method != "answerCallbackQuery" {
			continue
		}
		var params telego.AnswerCallbackQueryParams
		if json.Unmarshal(call.body, &params) == nil {
			answers = append(answers, params)
		}
	}
	return answers
}

type dispatchFixture struct {
	groupID             int64
	requiredChannel     int64
	botID               int64
	bot                 *telego.Bot
	cfg                 *config.Config
	connector           *telegram.Connector
	caller              *dispatchCaller
	settings            *store.Settings
	verification        *verification.Service
	verificationGateway *telegram.VerificationGateway
	administration      *panel.Panel
	moderation          *moderate.Service
	lookups             *lookup.Service
	application         *telegram.Updates
}

func newDispatchFixture(t *testing.T, requiredChannel int64) *dispatchFixture {
	t.Helper()
	const (
		groupID int64 = -1009000000801
		botID   int64 = 800
	)
	var channelID *int64
	if requiredChannel != 0 {
		channelID = &requiredChannel
	}
	cfg := &config.Config{
		Groups: []config.GroupConfig{{
			ID:                groupID,
			Lang:              "en",
			VerifyMode:        config.ModeKernel,
			RequiredChannelID: channelID,
			ChannelDisplay:    "@required",
		}},
		GroupIDs:            []int64{groupID},
		Lang:                "en",
		VerifyMode:          config.ModeKernel,
		BlockChannelSenders: boolPtr(true),
		PrivateQueryPerMin:  4,
	}
	cfg.PrivateReply = i18n.Messages.Bot.DirectMessage.AutoReply.Render(i18n.LangEN, cfg.PrivateQueryPerMin)
	settings, err := store.NewSettings("", botTestSettingsBaseline(t, cfg))
	if err != nil {
		t.Fatal(err)
	}
	caller := &dispatchCaller{botID: botID, members: make(map[[2]int64]telego.ChatMember)}
	telegramBot := testBot(t, caller)
	connector := telegram.NewConnector(telegramBot)
	stateDirectory := t.TempDir()
	verification := newTestVerifier(
		settings, connector, cfg,
		verification.Identity{ID: botID, Username: "dispatch_bot"}, stateDirectory,
	)
	verificationGateway := telegram.NewVerificationGateway(connector)
	moderation := moderate.New(settings, connector, cfg, nil)
	lookups := lookup.New(settings, connector, cfg, "")
	administration := panel.New(
		settings, connector, cfg, &i18n.Messages, verification, moderation, lookups, "test", time.Now(),
	)
	application := telegram.NewUpdates(cfg, settings, connector, telegramHandlers(verification, verificationGateway, administration, moderation, lookups))
	return &dispatchFixture{
		groupID:             groupID,
		requiredChannel:     requiredChannel,
		botID:               botID,
		bot:                 telegramBot,
		caller:              caller,
		cfg:                 cfg,
		connector:           connector,
		settings:            settings,
		verification:        verification,
		verificationGateway: verificationGateway,
		administration:      administration,
		moderation:          moderation,
		lookups:             lookups,
		application:         application,
	}
}

func runDirectHandler(t *testing.T, bot *telego.Bot, handler th.Handler, update telego.Update) {
	t.Helper()
	botHandler, err := th.NewBotHandler(bot, nil)
	if err != nil {
		t.Fatal(err)
	}
	botHandler.Handle(handler)
	if err := botHandler.BaseGroup().HandleUpdate(context.Background(), bot, update); err != nil {
		t.Fatal(err)
	}
}

func (f *dispatchFixture) joinRequest(userID int64) telego.Update {
	return telego.Update{ChatJoinRequest: &telego.ChatJoinRequest{
		Chat:       telego.Chat{ID: f.groupID, Type: telego.ChatTypeSupergroup},
		From:       telego.User{ID: userID, FirstName: "Applicant", LanguageCode: "en"},
		UserChatID: userID,
	}}
}

func (f *dispatchFixture) preparePanelInput(t *testing.T, userID int64) telego.Update {
	t.Helper()
	f.caller.setMember(f.groupID, userID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator,
		User:   telego.User{ID: userID},
	})
	f.caller.setMember(f.groupID, f.botID, &telego.ChatMemberAdministrator{
		Status: telego.MemberStatusAdministrator,
		User:   telego.User{ID: f.botID},
	})
	runDirectHandler(t, f.bot, f.administration.OnSettings, telego.Update{Message: &telego.Message{
		MessageID: 1,
		Chat:      telego.Chat{ID: f.groupID, Type: telego.ChatTypeSupergroup},
		From:      &telego.User{ID: userID, LanguageCode: "en"},
		Text:      "/settings",
	}})
	token := f.caller.latestLaunchToken(t)
	runDirectHandler(t, f.bot, f.administration.OnStart, telego.Update{Message: &telego.Message{
		MessageID: 2,
		Chat:      telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
		From:      &telego.User{ID: userID, LanguageCode: "en"},
		Text:      "/start panel_" + token,
	}})
	click := func(screen, field, value string) {
		t.Helper()
		data, messageID := f.caller.latestPanelCallback(t, screen, field, value)
		runDirectHandler(t, f.bot, f.administration.OnSettingsCallback, telego.Update{CallbackQuery: &telego.CallbackQuery{
			ID:   screen + field,
			From: telego.User{ID: userID, LanguageCode: "en"},
			Message: &telego.Message{
				MessageID: messageID,
				Chat:      telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
			},
			Data: data,
		}})
	}
	click("gl", "go", "gh")
	click("gh", "go", "vp")
	click("vp", "to", "_")
	promptID := f.caller.latestForceReplyMessageID(t)
	return telego.Update{Message: &telego.Message{
		MessageID: 3,
		Chat:      telego.Chat{ID: userID, Type: telego.ChatTypePrivate},
		From:      &telego.User{ID: userID, LanguageCode: "en"},
		Text:      "300",
		ReplyToMessage: &telego.Message{
			MessageID: promptID,
		},
	}}
}
