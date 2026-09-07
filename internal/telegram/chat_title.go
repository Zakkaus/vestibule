package telegram

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	chatTitleTTL      = time.Hour
	chatTitleFailTTL  = 45 * time.Second
	chatTitleCacheMax = 4096
	// Shared lookups outlive individual waiters but remain bounded.
	chatTitleRequestTimeout = 3 * time.Second
)

type chatTitleEntry struct {
	title   string
	expires time.Time
	ok      bool
}

type chatTitleCall struct {
	done  chan struct{}
	title string
	err   error
}

var errChatTitleUnavailable = errors.New("chat title unavailable")

// ChatTitle returns a group's current Telegram title. Successful and failed lookups are cached
// separately, and concurrent misses for one chat share one GetChat request.
func (c *Connector) ChatTitle(ctx context.Context, chatID int64) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c.titleMu.Lock()
	if entry, ok := c.titleCache[chatID]; ok && c.titleNow().Before(entry.expires) {
		c.titleMu.Unlock()
		if entry.ok {
			return entry.title, nil
		}
		return "", errChatTitleUnavailable
	}
	call, ok := c.titleInFlight[chatID]
	if !ok {
		call = &chatTitleCall{done: make(chan struct{})}
		if c.titleInFlight == nil {
			c.titleInFlight = make(map[int64]*chatTitleCall)
		}
		c.titleInFlight[chatID] = call
		go c.loadChatTitle(context.WithoutCancel(ctx), chatID, call)
	}
	c.titleMu.Unlock()
	select {
	case <-call.done:
		return call.title, call.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (c *Connector) loadChatTitle(ctx context.Context, chatID int64, call *chatTitleCall) {
	title, err := c.fetchChatTitle(ctx, chatID)
	c.titleMu.Lock()
	defer c.titleMu.Unlock()
	delete(c.titleInFlight, chatID)
	call.title, call.err = title, err
	ttl := chatTitleFailTTL
	if err == nil {
		ttl = chatTitleTTL
	}
	c.cacheChatTitleLocked(chatID, chatTitleEntry{
		title: title, ok: err == nil, expires: c.titleNow().Add(ttl),
	})
	close(call.done)
}

func (c *Connector) fetchChatTitle(ctx context.Context, chatID int64) (string, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, chatTitleRequestTimeout)
	defer cancel()
	chat, err := c.bot.GetChat(lookupCtx, &telego.GetChatParams{ChatID: tu.ID(chatID)})
	if err != nil || lookupCtx.Err() != nil || chat == nil || strings.TrimSpace(chat.Title) == "" {
		return "", errChatTitleUnavailable
	}
	return chat.Title, nil
}

func (c *Connector) cacheChatTitleLocked(chatID int64, entry chatTitleEntry) {
	delete(c.titleCache, chatID)
	if len(c.titleCache) >= chatTitleCacheMax {
		now := c.titleNow()
		for key, cached := range c.titleCache {
			if !now.Before(cached.expires) {
				delete(c.titleCache, key)
			}
		}
	}
	if len(c.titleCache) >= chatTitleCacheMax {
		for key := range c.titleCache {
			delete(c.titleCache, key)
			break
		}
	}
	c.titleCache[chatID] = entry
}
