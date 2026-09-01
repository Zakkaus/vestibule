package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	actionScanInterval = 5 * time.Second
	actionClaimLease   = 30 * time.Second
	actionScanBatch    = 32

	actionApprove     = "settle_approve"
	actionDecline     = "settle_decline"
	actionBan         = "settle_ban"
	actionDeleteGroup = "delete_group_message"
)

type settlementActionPayload struct {
	Record PendingRecord  `json:"record"`
	State  ChallengeState `json:"state"`
	Reason string         `json:"reason"`
}

type deleteGroupActionPayload struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

func (v *Service) newSettlementAction(key pkey, p *pending, state ChallengeState, reason string) (ActionIntent, error) {
	kind := actionDecline
	switch state {
	case ChallengeApproved:
		kind = actionApprove
	case ChallengeBanned:
		kind = actionBan
	case ChallengeDeclined, ChallengeExpired:
	default:
		return ActionIntent{}, fmt.Errorf("challenge state %q has no settlement action", state)
	}
	now := v.wallNow()
	payload, err := json.Marshal(settlementActionPayload{
		Record: pendingRecord(key, p), State: state, Reason: reason,
	})
	if err != nil {
		return ActionIntent{}, fmt.Errorf("encode %s action: %w", kind, err)
	}
	return ActionIntent{
		ID:         fmt.Sprintf("settle:%d:%d:%s:%s", key.gid, key.uid, p.nonce, state),
		Kind:       kind,
		Payload:    string(payload),
		NextTryAt:  now.Unix(),
		ClaimOwner: v.actionOwner,
		ClaimUntil: now.Add(actionClaimLease).Unix(),
	}, nil
}

// RunPendingActions executes ready outbox rows. The timer schedules bounded passes; individual
// challenge deadlines never have in-process callbacks.
func (v *Service) RunPendingActions(ctx context.Context) {
	for {
		v.RunPendingActionsOnce(ctx)
		timer := time.NewTimer(actionScanInterval)
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

// RunPendingActionsOnce is the bounded action pass used by the runtime and focused tests.
func (v *Service) RunPendingActionsOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	v.mu.Lock()
	if v.shuttingDown || v.stateUnavailable(v.statePath) {
		v.mu.Unlock()
		return
	}
	store, namespace, owner, bot, now := v.stateStore, v.statePath, v.actionOwner, v.gateway, v.wallNow()
	v.mu.Unlock()
	actions, err := store.ClaimActions(namespace, owner, now.Unix(), now.Add(actionClaimLease).Unix(), actionScanBatch)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("verification action scan: %v", err)
		}
		return
	}
	for _, action := range actions {
		if ctx.Err() != nil {
			return
		}
		v.executePendingAction(ctx, bot, owner, action)
	}
}

func (v *Service) executePendingAction(ctx context.Context, bot Gateway, owner string, action PendingAction) {
	switch action.Kind {
	case actionApprove, actionDecline, actionBan:
		v.executeSettlementAction(ctx, bot, owner, action)
	case actionDeleteGroup:
		v.executeDeleteGroupAction(ctx, bot, owner, action)
	default:
		v.failPendingAction(action, owner, fmt.Errorf("unknown verification action kind %q", action.Kind))
	}
}

func (v *Service) executeSettlementAction(ctx context.Context, bot Gateway, owner string, action PendingAction) {
	var payload settlementActionPayload
	if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
		v.failPendingAction(action, owner, fmt.Errorf("decode settlement action: %w", err))
		return
	}
	p := v.installActionPending(payload, action, owner)
	if p == nil {
		v.failPendingAction(action, owner, fmt.Errorf("settlement action is obsolete"))
		return
	}
	switch payload.State {
	case ChallengeApproved:
		_ = v.executeApprove(ctx, bot, payload.Record.GroupID, payload.Record.UserID, p)
	case ChallengeBanned:
		_ = v.executeBan(ctx, bot, payload.Record.GroupID, payload.Record.UserID, p)
	case ChallengeDeclined, ChallengeExpired:
		_, _ = v.finishDecline(ctx, bot, payload.Record.GroupID, payload.Record.UserID, p, payload.Reason)
	default:
		v.failPendingAction(action, owner, fmt.Errorf("settlement action has state %q", payload.State))
	}
}

func (v *Service) installActionPending(payload settlementActionPayload, action PendingAction, owner string) *pending {
	key := pkey{gid: payload.Record.GroupID, uid: payload.Record.UserID}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown {
		return nil
	}
	p := v.pend[key]
	if p == nil || p.nonce != payload.Record.Nonce {
		p = pendingFromRecord(payload.Record)
		p.persistedPath = v.statePath
		v.pend[key] = p
	}
	p.done = true
	p.claimedState = payload.State
	p.actionID = action.ID
	p.actionOwner = owner
	p.actionAttempts = action.Attempts
	v.markTerminalLocked(key, p)
	return p
}

