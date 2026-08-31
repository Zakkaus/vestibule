package verification

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/edition"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

// Three replies tolerate typos while bounding DM guess floods.
const kernelMaxTries = 3

// The impossible placeholder cannot collide with a real release.
// samplePrompt is the placeholder the prompt shows and the answer rule rejects. Both come
// from one constant so that a build cannot display one shape and detect another.
const samplePrompt = "X.Y.Z" + edition.KernelExampleSuffix

var kernelAnswerRule = rules.Rule{
	Accept: []rules.Condition{rules.VersionRange{Intervals: []rules.VersionInterval{
		{Minimum: rules.Version{Major: 0, Minor: 1}, Maximum: rules.Version{Major: 0, Minor: 99}},
		{Minimum: rules.Version{Major: 1}, Maximum: rules.Version{Major: 1, Minor: 3}},
		{Minimum: rules.Version{Major: 2}, Maximum: rules.Version{Major: 2, Minor: 6}},
		{Minimum: rules.Version{Major: 3}, Maximum: rules.Version{Major: 30, Minor: 99}},
	}}},
	Reject: []rules.Condition{rules.OneOf{Values: []string{samplePrompt}}},
}

// SetVerifyMode updates one group's challenge mode or restores its configured baseline.
func (v *Service) SetVerifyMode(groupID int64, mode string) error {
	return v.updateGroupSettings(groupID, func(_ store.GroupView, overrides *store.GroupOverrides) {
		if mode == "" {
			overrides.VerifyMode = nil
			return
		}
		overrides.VerifyMode = &mode
	})
}

// EffectiveMode returns one group's current challenge mode.
func (v *Service) EffectiveMode(groupID int64) string {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return config.ModeKernel
	}
	return group.VerifyMode().Value
}

func (v *Service) questions(groupID int64) []config.Question {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return nil
	}
	return group.Questions().Value
}

// Mixed mode uses a cryptographic coin flip; an empty quiz pool falls back to kernel.
func (v *Service) pickMode(gid int64) string {
	mode := v.EffectiveMode(gid)
	if mode == (config.ModeMixed) {
		mode = (config.ModeQuiz)
		if cryptoIntn(2) == 0 {
			mode = (config.ModeKernel)
		}
	}
	if mode == (config.ModeQuiz) && len(v.questions(gid)) == 0 {
		return config.ModeKernel
	}
	return mode
}

// Kernel challenges have no options and use correctIdx -1.
func (v *Service) newChallenge(gid int64, ul i18n.Lang) (mode, text string, opts []string, correctIdx int) {
	mode = v.pickMode(gid)
	if mode == (config.ModeKernel) {
		return mode, tgfmt.KernelQuestion(v.messages, ul), nil, -1
	}
	text, opts, correctIdx = shuffledQuestion(randomQuestion(v.questions(gid)))
	return mode, text, opts, correctIdx
}

// Operator fallback questions override the localized built-in questions.
func (v *Service) fallbackQuestion(groupID int64, l i18n.Lang) (string, []string) {
	var questions []config.ShortQuestion
	if group, ok := v.groupSettings(groupID); ok && !group.FallbackBuiltin().Value {
		questions = group.FallbackQuestions().Value
	}
	if len(questions) != 0 {
		question := questions[cryptoIntn(len(questions))]
		return question.Q, question.Answers
	}
	builtin := v.messages.Verification.Challenge.BuiltinFallback()
	return builtin[cryptoIntn(len(builtin))].For(l)
}

// answersAnotherFallback reports that this reply is the right answer to a fallback question the
// same applicant was given for a different group.
func (v *Service) answersAnotherFallback(gid, uid int64, text string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		if k.uid != uid || k.gid == gid || p.done || len(p.fbAnswers) == 0 {
			continue
		}
		if (rules.OneOf{Values: p.fbAnswers}).MatchesAnswer(text) {
			return true
		}
	}
	return false
}

// sharedFallbackQuestion reuses a fallback already drawn for this applicant in another group when
// both groups draw from the same source, so the common case never diverges in the first place.
func (v *Service) sharedFallbackQuestion(gid, uid int64, l i18n.Lang) (string, []string) {
	if reuseText, reuseAnswers, ok := v.drawnFallback(gid, uid); ok {
		return reuseText, reuseAnswers
	}
	return v.fallbackQuestion(gid, l)
}

