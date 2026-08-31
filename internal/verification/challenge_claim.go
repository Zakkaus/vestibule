package verification

import "log"

// Caller holds v.mu.
func (v *Service) releaseTerminalLocked(key pkey, p *pending) {
	if v.terminal[key] == p {
		delete(v.terminal, key)
	}
}

func (v *Service) claimPendingLocked(
	key pkey,
	p *pending,
	state ChallengeState,
	reason string,
	settledBy int64,
) bool {
	changed, err := v.transitionChallengeLocked(
		key, p, ChallengePending, state, reason, settledBy, p.epoch,
	)
	if err != nil {
		log.Printf("verification: claim %s for group %d user %d: %v", state, key.gid, key.uid, err)
		return false
	}
	if !changed {
		v.forgetPendingLocked(key, p)
		return false
	}
	p.done = true
	p.claimedState = state
	if p.timer != nil {
		p.timer.Stop()
	}
	v.markTerminalLocked(key, p)
	return true
}

// consume claims an administrator ban while keeping it recoverable until Telegram settlement returns.
func (v *Service) consume(gid, uid int64) (*pending, bool) {
	return v.consumeBy(gid, uid, 0)
}

func (v *Service) consumeBy(gid, uid, settledBy int64) (*pending, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || !v.claimPendingLocked(key, p, ChallengeBanned, "", settledBy) {
		return nil, false
	}
	return p, true
}

// Nonce and timer epoch must both match in memory and in storage, so superseded timers
// cannot claim a replacement or a re-armed challenge.
func (v *Service) claimPendingExpiry(gid, uid int64, nonce string, epoch uint64, reason string) (*pending, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown {
		return nil, false
	}
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || p.nonce != nonce || p.epoch != epoch {
		return nil, false
	}
	state := ChallengeExpired
	storedReason := ""
	if p.passing || reason == "approve-retry" {
		state = ChallengeApproved
	} else if reason == "decline-retry" || reason == "ban-retry" {
		state = ChallengeDeclined
		storedReason = "rejected"
	}
	if !v.claimPendingLocked(key, p, state, storedReason, 0) {
		return nil, false
	}
	p.failedAt = v.wallNow()
	return p, true
}
