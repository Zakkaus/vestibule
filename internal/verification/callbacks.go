package verification

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

type answerCallback struct {
	gid    int64
	owner  int64
	nonce  string
	choice int
}

func parseAnswerCallback(data string) (answerCallback, bool) {
	parts := strings.Split(strings.TrimPrefix(data, AnswerCallbackPrefix), ":")
	var nonce, choiceText string
	switch len(parts) {
	case 4:
		nonce, choiceText = parts[2], parts[3]
	case 3:
		choiceText = parts[2]
	default:
		return answerCallback{}, false
	}
	choice, err := strconv.Atoi(choiceText)
	if err != nil {
		return answerCallback{}, false
	}
	gid, _ := strconv.ParseInt(parts[0], 10, 64)
	owner, _ := strconv.ParseInt(parts[1], 10, 64)
	return answerCallback{gid: gid, owner: owner, nonce: nonce, choice: choice}, true
}

// OnAnswer settles one nonce-bound quiz callback.
func (v *Service) OnAnswer(ctx *HandlerContext, update Update) error {
	cq := update.CallbackQuery
	if cq == nil {
		return nil
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	answer, ok := parseAnswerCallback(cq.Data)
	if !ok {
		ackFast(c, bot, cq.ID)
		return nil
	}
	ul := v.applicantLanguage(answer.gid, answer.owner, cq.From.LanguageCode)
	groupLang := v.groupLanguage(answer.gid)
	result := &(*v.messages).Verification.Result
	channel := &(*v.messages).Verification.Channel
	if cq.From.ID != answer.owner {
		ackResult(c, bot, cq.ID, result.NotYours.For(ul), true)
		return nil
	}

	v.mu.Lock()
	p, exists := v.pend[pkey{answer.gid, answer.owner}]
	done := !exists || p.done
	correctIdx, currentNonce := -1, ""
	if exists {
		correctIdx, currentNonce = p.correctIdx, p.nonce
	}
	v.mu.Unlock()
	if done {
		ackResult(c, bot, cq.ID, result.AlreadyHandled.For(ul), false)
		return nil
	}
	if answer.nonce != currentNonce {
		// A stale button from a previous request cannot answer the current quiz.
		ackResult(c, bot, cq.ID, result.StaleQuestion.For(ul), true)
		return nil
	}

	if answer.choice != correctIdx {
		gate := v.pendingGate(answer.gid, answer.owner)
		outcome, banned, err := v.decline(c, bot, answer.gid, answer.owner, answer.nonce, wrongAnswerReason)
		if err != nil {
			ackFast(c, bot, cq.ID)
			return fmt.Errorf("settle quiz answer: %w", err)
		}
		text := v.voice(gate).AlreadyHandled.For(ul)
		if outcome != declineNoPending {
			text = v.declineResultText(outcome, ul, gate, func() string {
				return v.wrongAnswerText(answer.gid, ul, gate, banned)
			})
		}
		ackResult(c, bot, cq.ID, text, true)
		return nil
	}
	if !v.isChannelMember(c, bot, answer.gid, answer.owner, groupLang) {
		ackResult(c, bot, cq.ID, channel.NotFollowedYet.Render(ul, v.channelDisplay(answer.gid)), true)
		return nil
	}
	p, claimed, err := v.claimPendingNonce(answer.gid, answer.owner, answer.nonce)
	if err != nil {
		ackFast(c, bot, cq.ID)
		return fmt.Errorf("settle quiz answer: %w", err)
	}
	if claimed && v.executeApprove(c, bot, answer.gid, answer.owner, p) == approveConfirmed {
		text := v.voice(v.pendingGate(answer.gid, answer.owner)).Passed.For(ul)
		ackResult(c, bot, cq.ID, text, false)
		_, _ = sendText(c, bot, answer.owner, text)
	} else {
		ackResult(c, bot, cq.ID, result.AlreadyHandled.For(ul), true)
	}
	return nil
}

// Verification keeps no permanent record in the group: the failure notice for an
// administrator button is cleaned up like the challenge it belongs to.
const adminActionNoticeTTL = 240

// OnAdminAction settles one administrator approval or ban callback.
func (v *Service) OnAdminAction(ctx *HandlerContext, update Update) error {
	cq := update.CallbackQuery
	if cq == nil {
		return nil
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	parts := strings.SplitN(strings.TrimPrefix(cq.Data, AdminCallbackPrefix), ":", 4)
	if len(parts) < 3 {
		ackFast(c, bot, cq.ID)
		return nil
	}
	action := parts[0]
	gid, _ := strconv.ParseInt(parts[1], 10, 64)
	target, _ := strconv.ParseInt(parts[2], 10, 64)
	// The nonce prevents a stale button from settling a replacement verification.
	nonce := ""
	if len(parts) == 4 {
		nonce = parts[3]
	}

	l := v.groupLanguage(gid)
	admin := &v.messages.Verification.Admin
	if nonce != "" && !v.pendingHasNonce(gid, target, nonce) {
		ackResult(c, bot, cq.ID, admin.AlreadyHandled.For(l), true)
		return nil
	}
	if !v.isGroupAdmin(c, bot, gid, cq.From.ID) {
		ackResult(c, bot, cq.ID, admin.OnlyGroupAdmin.For(l), true)
		return nil
	}
	switch action {
	case "pass":
		return v.handleAdminPass(c, bot, cq, gid, target, l)
	case "ban":
		return v.handleAdminBan(c, bot, cq, gid, target, l)
	default:
		ackFast(c, bot, cq.ID)
		return nil
	}
}

func (v *Service) handleAdminPass(c context.Context, bot Gateway, cq *CallbackQuery, gid, target int64, l i18n.Lang) error {
	gate := v.pendingGate(gid, target)
	says := v.adminSays(gate)
	p, ok, err := v.claimPendingBy(gid, target, cq.From.ID)
	if err != nil {
		ackFast(c, bot, cq.ID)
		return fmt.Errorf("claim administrator approval: %w", err)
	}
	if !ok {
		ackResult(c, bot, cq.ID, says.CannotApprove.For(l), false)
		return nil
	}
	ackResult(c, bot, cq.ID, says.Approving.For(l), false)
	switch v.executeApprove(c, bot, gid, target, p) {
	case approveFailed:
		v.gatewayFor(bot).Notify(c, gid, says.ActionFailed.For(l), adminActionNoticeTTL)
	case approveGone:
		v.gatewayFor(bot).Notify(c, gid, says.AlreadyHandled.For(l), adminActionNoticeTTL)
	case approveConfirmed:
	}
	return nil
}

func (v *Service) handleAdminBan(c context.Context, bot Gateway, cq *CallbackQuery, gid, target int64, l i18n.Lang) error {
	gate := v.pendingGate(gid, target)
	says := v.adminSays(gate)
	p, ok, err := v.consumeBy(gid, target, cq.From.ID)
	if err != nil {
		ackFast(c, bot, cq.ID)
		return fmt.Errorf("claim administrator ban: %w", err)
	}
	if !ok {
		ackResult(c, bot, cq.ID, says.AlreadyHandled.For(l), false)
		return nil
	}
	duration := verificationBanDurationText(v.messages, l, v.verificationBanDuration(gid))
	ackResult(c, bot, cq.ID, says.Banning.Render(l, duration), false)
	if !v.executeBan(c, bot, gid, target, p) {
		v.gatewayFor(bot).Notify(c, gid, says.ActionFailed.For(l), adminActionNoticeTTL)
	}
	return nil
}