func (v *Service) drawnFallback(gid, uid int64) (string, []string, bool) {
	source := v.fallbackSource(gid)
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		if k.uid != uid || k.gid == gid || p.done || len(p.fbAnswers) == 0 || p.qText == "" {
			continue
		}
		if !sameFallbackSource(source, v.fallbackSource(k.gid)) {
			continue // another group's own question bank is not this group's to reuse
		}
		return p.qText, append([]string(nil), p.fbAnswers...), true
	}
	return "", nil, false
}

// fallbackSource returns nil for the built-in bank, or the group's configured questions.
func (v *Service) fallbackSource(groupID int64) []config.ShortQuestion {
	group, ok := v.groupSettings(groupID)
	if !ok || group.FallbackBuiltin().Value {
		return nil
	}
	return group.FallbackQuestions().Value
}

func sameFallbackSource(a, b []config.ShortQuestion) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Q != b[i].Q || len(a[i].Answers) != len(b[i].Answers) {
			return false
		}
		for j := range a[i].Answers {
			if a[i].Answers[j] != b[i].Answers[j] {
				return false
			}
		}
	}
	return true
}

// No-Linux phrases switch to a fallback without consuming an attempt.
// Detect other operating systems before version parsing so their build numbers cannot pass.
func mentionsOtherOS(text string) bool {
	text = rules.Normalize(text)
	for _, l := range i18n.Languages() {
		for _, phrase := range i18n.Messages.Verification.Input.OtherOSPhrases.For(l) {
			if (rules.Contains{Value: phrase}).MatchesNormalized(text) {
				return true
			}
		}
	}
	return false
}

// No-Linux declarations receive the fallback rather than a strike.
func saysNoLinux(text string) bool {
	text = rules.Normalize(text)
	for _, l := range i18n.Languages() {
		for _, phrase := range i18n.Messages.Verification.Input.NoLinuxPhrases.For(l) {
			if (rules.Contains{Value: phrase, CompactWhitespace: true}).MatchesNormalized(text) {
				return true
			}
		}
	}
	return false
}

// Route DMs only after the current kernel or fallback question was confirmed delivered.
func (v *Service) hasKernelPending(uid int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		// Before prompting, the applicant may have seen only the channel-follow step.
		if k.uid == uid && !p.done && p.mode == config.ModeKernel && p.prompted && !p.fallbackPending {
			return true
		}
	}
	return false
}

// One DM answer settles all simultaneously pending groups with confirmed prompts.
func (v *Service) kernelPendingGroups(uid int64) []int64 {
	v.mu.Lock()
	defer v.mu.Unlock()
	var gids []int64
	for k, p := range v.pend {
		if k.uid == uid && !p.done && p.mode == config.ModeKernel && p.prompted && !p.fallbackPending {
			gids = append(gids, k.gid)
		}
	}
	return gids
}

// KernelAnswerDM reports whether a private text message should be graded as a kernel answer.
func (v *Service) KernelAnswerDM(userID int64, text string, private bool) bool {
	if !private {
		return false
	}
	if text = strings.TrimSpace(text); text == "" || strings.HasPrefix(text, "/") {
		return false
	}
	return v.hasKernelPending(userID)
}

// OnKernelAnswer grades one private kernel or fallback answer.
func (v *Service) OnKernelAnswer(ctx *HandlerContext, update Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return nil
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	uid := msg.From.ID
	// Classify once: a model version could otherwise trip one group and pass another as a kernel.
	// One reply also records only one tally entry.
	if gid, nonce, tripped := v.trippedPending(uid, msg.Text); tripped {
		v.declineAgent(c, bot, gid, uid, nonce, msg.Text)
		for _, other := range v.kernelPendingGroups(uid) {
			if other != gid {
				v.declineAgent(c, bot, other, uid, "", msg.Text) // same reply, same verdict, no second tally
			}
		}
		return nil
	}
	for _, gid := range v.kernelPendingGroups(uid) {
		v.gradeKernelAnswer(c, bot, gid, uid, msg.Text)
	}
	return nil
}

// A nonce-derived tripwire can match at most one pending.
func (v *Service) trippedPending(uid int64, text string) (gid int64, nonce string, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		if k.uid == uid && !p.done && p.mode == config.ModeKernel && p.prompted && !p.fallbackPending &&
			rules.AgentReply(text, p.nonce) {
			return k.gid, p.nonce, true
		}
	}
	return 0, "", false
}

