package status

import (
	"context"
	"sync/atomic"
)

// Health separates process liveness from dependencies required to accept console traffic.
type Health struct {
	live            atomic.Bool
	configReady     atomic.Bool
	telegramReady   atomic.Bool
	databaseHealthy func(context.Context) error
}

// NewHealth constructs a live process whose readiness remains false until startup confirms it.
func NewHealth(databaseHealthy func(context.Context) error) *Health {
	health := &Health{databaseHealthy: databaseHealthy}
	health.live.Store(true)
	return health
}

func (h *Health) SetLive(ready bool) {
	h.live.Store(ready)
}

func (h *Health) SetConfigReady(ready bool) {
	h.configReady.Store(ready)
}

func (h *Health) SetTelegramReady(ready bool) {
	h.telegramReady.Store(ready)
}

// Live does not inspect the database, Telegram, or another dependency.
func (h *Health) Live() bool {
	return h.live.Load()
}

// Ready requires completed configuration, an established Telegram channel, and a live database.
func (h *Health) Ready(ctx context.Context) bool {
	if !h.configReady.Load() || !h.telegramReady.Load() || h.databaseHealthy == nil {
		return false
	}
	return h.databaseHealthy(ctx) == nil
}
