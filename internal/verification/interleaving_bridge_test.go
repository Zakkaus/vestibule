package verification

import (
	"path/filepath"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// InterleavingService installs the clock before restoration, without a production clock API.
func InterleavingService(groups *settings.Store, bot Gateway, store Store, cfg *settings.Config,
	directory string, clock func() time.Time,
) (*Service, error) {
	v, err := New(groups, bot, store, cfg, &i18n.Messages, nil,
		Identity{ID: 999, Username: "interleaving_bot"}, "", nil)
	if err != nil {
		return nil, err
	}
	v.timeNow = clock
	v.lastOnline = clock()
	v.statePath = filepath.Join(directory, "pending.json")
	v.vfailPath = filepath.Join(directory, "verifyfail.json")
	v.agentPath = filepath.Join(directory, "agents.json")
	v.hbPath = filepath.Join(directory, "heartbeat.json")
	if err := v.loadVerifyFails(); err != nil {
		return nil, err
	}
	if err := v.loadAgents(); err != nil {
		return nil, err
	}
	if err := v.load(bot); err != nil {
		return nil, err
	}
	if err := v.loadRecentPasses(); err != nil {
		return nil, err
	}
	return v, nil
}

// Timeout jumps model an online scheduler; advance/restart can still model downtime.
func (v *Service) InterleavingOnline() {
	v.mu.Lock()
	v.lastOnline = v.wallNow()
	v.mu.Unlock()
	v.saveHeartbeat()
}

type InterleavingMemory struct {
	Live    []PendingRecord
	Claimed map[string]string
	Passed  []PendingRef
}

func (v *Service) InterleavingSnapshot() InterleavingMemory {
	v.mu.Lock()
	defer v.mu.Unlock()
	result := InterleavingMemory{Claimed: make(map[string]string)}
	for key, p := range v.pend {
		if p != nil && !p.done {
			result.Live = append(result.Live, pendingRecord(key, p))
		}
	}
	for key, p := range v.terminal {
		if p != nil {
			result.Claimed[challengeConsoleID(key.gid, key.uid, p.nonce)] = p.actionID
		}
	}
	for key, at := range v.passed {
		if v.wallNow().Sub(at) <= recentPassWindow {
			result.Passed = append(result.Passed, PendingRef{GroupID: key.gid, UserID: key.uid})
		}
	}
	return result
}