func (v *Service) executeDeleteGroupAction(ctx context.Context, bot Gateway, owner string, action PendingAction) {
	var payload deleteGroupActionPayload
	if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
		v.failPendingAction(action, owner, fmt.Errorf("decode group deletion action: %w", err))
		return
	}
	if payload.ChatID == 0 || payload.MessageID == 0 {
		v.completePendingAction(action, owner, nil)
		return
	}
	err := v.gatewayFor(bot).Delete(ctx, payload.ChatID, payload.MessageID)
	if err == nil || gatewayFailureHas(err, FailureMessageGone) {
		v.completePendingAction(action, owner, nil)
		return
	}
	v.retryOrFailPendingAction(action, owner, err)
}

func (v *Service) completeSettlementAction(gid int64, p *pending) {
	if p.actionID == "" || v.stateUnavailable(v.statePath) {
		return
	}
	followups := []ActionIntent(nil)
	if p.groupMsgID != 0 {
		payload, err := json.Marshal(deleteGroupActionPayload{ChatID: gid, MessageID: p.groupMsgID})
		if err != nil {
			log.Printf("verification: encode group delete action %s: %v", p.actionID, err)
			return
		}
		followups = append(followups, ActionIntent{
			ID:        p.actionID + ":group-delete",
			Kind:      actionDeleteGroup,
			Payload:   string(payload),
			NextTryAt: v.wallNow().Unix(),
		})
	}
	v.completePendingAction(PendingAction{ActionIntent: ActionIntent{ID: p.actionID}, Attempts: p.actionAttempts}, p.actionOwner, followups)
}

func (v *Service) cleanupSettledChallenge(c context.Context, bot Gateway, gid, uid int64, p *pending) {
	if p.actionID != "" && !v.stateUnavailable(v.statePath) {
		v.completeSettlementAction(gid, p)
		return
	}
	v.deleteChallenges(c, bot, gid, uid, p.messages())
}

func (v *Service) completePendingAction(action PendingAction, owner string, followups []ActionIntent) {
	completedAt := v.wallNow().Unix()
	changed, err := retryStoreChange(func() (bool, error) {
		return v.stateStore.CompleteAction(v.statePath, action.ID, owner, completedAt, followups)
	})
	if err != nil {
		log.Printf("verification: complete action %s: %v", action.ID, err)
		return
	}
	if !changed {
		log.Printf("verification: action %s completion lost its lease", action.ID)
	}
}

func (v *Service) retrySettlementAction(p *pending, err error) bool {
	if p.actionID == "" || v.stateUnavailable(v.statePath) {
		return false
	}
	return v.retryOrFailPendingAction(PendingAction{
		ActionIntent: ActionIntent{ID: p.actionID}, Attempts: p.actionAttempts,
	}, p.actionOwner, err)
}

func (v *Service) retryOrFailPendingAction(action PendingAction, owner string, err error) bool {
	attempts := action.Attempts + 1
	if attempts >= maxSettleFailures || giveUpSettling(err) {
		v.failPendingAction(action, owner, err)
		return false
	}
	now := v.wallNow()
	delay := actionRetryDelay(attempts, err)
	nextTryAt := now.Add(delay).Unix()
	changed, retryErr := retryStoreChange(func() (bool, error) {
		return v.stateStore.RetryAction(v.statePath, action.ID, owner, attempts, nextTryAt, err.Error())
	})
	if retryErr != nil {
		log.Printf("verification: retry action %s: %v", action.ID, retryErr)
		return false
	}
	if !changed {
		log.Printf("verification: action %s retry lost its lease", action.ID)
		return false
	}
	return true
}

func (v *Service) failPendingAction(action PendingAction, owner string, err error) {
	if action.ID == "" || v.stateUnavailable(v.statePath) {
		return
	}
	failedAt := v.wallNow().Unix()
	changed, failErr := retryStoreChange(func() (bool, error) {
		return v.stateStore.FailAction(v.statePath, action.ID, owner, failedAt, err.Error())
	})
	if failErr != nil {
		log.Printf("verification: fail action %s: %v", action.ID, failErr)
		return
	}
	if !changed {
		log.Printf("verification: action %s failure lost its lease", action.ID)
	}
}

func actionRetryDelay(attempts int, err error) time.Duration {
	if retryAfter := gatewayFailureRetryAfter(err); retryAfter > 0 {
		return retryAfter
	}
	delay := 5 * time.Second
	for range attempts - 1 {
		if delay >= 5*time.Minute/2 {
			return 5 * time.Minute
		}
		delay *= 2
	}
	return delay
}
