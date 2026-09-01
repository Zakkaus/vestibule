package verification

import "fmt"

// Caller holds v.mu.
func (v *Service) releaseTerminalLocked(key pkey, p *pending) {
	if v.terminal[key] == p {
		delete(v.terminal, key)
	}
}

// Keep claimed approvals in the map so network failure can reopen them.
func (v *Service) claimPending(gid, uid int64) (*pending, bool, error) {
	return v.claimPendingBy(gid, uid, 0)
}

func (v *Service) claimPendingBy(gid, uid, settledBy int64) (*pending, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done {
		return nil, false, nil
	}
	claimed, err := v.claimPendingLocked(key, p, ChallengeApproved, "", settledBy)
	if err != nil || !claimed {
		return nil, false, err
	}
	return p, true, nil
}

// Bind answer validation and the database transition to the same nonce.
func (v *Service) claimPendingNonce(gid, uid int64, nonce string) (*pending, bool, error) {
	return v.claimPendingNonceAs(gid, uid, nonce, ChallengeApproved, "", 0)
}

func (v *Service) claimPendingNonceAs(
	gid, uid int64,
	nonce string,
	state ChallengeState,
	reason string,
	settledBy int64,
) (*pending, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown {
		return nil, false, nil
	}
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || p.nonce != nonce {
		return nil, false, nil
	}
	claimed, err := v.claimPendingLocked(key, p, state, reason, settledBy)
	if err != nil || !claimed {
		return nil, false, err
	}
	return p, true, nil
}

func (v *Service) claimPendingLocked(
	key pkey,
	p *pending,
	state ChallengeState,
	reason string,
	settledBy int64,
) (bool, error) {
	if p.failedAt.IsZero() {
		p.failedAt = v.wallNow()
	}
	var actions []ActionIntent
	if !v.stateUnavailable(v.statePath) {
		action, err := v.newSettlementAction(key, p, state, reason)
		if err != nil {
			return false, fmt.Errorf("prepare %s action for group %d user %d: %w", state, key.gid, key.uid, err)
		}
		actions = []ActionIntent{action}
	}
	changed, err := v.transitionChallengeLocked(
		key, p, ChallengePending, state, reason, settledBy, p.epoch, actions...,
	)
	if err != nil {
		return false, fmt.Errorf("claim %s for group %d user %d: %w", state, key.gid, key.uid, err)
	}
	if !changed {
		v.forgetPendingLocked(key, p)
		return false, nil
	}
	p.done = true
	p.claimedState = state
	if len(actions) != 0 {
		p.actionID = actions[0].ID
		p.actionOwner = actions[0].ClaimOwner
	}
	v.markTerminalLocked(key, p)
	return true, nil
}

// consume claims an administrator ban while keeping it recoverable until Telegram settlement returns.
func (v *Service) consume(gid, uid int64) (*pending, bool, error) {
	return v.consumeBy(gid, uid, 0)
}

func (v *Service) consumeBy(gid, uid, settledBy int64) (*pending, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done {
		return nil, false, nil
	}
	claimed, err := v.claimPendingLocked(key, p, ChallengeBanned, "", settledBy)
	if err != nil || !claimed {
		return nil, false, err
	}
	return p, true, nil
}

// Nonce and scanner epoch must both match in memory and storage, so a superseded scan
// cannot claim a replacement or a re-armed challenge.
func (v *Service) claimPendingExpiry(gid, uid int64, nonce string, epoch uint64, reason string) (*pending, bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown {
		return nil, false, nil
	}
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || p.nonce != nonce || p.epoch != epoch {
		return nil, false, nil
	}
	state := ChallengeExpired
	storedReason := ""
	if p.passing || reason == "approve-retry" {
		state = ChallengeApproved
	} else if reason == "decline-retry" || reason == "ban-retry" {
		state = ChallengeDeclined
		storedReason = "rejected"
	}
	claimed, err := v.claimPendingLocked(key, p, state, storedReason, 0)
	if err != nil || !claimed {
		return nil, false, err
	}
	return p, true, nil
}
