package telegram

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// AutoReplyStore returns one group's rules in ordinal order.
type AutoReplyStore interface {
	ListRules(context.Context, int64, string) ([]rules.Record, error)
}

type cachedAutoReply struct {
	definition []byte
	rule       rules.AutoReply
	last       time.Time
	loaded     bool
	present    bool
}

type autoReplyHandler struct {
	store     AutoReplyStore
	settings  SettingsReader
	connector *Connector
	now       func() time.Time
	mu        sync.Mutex
	groups    map[int64]map[string]*cachedAutoReply
}

// NewAutoReplyHandler creates a group-only responder, independent of the private_reply throttle.
func NewAutoReplyHandler(store AutoReplyStore, settings SettingsReader, connector *Connector) th.Handler {
	handler := &autoReplyHandler{store: store, settings: settings, connector: connector, now: time.Now, groups: make(map[int64]map[string]*cachedAutoReply)}
	return handler.handle
}

func (h *autoReplyHandler) handle(ctx *th.Context, update telego.Update) error {
	message := update.Message
	if !autoReplyMessage(message) || !h.settings.IsGroup(message.Chat.ID) {
		return nil
	}
	records, err := h.store.ListRules(ctx.Context(), message.Chat.ID, rules.AutoReplyCollection)
	if err != nil {
		log.Printf("auto_reply read for group %d failed: %v", message.Chat.ID, err)
		return nil
	}
	reply := h.match(message.Chat.ID, message.Text, records)
	if reply != "" {
		h.connector.ReplyPlain(ctx.Context(), message.Chat.ID, message.MessageID, reply, 0)
	}
	return nil
}

func autoReplyMessage(message *telego.Message) bool {
	if message == nil || message.From == nil || message.From.IsBot || message.SenderChat != nil || message.Text == "" {
		return false
	}
	if message.Chat.Type != telego.ChatTypeGroup && message.Chat.Type != telego.ChatTypeSupergroup {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(message.Text), "/") {
		return false
	}
	for _, entity := range message.Entities {
		if entity.Type == telego.EntityTypeBotCommand {
			return false
		}
	}
	return true
}

func (h *autoReplyHandler) match(chatID int64, text string, records []rules.Record) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	cache := h.groups[chatID]
	if cache == nil {
		cache = make(map[string]*cachedAutoReply)
		h.groups[chatID] = cache
	}
	pruneAutoReplies(cache, records)
	now := h.now()
	for _, record := range records {
		if !record.Enabled {
			continue
		}
		state := cache[record.ID]
		if state == nil {
			state = &cachedAutoReply{}
			cache[record.ID] = state
		}
		if !state.loaded || !bytes.Equal(state.definition, record.Definition) {
			state.loaded = true
			state.definition = append(state.definition[:0], record.Definition...)
			decoded, err := rules.DecodeAutoReply(record.Definition)
			state.rule = decoded
			if err != nil {
				log.Printf("auto_reply rule %q in group %d disabled: %v", record.ID, chatID, err)
			}
		}
		if !state.rule.Matches(text) {
			continue
		}
		if !state.last.IsZero() && now.Sub(state.last) < state.rule.Cooldown {
			return ""
		}
		state.last = now
		return state.rule.Reply
	}
	return ""
}

func pruneAutoReplies(cache map[string]*cachedAutoReply, records []rules.Record) {
	for _, state := range cache {
		state.present = false
	}
	for _, record := range records {
		if state := cache[record.ID]; state != nil {
			state.present = true
		}
	}
	for id, state := range cache {
		if !state.present {
			delete(cache, id)
		}
	}
}
