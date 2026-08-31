package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/telegram"
	th "github.com/mymmrac/telego/telegohandler"
)

const (
	updatePollLeaseTTL   = 45 * time.Second
	updatePollLeaseRenew = 15 * time.Second
)

type updatePollLease interface {
	Acquire(context.Context, string, int64, int64) (bool, error)
	Renew(context.Context, string, int64, int64) (bool, error)
	Release(context.Context, string) error
}

// pollingLease owns the single durable right to consume Telegram updates. Losing renewal cancels
// the long-poll context; the process stays alive for its non-update work and can be restarted or
// reconfigured without two instances handling the same update.
type pollingLease struct {
	lease updatePollLease
	owner string
	held  bool
	now   func() time.Time

	mu        sync.Mutex
	polling   *telegram.Polling
	cancel    context.CancelFunc
	done      chan error
	stopping  bool
	leaseLost bool
	once      sync.Once
}

func acquirePollingLease(ctx context.Context, lease updatePollLease) (*pollingLease, error) {
	p := &pollingLease{lease: lease, owner: newPollLeaseOwner(), now: time.Now}
	now := p.now()
	held, err := lease.Acquire(ctx, p.owner, now.Unix(), now.Add(updatePollLeaseTTL).Unix())
	if err != nil {
		return nil, fmt.Errorf("acquire update-poll lease: %w", err)
	}
	p.held = held
	if !held {
		log.Printf("update polling is disabled: another instance holds the database lease")
	}
	return p, nil
}

func newPollLeaseOwner() string {
	var token [12]byte
	if _, err := rand.Read(token[:]); err == nil {
		return "poll-" + hex.EncodeToString(token[:])
	}
	return fmt.Sprintf("poll-%d", time.Now().UnixNano())
}

func (p *pollingLease) Start(ctx context.Context, runtime *services) error {
	if !p.held {
		return nil
	}
	if err := runtime.registration.EnsureOwnerClaim(); err != nil {
		log.Printf("WARNING: owner claim is unavailable until durable settings storage is restored: %v", err)
	}
	runtime.updates.SetupCommands(ctx, runtime.bot)
	pollCtx, cancel := context.WithCancel(ctx)
	polling, err := telegram.StartPolling(pollCtx, runtime.bot, func(handler *th.BotHandler) {
		runtime.registration.Register(handler)
		runtime.updates.Register(handler)
	})
	if err != nil {
		cancel()
		p.release(ctx)
		return err
	}
	p.mu.Lock()
	p.polling, p.cancel, p.done = polling, cancel, make(chan error, 1)
	p.mu.Unlock()
	go p.forwardPollingDone(polling.Done())
	go p.renew(pollCtx)
	return nil
}

func (p *pollingLease) Done() <-chan error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}

func (p *pollingLease) forwardPollingDone(done <-chan error) {
	err := <-done
	p.mu.Lock()
	intentional := p.stopping || p.leaseLost
	out := p.done
	p.mu.Unlock()
	if !intentional && out != nil {
		out <- err
	}
}

func (p *pollingLease) renew(ctx context.Context) {
	ticker := time.NewTicker(updatePollLeaseRenew)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !p.renewOnce(ctx) {
				return
			}
		}
	}
}

func (p *pollingLease) renewOnce(ctx context.Context) bool {
	now := p.now()
	renewed, err := p.lease.Renew(ctx, p.owner, now.Unix(), now.Add(updatePollLeaseTTL).Unix())
	if err == nil && renewed {
		return true
	}
	if err != nil {
		log.Printf("update polling lease renewal failed: %v; cancelling long polling", err)
	} else {
		log.Printf("update polling lease was lost; cancelling long polling")
	}
	p.mu.Lock()
	p.leaseLost = true
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return false
}

func (p *pollingLease) Stop(ctx context.Context) error {
	var stopErr error
	p.once.Do(func() {
		p.mu.Lock()
		p.stopping = true
		cancel, polling := p.cancel, p.polling
		p.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		p.release(ctx)
		if polling != nil {
			stopErr = polling.Stop(ctx)
		}
	})
	return stopErr
}

func (p *pollingLease) release(ctx context.Context) {
	if !p.held {
		return
	}
	if err := p.lease.Release(ctx, p.owner); err != nil {
		log.Printf("release update-poll lease: %v", err)
	}
}

func newRuntimePollingLease(ctx context.Context, runtime *services) (*pollingLease, error) {
	return acquirePollingLease(ctx, database.NewUpdatePollLease(runtime.database))
}
