package verification

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func (v *Service) loadRecoveryState() (time.Time, []PendingRecord, error) {
	lastOnline, err := v.loadHeartbeat()
	if err != nil {
		v.disablePendingState(err)
		return time.Time{}, nil, err
	}
	records, err := v.stateStore.LoadPending(v.statePath)
	if err != nil {
		v.disablePendingState(err)
		return time.Time{}, nil, fmt.Errorf("load pending challenges: %w", err)
	}
	return lastOnline, records, nil
}

func (v *Service) disablePendingState(err error) {
	if errors.Is(err, ErrStoreReadOnly) {
		v.statePath = ""
	}
}

func recoveryDowntime(now, lastOnline time.Time) time.Duration {
	if lastOnline.IsZero() || !now.After(lastOnline) {
		return 0
	}
	return now.Sub(lastOnline)
}

func (v *Service) restorePendingRecord(bot Gateway, record PendingRecord, now time.Time, longOutage bool) (renotifyItem, bool) {
	gid, uid := record.GroupID, record.UserID
	if !v.settings.IsGroup(gid) {
		log.Printf("state load: skip pending for unknown group %d (user %d)", gid, uid)
		v.supersedePendingRecord(record)
		return renotifyItem{}, false
	}
	if !validRestoredChallenge(record) {
		log.Printf("state load: skip pending with invalid question payload (group %d user %d)", gid, uid)
		v.supersedePendingRecord(record)
		return renotifyItem{}, false
	}
	p := pendingFromRecord(record)
	delay, reason, renotify := v.restoredDeadline(gid, uid, p, now, longOutage)
	if !v.installRestoredPending(bot, record, p, delay, reason) {
		return renotifyItem{}, false
	}
	item := renotifyItem{gid: gid, uid: uid, name: record.Name, oldMessages: p.messages(), p: p}
	return item, restoredChallengeNeedsRenotify(true, renotify)
}

func validRestoredChallenge(record PendingRecord) bool {
	mode := record.Mode
	if mode == "" {
		mode = settings.ModeQuiz
	}
	if mode != settings.ModeQuiz {
		return true
	}
	return len(record.QOpts) >= 2 && record.CorrectIdx >= 0 && record.CorrectIdx < len(record.QOpts)
}

func (v *Service) restoredDeadline(gid, uid int64, p *pending, now time.Time, longOutage bool) (time.Duration, string, bool) {
	delay := p.deadline.Sub(now)
	reason := challengeExpiryReason(p.challengeDelivered && !p.fallbackPending)
	renotify := false
	if !p.deferralCapReached && !p.deferredSince.IsZero() && !now.Before(p.deferredSince.Add(maxVerificationDeferral)) {
		p.deferralCapReached = true
		logDeferralCapReached(gid, uid)
	}
	switch {
	case p.deferralCapReached:
		reason = deferredExpiryReason
		if delay <= 0 || delay > noFaultGrace {
			delay = noFaultGrace
			p.deadline = now.Add(delay)
		}
	case longOutage:
		delay = max(v.gateTimeout(gid, p.gate), recoveryWindow)
		p.deadline = now.Add(delay)
		p.lastRenotify = now
		reason = "recovered"
		renotify = true
	case delay <= 0:
		delay = noFaultGrace
		p.deadline = now.Add(delay)
		reason = "restart-lapsed"
	case delay < time.Second:
		delay = time.Second
	}
	return delay, reason, renotify
}

func (v *Service) installRestoredPending(bot Gateway, record PendingRecord, p *pending, delay time.Duration, reason string) bool {
	gid, uid := record.GroupID, record.UserID
	v.mu.Lock()
	key := pkey{gid, uid}
	if _, replacing := v.pend[key]; !replacing && !v.pendingCapacityOKLocked(gid) {
		v.supersedePendingRecord(record)
		v.mu.Unlock()
		log.Printf("state load: pending cap reached; leaving user %d in group %d for manual review", uid, gid)
		v.alertPendingCap(context.Background(), bot, gid, record.Gate)
		return false
	}
	v.pend[key] = p
	p.persistedPath = v.statePath
	expectedEpoch := p.epoch
	v.armExpiry(bot, p, gid, uid, delay, reason)
	persisted := v.persistPendingLocked(key, p, expectedEpoch)
	v.mu.Unlock()
	return persisted
}

func (v *Service) renotifyRestored(bot Gateway, refresh []renotifyItem, downtime time.Duration) {
	if len(refresh) == 0 {
		return
	}
	capped := 0
	if len(refresh) > renotifyCap {
		capped = len(refresh) - renotifyCap
		refresh = refresh[:renotifyCap]
	}
	for _, item := range refresh {
		v.renotifyPending(context.Background(), bot, item.gid, item.uid, item.name, item.oldMessages, item.p, downtime)
	}
	log.Printf("recovery: re-notified %d restored verification(s) after ~%s down%s", len(refresh), downtime.Round(time.Second), capNote(capped))
}
