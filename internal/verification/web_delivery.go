package verification

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// WebModeUnavailable reports runtime capability or private-delivery constraints without rewriting settings.
func (v *Service) WebModeUnavailable(gid int64, mode string) string {
	if !settings.IsWebMode(mode) {
		return ""
	}
	group, exists := v.groupSettings(gid)
	if exists && group.DeliveryMode().Value == settings.DeliveryGroup {
		return "private_delivery_required"
	}
	return v.settings.WebModeUnavailable(mode)
}

func (v *Service) sendWebChallenge(ctx context.Context, bot Gateway, uid int64, prompt dmPrompt, resend bool) (int, error) {
	v.mu.Lock()
	p := v.pend[pkey{prompt.gid, uid}]
	if p != prompt.pending || p == nil || p.done || p.nonce != prompt.nonce {
		v.mu.Unlock()
		return 0, nil
	}
	message := OutgoingMessage{ChatID: prompt.chatID, ChallengeID: ChallengeID(pendingRecord(pkey{prompt.gid, uid}, p).Ref()),
		WebMode: p.mode, Language: p.persistedLang(), WebToken: p.webToken, Resend: resend}
	if v.RequiredChannelID(prompt.gid) != 0 {
		message.HTML = true
		message.Text = v.messages.Verification.Web.ChannelRequired.For(p.lang) + "\n\n" + v.channelLinkHTML(prompt.gid, p.lang)
	}
	v.mu.Unlock()
	return bot.Send(ctx, message)
}

// Recovery publishes only the /start link: hashed tokens cannot be recovered or silently rotated.
func (v *Service) renotifyWebPending(ctx context.Context, bot Gateway, gid, uid int64, name string, old challengeMessages, p *pending) {
	messageID := v.postGroupChallenge(ctx, bot, gid, uid, name, v.groupLanguage(gid), v.voiceFor(gid, p))
	if messageID == 0 {
		return
	}
	v.mu.Lock()
	current := v.pend[pkey{gid, uid}] == p && !p.done
	if current {
		p.groupMsgID = messageID
		current = v.persistPendingLocked(pkey{gid, uid}, p, p.epoch)
	}
	v.mu.Unlock()
	if !current {
		v.deleteChallenge(ctx, bot, gid, messageID)
		return
	}
	v.deleteChallenges(ctx, bot, gid, uid, old)
}

// DeliverWebFallback delivers a committed C5 replacement without charging the resend throttle.
func (v *Service) DeliverWebFallback(ctx context.Context, id string) {
	key, nonce, valid := parseChallengeID(id)
	if !valid {
		return
	}
	prompt, exists := v.pendingDMChallenge(key.gid, key.uid, nil)
	if !exists || prompt.nonce != nonce || !prompt.fallbackPending {
		return
	}
	v.mu.Lock()
	p := prompt.pending
	if v.pend[key] != p || p.done {
		v.mu.Unlock()
		return
	}
	name, oldMessages := p.name, p.messages()
	v.mu.Unlock()
	delivery := v.deliverPendingChallenge(ctx, v.gateway, key.gid, key.uid, name, p)
	if !delivery.active || !delivery.delivered {
		return
	}
	v.mu.Lock()
	current := v.pend[key] == p && !p.done
	if current {
		p.groupMsgID = delivery.messages.groupMsgID
		current = v.persistPendingLocked(key, p, p.epoch)
	}
	v.mu.Unlock()
	if !current {
		v.deleteChallenges(ctx, v.gateway, key.gid, key.uid, delivery.messages)
		return
	}
	v.deleteChallenges(ctx, v.gateway, key.gid, key.uid, oldMessages)
}

func (v *Service) privateChallengeDestination(uid int64, prompt dmPrompt, resend bool) int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	p := prompt.pending
	if p == nil || resend {
		return uid
	}
	if settings.IsWebMode(prompt.mode) {
		if p.userChatID > 0 && p.requestDate > 0 && v.wallNow().Unix() < p.requestDate+300 {
			return p.userChatID
		}
	} else if prompt.fallbackPending && !prompt.fallback && p.privateChatID > 0 {
		return p.privateChatID
	}
	return uid
}

func recordPrivateDestination(p *pending, prompt dmPrompt, question bool) bool {
	if !question || prompt.chatID == 0 || p.privateChatID == prompt.chatID {
		return false
	}
	p.privateChatID = prompt.chatID
	return true
}

func (v *Service) webChannelRequired(gid int64) WebResult {
	return WebResult{Outcome: WebChannelRequired, ChannelName: v.channelDisplay(gid), ChannelURL: v.channelURL(gid)}
}

func (v *Service) continueWebFallback(ctx context.Context, gid, uid int64) bool {
	prompt, exists := v.pendingDMChallenge(gid, uid, nil)
	if !exists || !prompt.fallbackPending || prompt.fallback {
		return false
	}
	v.DeliverWebFallback(ctx, ChallengeID(PendingRef{GroupID: gid, UserID: uid, Nonce: prompt.nonce}))
	return true
}
