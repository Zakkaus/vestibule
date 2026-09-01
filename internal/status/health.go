package status

import (
	"context"
	"sync/atomic"
	"time"
)

// TelegramProbe records one successful Bot API probe.
type TelegramProbe struct {
	At      time.Time
	Latency time.Duration
}

// HealthSnapshot captures Health state without invoking the database readiness check.
type HealthSnapshot struct {
	Live          bool
	ConfigReady   bool
	TelegramReady bool
	TelegramProbe *TelegramProbe
}

type telegramProbe struct {
	at      time.Time
	latency time.Duration
}

// Health separates process liveness from dependencies required to accept console traffic.
type Health struct {
	live            atomic.Bool
	configReady     atomic.Bool
	telegramReady   atomic.Bool
	telegramProbe   atomic.Pointer[telegramProbe]
	databaseHealthy func(context.Context) error
}

// RecordTelegramProbe records the time and latency of a successful Bot API probe.
func (h *Health) RecordTelegramProbe(at time.Time, latency time.Duration) {
	if at.IsZero() {
		return
	}
	h.telegramProbe.Store(&telegramProbe{at: at, latency: latency})
}

// Snapshot returns the atomically recorded Health state.
func (h *Health) Snapshot() HealthSnapshot {
	snapshot := HealthSnapshot{
		Live:          h.live.Load(),
		ConfigReady:   h.configReady.Load(),
		TelegramReady: h.telegramReady.Load(),
	}
	if probe := h.telegramProbe.Load(); probe != nil {
		snapshot.TelegramProbe = &TelegramProbe{At: probe.at, Latency: probe.latency}
	}
	return snapshot
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
