package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

const actionReleaseHold = "release_verification_hold"

// CleanupActions encodes idempotent group-message deletion and optional hold release intents.
func CleanupActions(record PendingRecord, at int64, release bool) ([]ActionIntent, error) {
	var actions []ActionIntent
	if release && record.Gate == gateMute && record.Held {
		payload, err := json.Marshal(settlementActionPayload{Record: record})
		if err != nil {
			return nil, err
		}
		actions = append(actions, ActionIntent{
			ID:   fmt.Sprintf("cleanup:%d:%d:%s:release", record.GroupID, record.UserID, record.Nonce),
			Kind: actionReleaseHold, Payload: string(payload), NextTryAt: at,
		})
	}
	if record.GroupMsgID != 0 {
		payload, err := json.Marshal(deleteGroupActionPayload{ChatID: record.GroupID, MessageID: record.GroupMsgID})
		if err != nil {
			return nil, err
		}
		actions = append(actions, ActionIntent{
			ID:   fmt.Sprintf("cleanup:%d:%d:delete", record.GroupID, record.GroupMsgID),
			Kind: actionDeleteGroup, Payload: string(payload), NextTryAt: at,
		})
	}
	return actions, nil
}

// Persist before the first network attempt. The normal action worker owns retries,
// including after cancellation removed the challenge from the in-memory live set.
func (v *Service) cleanupChallenge(ctx context.Context, bot Gateway, record PendingRecord, release bool) bool {
	if v.stateUnavailable(v.statePath) {
		return false
	}
	now := v.wallNow()
	intents, err := CleanupActions(record, now.Unix(), release)
	if err != nil {
		log.Printf("verification: encode cleanup: %v", err)
		return false
	}
	for i := range intents {
		intents[i].ClaimOwner = v.actionOwner
		intents[i].ClaimUntil = now.Add(actionClaimLease).Unix()
	}
	var actions []PendingAction
	err = retryStoreWrite(nil, func() error {
		var err error
		actions, err = v.stateStore.EnqueueActions(v.statePath, record.Ref(), intents)
		return err
	})
	if err != nil {
		log.Printf("verification: persist cleanup for %d in %d: %v", record.UserID, record.GroupID, err)
		return false
	}
	for _, action := range actions {
		v.executePendingAction(ctx, bot, v.actionOwner, action)
	}
	return true
}

func (v *Service) cleanupCanceledChallenge(ctx context.Context, bot Gateway, record PendingRecord) {
	if v.cleanupChallenge(ctx, bot, record, true) {
		return
	}
	v.deleteChallenge(ctx, bot, record.GroupID, record.GroupMsgID)
	if record.Gate == gateMute && record.Held {
		if err := v.releaseMember(ctx, bot, record.GroupID, record.UserID, pendingFromRecord(record)); err != nil {
			log.Printf("verification: could not release canceled hold: %v", err)
		}
	}
}

// Message cleanup never deletes the applicant's private conversation.
func (v *Service) deleteChallenges(ctx context.Context, bot Gateway, gid, uid int64, owner *pending, messages challengeMessages) {
	if messages.groupMsgID == 0 {
		return
	}
	if owner != nil {
		v.mu.Lock()
		record := pendingRecord(pkey{gid, uid}, owner)
		record.GroupMsgID = messages.groupMsgID
		v.mu.Unlock()
		if v.cleanupChallenge(ctx, bot, record, false) {
			return
		}
	}
	v.deleteChallenge(ctx, bot, gid, messages.groupMsgID)
}

func (v *Service) deleteChallenge(ctx context.Context, bot Gateway, gid int64, msgID int) {
	if msgID == 0 {
		return
	}
	if err := v.gatewayFor(bot).Delete(ctx, gid, msgID); err != nil && !gatewayFailureHas(err, FailureMessageGone) {
		log.Printf("verification: delete message %d in %d: %v", msgID, gid, err)
	}
}

func (v *Service) executeReleaseHoldAction(ctx context.Context, bot Gateway, owner string, action PendingAction) {
	var payload settlementActionPayload
	if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
		v.failPendingAction(action, owner, fmt.Errorf("decode hold cleanup: %w", err))
		return
	}
	record := payload.Record
	v.mu.Lock()
	current := v.pend[pkey{record.GroupID, record.UserID}]
	replaced := current != nil && current.nonce != record.Nonce
	v.mu.Unlock()
	if replaced {
		v.completePendingAction(action, owner, nil)
		return
	}
	var stillBanned bool
	err := v.releaseMember(ctx, bot, record.GroupID, record.UserID, pendingFromRecord(record))
	if err != nil && giveUpSettling(err) {
		stillBanned, err = v.removeMember(ctx, bot, record.GroupID, record.UserID)
	}
	if err != nil {
		v.retryOrFailPendingAction(action, owner, err)
		return
	}
	var followups []ActionIntent
	if stillBanned {
		encoded, err := json.Marshal(undoBanActionPayload{ChatID: record.GroupID, UserID: record.UserID})
		if err != nil {
			v.retryOrFailPendingAction(action, owner, err)
			return
		}
		followups = []ActionIntent{{
			ID: action.ID + ":undo-ban", Kind: actionUndoBan,
			Payload: string(encoded), NextTryAt: v.wallNow().Unix(),
		}}
	}
	v.completePendingAction(action, owner, followups)
}
