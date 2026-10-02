package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

const replyTestChat int64 = -1009000000801
const replyOtherChat int64 = -1009000000802

type autoReplyCaller struct {
	sent []struct {
		ChatID    int64  `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}
}

func (c *autoReplyCaller) Call(_ context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	var message struct {
		ChatID    int64  `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}
	if !strings.HasSuffix(url, "/sendMessage") {
		panic("unexpected method: " + url)
	}
	if err := json.Unmarshal(data.BodyRaw, &message); err != nil {
		return nil, err
	}
	c.sent = append(c.sent, message)
	return apiResponse(&telego.Message{MessageID: len(c.sent)})
}

type autoReplyHarness struct {
	handler  *autoReplyHandler
	caller   *autoReplyCaller
	bot      *telego.Bot
	db       *database.Database
	store    *database.RuleStore
	now      time.Time
	commands CommandModules
}

func newAutoReplyHarness(t *testing.T) *autoReplyHarness {
	t.Helper()
	db, err := database.Open(context.Background(), database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, id := range []int64{replyTestChat, replyOtherChat} {
		if _, err = db.Exec(context.Background(), "INSERT INTO chat (id,title) VALUES ($1,'reply test')", id); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &settings.Config{GroupIDs: []int64{replyTestChat, replyOtherChat}, WarnLimit: 3}
	groups, err := settings.NewStore("", botTestSettingsBaseline(t, cfg), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h := &autoReplyHarness{db: db, store: database.NewRuleStore(db), caller: &autoReplyCaller{}, now: time.Unix(1000000, 0)}
	h.bot = testBot(t, h.caller)
	h.handler = &autoReplyHandler{store: h.store, settings: groups, connector: NewConnector(h.bot), now: func() time.Time { return h.now }, groups: make(map[int64]map[string]*cachedAutoReply)}
	return h
}

func replyRecord(id, trigger, reply string, enabled bool, cooldown int) rules.Record {
	data, _ := json.Marshal(map[string]any{"trigger": trigger, "reply": reply, "cooldown_seconds": cooldown})
	return rules.Record{ID: id, ChatID: replyTestChat, Collection: rules.AutoReplyCollection, Enabled: enabled, Definition: data}
}

func (h *autoReplyHarness) replace(t *testing.T, chatID int64, records []rules.Record) {
	t.Helper()
	old, err := h.store.ListRules(context.Background(), chatID, rules.AutoReplyCollection)
	if err != nil {
		t.Fatal(err)
	}
	for index := range records {
		records[index].Ordinal = index
		records[index].ChatID = chatID
	}
	if _, _, err = h.store.ReplaceRules(context.Background(), chatID, rules.AutoReplyCollection, old, records); err != nil {
		t.Fatal(err)
	}
}

func replyUpdate(chatID, userID int64, text string) telego.Update {
	return telego.Update{Message: &telego.Message{MessageID: 7, Chat: telego.Chat{ID: chatID, Type: telego.ChatTypeSupergroup}, From: &telego.User{ID: userID}, Text: text}}
}

func (h *autoReplyHarness) dispatch(t *testing.T, update telego.Update) {
	t.Helper()
	router := NewUpdates(&settings.Config{}, h.handler.settings, h.handler.connector, HandlerSet{
		AutoReply: h.handler.handle,
		Commands:  h.commands,
		Moderation: ModerationHandlers{FilterChannelSenders: func(ctx *th.Context, update telego.Update) error {
			return ctx.Next(update)
		}},
	})
	handler, err := th.NewBotHandler(h.bot, make(chan telego.Update))
	if err != nil {
		t.Fatal(err)
	}
	router.Register(handler)
	if err = handler.BaseGroup().HandleUpdate(context.Background(), h.bot, update); err != nil {
		t.Fatal(err)
	}
}

func TestAutoReplyOrderDisabledAndCooldown(t *testing.T) {
	h := newAutoReplyHarness(t)
	disabled := replyRecord("disabled", "matrix", "disabled", false, 600)
	first := replyRecord("first", "matrix", "<first>", true, 600)
	second := replyRecord("second", "matrix", "second", true, 600)
	h.replace(t, replyTestChat, []rules.Record{disabled, first, second})
	h.dispatch(t, replyUpdate(replyTestChat, 1, "matrix"))
	h.dispatch(t, replyUpdate(replyTestChat, 2, "matrix"))
	if len(h.caller.sent) != 1 || h.caller.sent[0].Text != "<first>" || h.caller.sent[0].ParseMode != "" {
		t.Fatalf("first-match or group cooldown broken: %+v", h.caller.sent)
	}
	h.replace(t, replyTestChat, []rules.Record{second, first, disabled})
	h.dispatch(t, replyUpdate(replyTestChat, 1, "matrix"))
	if len(h.caller.sent) != 2 || h.caller.sent[1].Text != "second" {
		t.Fatalf("reorder did not select new first rule: %+v", h.caller.sent)
	}
	h.now = h.now.Add(599 * time.Second)
	h.dispatch(t, replyUpdate(replyTestChat, 3, "matrix"))
	if len(h.caller.sent) != 2 {
		t.Fatal("cooldown expired before its boundary")
	}
	h.now = h.now.Add(time.Second)
	h.dispatch(t, replyUpdate(replyTestChat, 3, "matrix"))
	if len(h.caller.sent) != 3 || h.caller.sent[2].Text != "second" {
		t.Fatalf("cooldown boundary: %+v", h.caller.sent)
	}
	second.Enabled = false
	h.replace(t, replyTestChat, []rules.Record{second, first, disabled})
	h.dispatch(t, replyUpdate(replyTestChat, 3, "matrix"))
	if len(h.caller.sent) != 4 || h.caller.sent[3].Text != "<first>" {
		t.Fatalf("live disable ignored: %+v", h.caller.sent)
	}
}

func TestAutoReplyScopeAndConfiguredCooldown(t *testing.T) {
	h := newAutoReplyHarness(t)
	rule := replyRecord("one", "matrix", "one", true, 2)
	h.replace(t, replyTestChat, []rules.Record{rule})
	h.replace(t, replyOtherChat, []rules.Record{replyRecord("other", "matrix", "other", true, 2)})
	for _, test := range []string{"bot", "private", "command", "command-entity", "unmanaged"} {
		update := replyUpdate(replyTestChat, 1, "matrix")
		switch test {
		case "bot":
			update.Message.From.IsBot = true
		case "private":
			update.Message.Chat.Type = telego.ChatTypePrivate
		case "command":
			update.Message.Text = "/help matrix"
		case "command-entity":
			update.Message.Entities = []telego.MessageEntity{{Type: telego.EntityTypeBotCommand, Offset: 0, Length: 6}}
		case "unmanaged":
			update.Message.Chat.ID = -1009000000803
		}
		// Direct invocation proves DM exclusion independently of the existing DM route.
		runHandlerUpdates(t, h.bot, h.handler.handle, []telego.Update{update})
	}
	if len(h.caller.sent) != 0 {
		t.Fatalf("excluded messages replied: %+v", h.caller.sent)
	}
	h.dispatch(t, replyUpdate(replyTestChat, 1, "matrix"))
	h.dispatch(t, replyUpdate(replyOtherChat, 1, "matrix"))
	h.now = h.now.Add(time.Second)
	h.dispatch(t, replyUpdate(replyTestChat, 2, "matrix"))
	if len(h.caller.sent) != 2 {
		t.Fatalf("cross-group isolation: %+v", h.caller.sent)
	}
	h.now = h.now.Add(time.Second)
	h.dispatch(t, replyUpdate(replyTestChat, 2, "matrix"))
	if len(h.caller.sent) != 3 || h.caller.sent[1].ChatID != replyOtherChat {
		t.Fatalf("configured cooldown: %+v", h.caller.sent)
	}
}

func TestAutoReplyInvalidStoredRowLogsOnceAndContinues(t *testing.T) {
	h := newAutoReplyHarness(t)
	h.replace(t, replyTestChat, []rules.Record{replyRecord("bad", "matrix", "secret", true, 600), replyRecord("good", "matrix", "good", true, 600)})
	if _, err := h.db.Exec(context.Background(), "UPDATE rule SET definition=$1 WHERE id='bad'", `{`); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	h.dispatch(t, replyUpdate(replyTestChat, 1, "matrix"))
	h.dispatch(t, replyUpdate(replyTestChat, 2, "matrix"))
	if len(h.caller.sent) != 1 || h.caller.sent[0].Text != "good" || strings.Count(logs.String(), "disabled:") != 1 {
		t.Fatalf("invalid row blocked valid rule or repeated logging: sent=%+v logs=%s", h.caller.sent, logs.String())
	}
}

func TestAutoReplyLeavesCommandsAndPrivateRepliesAlone(t *testing.T) {
	h := newAutoReplyHarness(t)
	h.replace(t, replyTestChat, []rules.Record{replyRecord("rule", "matrix", "rule reply", true, 600)})
	command := commandDefinitionForTest("help", "test.help")
	command.Handler = func(ctx *th.Context, update telego.Update) error {
		h.handler.connector.ReplyPlain(ctx.Context(), update.Message.Chat.ID, update.Message.MessageID, "help result", 0)
		return nil
	}
	var err error
	h.commands, err = NewCommandModules(commandModuleForTest("core", false, command))
	if err != nil {
		t.Fatal(err)
	}
	h.dispatch(t, replyUpdate(replyTestChat, 1, "/help matrix"))
	private := replyUpdate(1, 1, "matrix")
	private.Message.Chat.Type = telego.ChatTypePrivate
	h.dispatch(t, private)
	h.dispatch(t, replyUpdate(replyTestChat, 1, "matrix"))
	if len(h.caller.sent) != 3 || h.caller.sent[0].Text != "help result" ||
		h.caller.sent[1].ChatID != 1 || h.caller.sent[1].Text == "rule reply" || h.caller.sent[2].Text != "rule reply" {
		t.Fatalf("command or private routing consumed by rules: %+v", h.caller.sent)
	}
}