// Decline every affected group, but tally the one reply only once.
func (v *Service) declineAgent(c context.Context, bot Gateway, gid, uid int64, nonce, text string) {
	ul, cur, _, gate, ok := v.kernelPendingInfo(gid, uid)
	if !ok {
		return
	}
	if nonce != "" {
		model, total := v.recordAgent(text)
		log.Printf("verify: automated-agent tripwire triggered by %d in %d (model %q, %d total) — declining", uid, gid, model, total)
		alert := v.messages.Verification.Admin.AgentCaught.Render(v.groupLanguage(gid), uid, gid, model, total)
		v.adminRecord(c, bot, alert)
	} else {
		log.Printf("verify: declining %d in %d — the same reply tripped the tripwire in another group", uid, gid)
	}
	outcome, banned := v.decline(c, bot, gid, uid, cur, wrongAnswerReason)
	if outcome == declineNoPending {
		return
	}
	msg := v.declineResultText(outcome, ul, gate, func() string { return v.agentCaughtText(gid, ul, gate, banned) })
	_, _ = sendText(c, bot, uid, msg)
}

// A plausible version passes after the channel gate; the final failed reply declines.
func (v *Service) gradeKernelAnswer(c context.Context, bot Gateway, gid, uid int64, text string) {
	ul, nonce, fbAnswers, gate, ok := v.kernelPendingInfo(gid, uid)
	if !ok {
		return // handled or replaced meanwhile
	}
	groupLang := v.groupLanguage(gid)
	challenge := &v.messages.Verification.Challenge
	// Tripwire compliance declines immediately and counts as a normal failed verification.
	if rules.AgentReply(text, nonce) {
		v.declineAgent(c, bot, gid, uid, nonce, text)
		return
	}
	verdict := kernelAnswerRule.Evaluate(text)
	// Guard every acceptance path from the prompt's impossible example; only the first copy is free.
	if verdict == rules.Rejected && v.markSampleBounced(gid, uid, nonce) {
		v.save()
		_, _ = sendHTML(c, bot, uid, challenge.SampleCopied.For(ul), nil)
		return
	}
	// Fallback answers are authoritative, but a real kernel remains acceptable.
	if len(fbAnswers) > 0 {
		if (rules.OneOf{Values: fbAnswers}).MatchesAnswer(text) || (verdict == rules.Accepted && !mentionsOtherOS(text)) {
			v.finishKernelPass(c, bot, gid, uid, nonce, ul, groupLang)
			return
		}
		// One reply lands in every group this applicant is verifying for. When the groups drew
		// different fallback questions, a reply that correctly answers one of the others is not
		// a wrong answer here — it just is not this group's turn yet.
		if v.answersAnotherFallback(gid, uid, text) {
			log.Printf("verify: %d answered another group's fallback question; not charging a try in %d", uid, gid)
			return
		}
		left, curNonce, ok := v.recordKernelTry(gid, uid, nonce)
		if !ok {
			return
		}
		if left > 0 {
			v.save()
			_, _ = sendHTML(c, bot, uid, heldOr(gate, challenge.FallbackWrongHeld, challenge.FallbackWrong).Render(ul, left), nil)
			return
		}
		// Decline only the nonce charged by recordKernelTry, never a replacement pending.
		outcome, banned := v.decline(c, bot, gid, uid, curNonce, wrongAnswerReason)
		if outcome == declineNoPending {
			return
		}
		msg := v.declineResultText(outcome, ul, gate, func() string { return v.wrongAnswerText(gid, ul, gate, banned) })
		_, _ = sendText(c, bot, uid, msg)
		return
	}
	// Give WSL or VM users one free clarification before accepting the same real kernel.
	if mentionsOtherOS(text) && verdict == rules.Accepted && v.markOSClarified(gid, uid, nonce) {
		v.save()
		_, _ = sendHTML(c, bot, uid, challenge.OSMixed.For(ul), nil)
		return
	}
	if verdict != rules.Accepted { // another system's build number is not a kernel version
		// Offer the answer-hidden short question once and without charging an attempt.
		// It remains a typed-knowledge gate, not a click path.
		if saysNoLinux(text) || mentionsOtherOS(text) {
			// The current minute proves the advertised escape is not a canned reply.
			// One malformed attempt gets a free format reminder.
			if !rules.MinuteProof(text, time.Now()) {
				if v.markNoLinuxReminded(gid, uid, nonce) {
					_, _ = sendHTML(c, bot, uid, challenge.NoLinuxRetry.For(ul), nil)
					return
				}
			} else {
				qText, answers := v.sharedFallbackQuestion(gid, uid, ul)
				if v.beginKernelFallback(bot, gid, uid, nonce, qText, answers) {
					v.save()
					prompt, current := v.pendingDMChallenge(gid, uid, nil)
					if !current {
						return
					}
					result, _ := v.sendDMQuestion(c, bot, uid, prompt)
					if result.stateChanged {
						v.save()
					}
					if !result.current {
						v.deleteChallenge(c, bot, uid, result.messageID)
					}
					return
				}
			}
		}
		left, curNonce, ok := v.recordKernelTry(gid, uid, nonce)
		if !ok {
			return
		}
		if left > 0 {
			v.save() // keep the used-up tries across a restart
			_, _ = sendHTML(c, bot, uid, heldOr(gate, challenge.KernelWrongHeld, challenge.KernelWrong).Render(ul, left), nil)
			return
		}
		outcome, banned := v.decline(c, bot, gid, uid, curNonce, wrongAnswerReason) // the nonce as of the charge, see above
		if outcome == declineNoPending {
			return
		}
		msg := v.declineResultText(outcome, ul, gate, func() string { return v.wrongAnswerText(gid, ul, gate, banned) })
		_, _ = sendText(c, bot, uid, msg)
		return
	}
	v.finishKernelPass(c, bot, gid, uid, nonce, ul, groupLang)
}

