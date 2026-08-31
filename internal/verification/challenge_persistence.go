package verification

import "log"

func pendingRecord(key pkey, p *pending) PendingRecord {
	var deferredSince int64
	if !p.deferredSince.IsZero() {
		deferredSince = p.deferredSince.Unix()
	}
	return PendingRecord{
		UserID: key.uid, GroupID: key.gid,
		GroupMsgID: p.groupMsgID, PrivateMsgID: p.privateMsgID,
		ChallengeDelivered: p.challengeDelivered && p.groupMsgID == 0 && p.privateMsgID == 0,
		Mode:               p.mode, Lang: p.persistedLang(),
		FbAnswers: append([]string(nil), p.fbAnswers...), FallbackPending: p.fallbackPending, Prompted: p.prompted,
		Tries: p.tries, Hinted: p.hinted, SampleBounced: p.sampleBounced,
		NoLinuxReminded: p.noLinuxReminded, OSClarified: p.osClarified,
		QText: p.qText, QOpts: append([]string(nil), p.qOpts...), CorrectIdx: p.correctIdx, Nonce: p.nonce, Name: p.name,
		Deadline: p.deadline.Unix(), Epoch: p.epoch, DeferredSince: deferredSince, DeferralCapReached: p.deferralCapReached,
		SettleFailures: p.settleFailures, SettlePendingSaid: p.settlePendingSaid, Gate: p.gate, Invited: p.invited, Held: p.held, HoldUntil: p.holdUntil, Passing: p.passing,
		ChannelUnreadable: p.channelUnreadable,
	}
}

// save is retained for legacy-state tests. It issues one row operation per pending.
func (v *Service) save() {
	if v.stateUnavailable(v.statePath) {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for key, p := range v.pend {
		if p != nil && !p.done {
			v.persistPendingLocked(key, p, p.epoch)
		}
	}
}

func (v *Service) stateUnavailable(path string) bool {
	return path == "" || v.stateStore == nil
}

func (v *Service) updatePendingLocked(key pkey, p *pending, expectedEpoch uint64) (bool, error) {
	if v.stateUnavailable(v.statePath) {
		return true, nil
	}
	expected := PendingRef{GroupID: key.gid, UserID: key.uid, Nonce: p.nonce, Epoch: expectedEpoch}
	return v.stateStore.UpdatePending(v.statePath, expected, pendingRecord(key, p))
}

func (v *Service) transitionChallengeLocked(
	key pkey,
	p *pending,
	from, to ChallengeState,
	reason string,
	settledBy int64,
	expectedEpoch uint64,
) (bool, error) {
	if v.stateUnavailable(v.statePath) {
		return true, nil
	}
	return v.stateStore.TransitionChallenge(v.statePath, ChallengeTransition{
		Expected: PendingRef{GroupID: key.gid, UserID: key.uid, Nonce: p.nonce, Epoch: expectedEpoch},
		Record:   pendingRecord(key, p), From: from, To: to, Reason: reason,
		SettledAt: v.wallNow().Unix(), SettledBy: settledBy,
	})
}

func (v *Service) forgetPendingLocked(key pkey, p *pending) {
	if p.timer != nil {
		p.timer.Stop()
	}
	p.done = true
	if v.pend[key] == p {
		delete(v.pend, key)
	}
	v.releaseTerminalLocked(key, p)
}

func (v *Service) supersedePendingLocked(key pkey, p *pending) {
	from := ChallengePending
	if p.claimedState != "" {
		from = p.claimedState
	}
	if _, err := v.transitionChallengeLocked(key, p, from, ChallengeSuperseded, "", 0, p.epoch); err != nil {
		log.Printf("verification: supersede pending for group %d user %d: %v", key.gid, key.uid, err)
	}
	v.forgetPendingLocked(key, p)
}

func (v *Service) persistPendingLocked(key pkey, p *pending, expectedEpoch uint64) bool {
	var changed bool
	var err error
	if !v.stateUnavailable(v.statePath) && p.persistedPath != v.statePath {
		changed, err = v.stateStore.InsertPending(v.statePath, pendingRecord(key, p))
	} else {
		changed, err = v.updatePendingLocked(key, p, expectedEpoch)
	}
	if err != nil {
		log.Printf("verification: update pending for group %d user %d: %v", key.gid, key.uid, err)
	}
	if err != nil || !changed {
		v.forgetPendingLocked(key, p)
		return false
	}
	p.persistedPath = v.statePath
	return true
}

func (v *Service) deleteUnexposedPending(gid, uid int64, p *pending) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if v.pend[key] != p || p.done {
		return
	}
	if !v.stateUnavailable(v.statePath) {
		_, err := v.stateStore.DeletePending(v.statePath, pendingRecord(key, p).Ref())
		if err != nil {
			log.Printf("verification: delete undeliverable pending for group %d user %d: %v", gid, uid, err)
			return
		}
	}
	// A zero-row delete means another path already settled it; either way this cache entry is stale.
	v.forgetPendingLocked(key, p)
}

func (v *Service) supersedePendingRecord(record PendingRecord) {
	if v.stateUnavailable(v.statePath) {
		return
	}
	_, err := v.stateStore.TransitionChallenge(v.statePath, ChallengeTransition{
		Expected: record.Ref(), Record: record, From: ChallengePending, To: ChallengeSuperseded,
		SettledAt: v.wallNow().Unix(),
	})
	if err != nil {
		log.Printf("verification: supersede unrestorable pending for group %d user %d: %v", record.GroupID, record.UserID, err)
	}
}
