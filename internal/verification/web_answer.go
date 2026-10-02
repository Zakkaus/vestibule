package verification

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// SetTurnstileVerifier supplies the provider at process assembly, before handling requests.
func (v *Service) SetTurnstileVerifier(verifier TurnstileVerifier) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.turnstile = verifier
}

func parseChallengeID(id string) (pkey, string, bool) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return pkey{}, "", false
	}
	gid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return pkey{}, "", false
	}
	uid, err := strconv.ParseInt(parts[1], 10, 64)
	return pkey{gid, uid}, parts[2], err == nil
}

// IssueWebToken is called once before delivery, or explicitly to rotate on resend.
func (v *Service) IssueWebToken(ctx context.Context, id string) (string, WebTokenRecord, error) {
	store, ok := v.stateStore.(WebStore)
	if !ok {
		return "", WebTokenRecord{}, fmt.Errorf("durable web store required")
	}
	key, nonce, ok := parseChallengeID(id)
	if !ok {
		return "", WebTokenRecord{}, fmt.Errorf("invalid challenge ID")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.pend[key]
	if p == nil || p.done || p.nonce != nonce || !settings.IsWebMode(p.mode) {
		return "", WebTokenRecord{}, nil
	}
	group, _ := v.groupSettings(key.gid)
	raw, record, err := NewWebToken(group.PoWBits().Value, v.wallNow().Unix())
	if err != nil {
		return "", record, err
	}
	record.ChallengeID = id
	written, err := store.IssueWebToken(ctx, pendingRecord(key, p).Ref(), record)
	if err != nil || !written {
		return "", record, err
	}
	return raw, record, nil
}

// AnswerWeb checks proofs and channel membership before atomically claiming the challenge and outbox.
// OperatorAlert tells the adapter to report a provider configuration error without logging proofs.
func (v *Service) AnswerWeb(ctx context.Context, id string, proof WebProof) (WebResult, error) {
	settled := WebResult{Outcome: WebSettled}
	token, valid, err := v.loadWebToken(ctx, id, proof.Token)
	if err != nil || !valid {
		return settled, err
	}
	target, ok := v.webAnswerTarget(id)
	if !ok {
		return settled, nil
	}
	key, p, ref := target.key, target.pending, target.ref
	outcome := checkWebProof(ctx, target.mode, target.verifier, token, proof)
	if outcome == TurnstileOutage || outcome == TurnstileConfiguration {
		return v.webUnavailable(ctx, key, p, ref, token.TokenHash, outcome)
	}
	if outcome == TurnstilePassed && !v.isChannelMember(ctx, v.gateway, key.gid, key.uid, v.groupLanguage(key.gid)) {
		return WebResult{Outcome: WebChannelRequired}, nil
	}
	claimed, state, err := v.claimWebAnswer(ctx, key, p, ref, token.TokenHash, outcome == TurnstileFailed, ChallengeApproved)
	if err != nil || !claimed {
		return settled, err
	}
	if state == ChallengePending {
		return WebResult{Outcome: WebWrong}, nil
	}
	if state == ChallengeDeclined {
		_, _ = v.finishDecline(ctx, v.gateway, key.gid, key.uid, p, wrongAnswerReason)
	} else {
		v.executeApprove(ctx, v.gateway, key.gid, key.uid, p)
	}
	return WebResult{Outcome: WebReceived}, nil
}

func (v *Service) claimWebAnswer(ctx context.Context, key pkey, p *pending, ref PendingRef, hash string, wrong bool, state ChallengeState) (bool, ChallengeState, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.shuttingDown || !v.webPendingMatches(key, p, ref) {
		return false, state, nil
	}
	next := *p
	reason := ""
	if state == ChallengeDeclined {
		reason = "challenge-post-failed"
	}
	if wrong {
		next.tries++
		state = ChallengePending
		if next.tries >= kernelMaxTries {
			state, reason = ChallengeDeclined, wrongAnswerReason
		}
	}
	transition := ChallengeTransition{Expected: ref, Record: pendingRecord(key, &next), From: ChallengePending, To: state,
		Reason: "", SettledAt: v.wallNow().Unix(), WebClaim: &WebClaim{TokenHash: hash, Now: v.wallNow().Unix()}}
	if state == ChallengeDeclined {
		transition.Reason = storedDeclineReason(reason)
	}
	if state != ChallengePending {
		next.failedAt = v.wallNow()
		transition.Record = pendingRecord(key, &next)
		action, err := v.newSettlementAction(key, &next, state, reason)
		if err != nil {
			return false, state, err
		}
		transition.Actions = []ActionIntent{action}
	}
	changed, err := v.stateStore.TransitionChallenge(v.statePath, transition)
	if err != nil || !changed {
		return false, state, err
	}
	*p = next
	if state != ChallengePending {
		p.done, p.claimedState = true, state
		p.actionID, p.actionOwner = transition.Actions[0].ID, transition.Actions[0].ClaimOwner
		v.markTerminalLocked(key, p)
	}
	return true, state, nil
}

type webAnswerTarget struct {
	key      pkey
	pending  *pending
	ref      PendingRef
	mode     string
	verifier TurnstileVerifier
}

func (v *Service) loadWebToken(ctx context.Context, id, raw string) (WebTokenRecord, bool, error) {
	store, ok := v.stateStore.(WebStore)
	if !ok {
		return WebTokenRecord{}, false, fmt.Errorf("durable web store required")
	}
	return store.LoadWebToken(ctx, id, HashWebToken(raw), v.wallNow().Unix())
}

func (v *Service) webAnswerTarget(id string) (webAnswerTarget, bool) {
	key, nonce, ok := parseChallengeID(id)
	if !ok {
		return webAnswerTarget{}, false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.pend[key]
	if p == nil || p.done || p.nonce != nonce || !settings.IsWebMode(p.mode) {
		return webAnswerTarget{}, false
	}
	return webAnswerTarget{key, p, pendingRecord(key, p).Ref(), p.mode, v.turnstile}, true
}

func (v *Service) webPendingMatches(key pkey, p *pending, ref PendingRef) bool {
	return v.pend[key] == p && !p.done && p.epoch == ref.Epoch && p.nonce == ref.Nonce && settings.IsWebMode(p.mode)
}

func checkWebProof(ctx context.Context, mode string, verifier TurnstileVerifier, token WebTokenRecord, proof WebProof) TurnstileOutcome {
	if mode == settings.ModePoW {
		if VerifyPoW(token.Salt, proof.Nonce, token.PoWBits) {
			return TurnstilePassed
		}
		return TurnstileFailed
	}
	if verifier == nil {
		return TurnstileConfiguration
	}
	return verifier.Verify(ctx, proof.CaptchaResponse)
}

// ResolveWebToken is a non-consuming lookup for the public surface; it never rotates.
func (v *Service) ResolveWebToken(ctx context.Context, raw string) (PendingRecord, WebTokenRecord, bool, error) {
	store, ok := v.stateStore.(WebStore)
	if !ok {
		return PendingRecord{}, WebTokenRecord{}, false, fmt.Errorf("durable web store required")
	}
	token, valid, err := store.ResolveWebToken(ctx, HashWebToken(raw), v.wallNow().Unix())
	if err != nil || !valid {
		return PendingRecord{}, token, false, err
	}
	key, nonce, ok := parseChallengeID(token.ChallengeID)
	if !ok {
		return PendingRecord{}, token, false, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	p := v.pend[key]
	if p == nil || p.done || p.nonce != nonce || !settings.IsWebMode(p.mode) {
		return PendingRecord{}, token, false, nil
	}
	return pendingRecord(key, p), token, true, nil
}