// Nonce-bind approval across the channel lookup so a stale answer cannot settle a replacement.
func (v *Service) finishKernelPass(c context.Context, bot Gateway, gid, uid int64, nonce string, ul, groupLang i18n.Lang) {
	channel := &v.messages.Verification.Channel
	voice := v.voice(v.pendingGate(gid, uid))
	if !v.isChannelMember(c, bot, gid, uid, groupLang) {
		message := channel.First.Render(ul, v.channelLinkHTML(gid, ul))
		_, _ = sendHTML(c, bot, uid, message, nil)
		return
	}
	p, ok := v.claimPendingNonce(gid, uid, nonce)
	if ok && v.executeApprove(c, bot, gid, uid, p) == approveConfirmed {
		_, _ = sendText(c, bot, uid, voice.Passed.For(ul))
		return
	}
	_, _ = sendText(c, bot, uid, voice.AlreadyHandled.For(ul))
}

// Return only live, confirmed pending data needed for grading.
func (v *Service) kernelPendingInfo(gid, uid int64) (ul i18n.Lang, nonce string, fbAnswers []string, gate string, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, exists := v.pend[pkey{gid, uid}]
	if !exists || p.done || p.fallbackPending {
		return i18n.LangZH, "", nil, gateRequest, false
	}
	return p.lang, p.nonce, p.fbAnswers, p.gate, true
}

// heldOr picks the wording that matches where the applicant is standing.
func heldOr(gate string, held, request i18n.Format) i18n.Format {
	if gate == gateMute {
		return held
	}
	return request
}

// Prepare a hidden fallback and suspend grading until its prompt delivery is confirmed.
func (v *Service) beginKernelFallback(bot Gateway, gid, uid int64, nonce, question string, answers []string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done || p.nonce != nonce || p.hinted || p.fallbackPending {
		return false
	}
	p.hinted = true
	p.qText = question
	p.fbAnswers = append([]string(nil), answers...)
	p.fallbackPending = true
	if p.timer != nil {
		p.timer.Stop()
	}
	p.deadline = v.wallNow().Add(pendingDeliveryTimeout)
	v.armExpiry(bot, p, gid, uid, pendingDeliveryTimeout, challengeExpiryReason(false))
	return true
}

// A malformed no-Linux declaration receives only one free reminder.
func (v *Service) markNoLinuxReminded(gid, uid int64, nonce string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done || p.nonce != nonce || p.noLinuxReminded {
		return false // gone, handled, or a newer request now holds this key
	}
	p.noLinuxReminded = true
	return true
}

// Clarify a mixed OS/kernel reply once rather than looping a valid WSL user toward a ban.
func (v *Service) markOSClarified(gid, uid int64, nonce string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done || p.nonce != nonce || p.osClarified {
		return false
	}
	p.osClarified = true
	return true
}

// The copied-example nudge is free only once.
func (v *Service) markSampleBounced(gid, uid int64, nonce string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done || p.nonce != nonce || p.sampleBounced {
		return false
	}
	p.sampleBounced = true
	return true
}

// Return the nonce charged with the failed reply so decline cannot claim a replacement.
func (v *Service) recordKernelTry(gid, uid int64, want string) (left int, nonce string, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, exists := v.pend[pkey{gid, uid}]
	if !exists || p.done || p.nonce != want {
		return 0, "", false // a newer request replaced this pending: never charge IT for an old reply
	}
	p.tries++
	left = kernelMaxTries - p.tries
	if left < 0 {
		left = 0
	}
	return left, p.nonce, true
}
