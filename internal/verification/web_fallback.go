package verification

import (
	"context"
	"log"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func (v *Service) webUnavailable(ctx context.Context, key pkey, p *pending, ref PendingRef, hash string, outcome TurnstileOutcome) (WebResult, error) {
	group, _ := v.groupSettings(key.gid)
	policy := group.CaptchaUnavailable().Value
	alert := outcome == TurnstileConfiguration
	if alert {
		policy = settings.CaptchaFallback
	}
	if policy == settings.CaptchaFallback {
		changed, err := v.fallbackWeb(ctx, key, p, ref, &WebClaim{TokenHash: hash, Now: v.wallNow().Unix()})
		result := WebResult{Outcome: WebSettled, OperatorAlert: alert}
		if changed {
			result.Outcome = WebFallback
		}
		return result, err
	}
	state := ChallengeDeclined
	if policy == settings.CaptchaApprove {
		state = ChallengeApproved
		if !v.isChannelMember(ctx, v.gateway, key.gid, key.uid, v.groupLanguage(key.gid)) {
			return WebResult{Outcome: WebChannelRequired}, nil
		}
	}
	claimed, _, err := v.claimWebAnswer(ctx, key, p, ref, hash, false, state)
	if err != nil || !claimed {
		return WebResult{Outcome: WebSettled}, err
	}
	if state == ChallengeApproved {
		v.executeApprove(ctx, v.gateway, key.gid, key.uid, p)
	} else {
		_, _ = v.finishDecline(ctx, v.gateway, key.gid, key.uid, p, "challenge-post-failed")
	}
	return WebResult{Outcome: WebReceived}, nil
}

// FallbackWeb retains the challenge identity, gate and hold; delivery starts the new answer window.
func (v *Service) FallbackWeb(ctx context.Context, id string) (bool, error) {
	key, nonce, ok := parseChallengeID(id)
	if !ok {
		return false, nil
	}
	v.mu.Lock()
	p := v.pend[key]
	if p == nil || p.done || p.nonce != nonce {
		v.mu.Unlock()
		return false, nil
	}
	ref := pendingRecord(key, p).Ref()
	v.mu.Unlock()
	return v.fallbackWeb(ctx, key, p, ref, nil)
}

func (v *Service) fallbackWeb(ctx context.Context, key pkey, p *pending, ref PendingRef, claim *WebClaim) (bool, error) {
	store, ok := v.stateStore.(WebStore)
	if !ok {
		return false, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pend[key] != p || p.done || p.epoch != ref.Epoch || !settings.IsWebMode(p.mode) {
		return false, nil
	}
	next := *p
	next.mode = settings.ModeQuiz
	bank := v.questions(key.gid)
	if len(bank) == 0 {
		next.mode, next.qText, next.qOpts, next.correctIdx = settings.ModeKernel, tgfmt.KernelQuestion(v.messages, p.lang), nil, -1
	} else {
		next.qText, next.qOpts, next.correctIdx = shuffledQuestion(randomQuestion(bank))
	}
	next.challengeDelivered, next.fallbackPending, next.prompted = false, true, false
	next.deadline = v.wallNow().Add(pendingDeliveryTimeout)
	next.epoch++
	changed, err := store.FallbackWeb(ctx, ref, pendingRecord(key, &next), claim)
	if err != nil || !changed {
		return false, err
	}
	*p = next
	log.Printf("verification: web fallback for group %d user %d", key.gid, key.uid)
	return true, nil
}
