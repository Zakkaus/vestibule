package verification

import (
	"context"
	"log"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

const (
	expiryScanInterval = 5 * time.Second
	expiryClaimLease   = 30 * time.Second
	expiryScanBatch    = 32
)

// RunExpiryScanner settles durable deadlines. It never owns a per-challenge timer: a restart or
// a second process merely sees the same due row, whose conditional lease lets one worker proceed.
func (v *Service) RunExpiryScanner(ctx context.Context) {
	for {
		v.ScanExpired(ctx)
		timer := time.NewTimer(expiryScanInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-timer.C:
		}
	}
}

// ScanExpired performs one bounded expiry pass. It is exported for lifecycle assembly and tests;
// callers supply a cancellable context so shutdown cannot start new settlement work.
func (v *Service) ScanExpired(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	v.mu.Lock()
	if v.shuttingDown || v.stateUnavailable(v.statePath) {
		v.mu.Unlock()
		return
	}
	store, namespace, bot, now := v.stateStore, v.statePath, v.gateway, v.wallNow()
	v.mu.Unlock()
	claimed, err := store.ClaimExpired(namespace, now.Unix(), now.Add(expiryClaimLease).Unix(), expiryScanBatch)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("verification expiry scan: %v", err)
		}
		return
	}
	for _, record := range claimed {
		if ctx.Err() != nil || !v.installExpiryClaim(record) {
			return
		}
		v.onExpiry(ctx, bot, record.GroupID, record.UserID, record.Nonce, record.Epoch, expiryReason(record))
	}
}

func expiryReason(record PendingRecord) string {
	if record.DeferralCapReached {
		return deferredExpiryReason
	}
	if record.Passing {
		return "approve-retry"
	}
	return challengeExpiryReason(record.ChallengeDelivered && !record.FallbackPending)
}

// installExpiryClaim makes the database lease visible to this process before the legacy
// settlement helpers inspect the in-memory entry. A record created by another instance is fully
// reconstructed; an existing pointer is retained so in-flight handlers can notice its new epoch.
func (v *Service) installExpiryClaim(record PendingRecord) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown {
		return false
	}
	key := pkey{gid: record.GroupID, uid: record.UserID}
	if current := v.pend[key]; current != nil && !current.done && current.nonce == record.Nonce {
		current.deadline = time.Unix(record.Deadline, 0)
		current.epoch = record.Epoch
		return true
	}
	p := pendingFromRecord(record)
	p.persistedPath = v.statePath
	v.pend[key] = p
	return true
}

func pendingFromRecord(record PendingRecord) *pending {
	mode := record.Mode
	if mode == "" {
		mode = "quiz"
	}
	var deferredSince time.Time
	if record.DeferredSince != 0 {
		deferredSince = time.Unix(record.DeferredSince, 0)
	}
	var failedAt time.Time
	if record.FailedAt != 0 {
		failedAt = time.Unix(record.FailedAt, 0)
	}
	var createdAt time.Time
	if record.CreatedAt != 0 {
		createdAt = time.Unix(record.CreatedAt, 0)
	}
	return &pending{
		groupMsgID: record.GroupMsgID, privateMsgID: record.PrivateMsgID,
		challengeDelivered: record.ChallengeDelivered || record.GroupMsgID != 0 || record.PrivateMsgID != 0,
		mode:               mode, lang: i18n.FromStored(record.Lang), storedLang: record.Lang, preserveStoredLang: true,
		fbAnswers: record.FbAnswers, fallbackPending: record.FallbackPending, prompted: record.Prompted,
		tries: record.Tries, hinted: record.Hinted, sampleBounced: record.SampleBounced,
		noLinuxReminded: record.NoLinuxReminded, osClarified: record.OSClarified,
		qText: record.QText, qOpts: record.QOpts, correctIdx: record.CorrectIdx,
		nonce: record.Nonce, name: record.Name, createdAt: createdAt, deadline: time.Unix(record.Deadline, 0), epoch: record.Epoch,
		failedAt: failedAt, deferredSince: deferredSince, deferralCapReached: record.DeferralCapReached,
		settleFailures: record.SettleFailures, gate: record.Gate, invited: record.Invited, held: record.Held,
		holdUntil: record.HoldUntil, passing: record.Passing, channelUnreadable: record.ChannelUnreadable,
		settlePendingSaid: record.SettlePendingSaid,
	}
}
