package verification

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// Tallies self-reported models from the kernel challenge's agent tripwire.
// Claims are untrusted usage data, never evidence.

// Unknown models fold into "other" after this many distinct keys.
const agentModelMax = 200

// Only model-ID characters may reach logs or persisted state.
var modelValue = regexp.MustCompile(`[^0-9A-Za-z.:_/+-]+`)

// modelDeclare matches the explicit form requested by the tripwire.
var modelDeclare = regexp.MustCompile(`(?i)\bmodel\s*[=:]\s*([0-9A-Za-z][0-9A-Za-z.:_/+-]*)`)

// Prose replies are normalized to these families. Longest matches win.
var modelFamilies = []string{
	// western
	"claude", "sonnet", "opus", "haiku", "chatgpt", "gpt", "openai", "o3", "o4", "gemini", "gemma",
	"bard", "grok", "llama", "mistral", "mixtral", "command-r", "cohere", "copilot", "perplexity",
	"sonar", "phi",
	// chinese / low-cost, the ones cheap spam bots actually run
	"deepseek", "qwen", "tongyi", "kimi", "moonshot", "chatglm", "glm", "zhipu", "doubao", "hunyuan",
	"ernie", "wenxin", "spark", "minimax", "abab", "baichuan", "internlm", "yi", "step", "skywork",
	"telechat", "sensechat",
	// hosting layers an agent may name instead of a model
	"ollama", "openrouter", "groq", "together", "siliconflow",
}

// Whole-word matching prevents short names such as "yi" from matching inside other words.
var familyRe *regexp.Regexp

func init() {
	sort.Slice(modelFamilies, func(i, j int) bool {
		if len(modelFamilies[i]) != len(modelFamilies[j]) {
			return len(modelFamilies[i]) > len(modelFamilies[j]) // "chatgpt" must win over "gpt"
		}
		return modelFamilies[i] < modelFamilies[j]
	})
	familyRe = regexp.MustCompile(`(?i)\b(` + strings.Join(modelFamilies, "|") + `)\b`)
}

// claimedModel extracts and sanitizes an explicit model ID or recognized family.
func claimedModel(text string) string {
	if m := modelDeclare.FindStringSubmatch(text); len(m) == 2 {
		return sanitizeModel(m[1])
	}
	if f := familyRe.FindString(text); f != "" {
		return strings.ToLower(f)
	}
	return "unknown"
}

func sanitizeModel(s string) string {
	s = strings.ToLower(strings.TrimSpace(modelValue.ReplaceAllString(s, "")))
	if s == "" {
		return "unknown"
	}
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

// recordAgent persists one tripwire result and returns its model and the new total.
func (v *Service) recordAgent(text string) (model string, total int) {
	model = claimedModel(text)
	v.agentMu.Lock()
	if v.agents.Counts == nil {
		v.agents.Counts = map[string]int{}
	}
	if _, known := v.agents.Counts[model]; !known && len(v.agents.Counts) >= agentModelMax {
		model = "other"
	}
	v.agents.Counts[model]++
	v.agents.Total++
	total = v.agents.Total
	v.agentMu.Unlock()
	v.saveAgents()
	return model, total
}

func (v *Service) agentSnapshot() AgentTally {
	v.agentMu.Lock()
	defer v.agentMu.Unlock()
	return AgentTally{Total: v.agents.Total, Counts: copyCounts(v.agents.Counts)}
}

func (v *Service) saveAgents() {
	if v.agentPath == "" || v.stateStore == nil || !v.agentWritable {
		return
	}
	if err := retryStoreWrite(v.rollbackObserver, func() error {
		return v.stateStore.SaveAgents(v.agentPath, v.agentSnapshot)
	}); err != nil {
		log.Printf("verification: save automated-agent tally: %v", err)
	}
}

// copyCounts isolates the persisted snapshot from later increments.
func copyCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, n := range m {
		out[k] = n
	}
	return out
}

// Missing state restores as an empty tally; read failures disable snapshot writes.
func (v *Service) loadAgents() error {
	if v.agentPath == "" || v.stateStore == nil {
		return nil
	}
	t, err := v.stateStore.LoadAgents(v.agentPath)
	if err != nil {
		v.agentWritable = false
		if errors.Is(err, ErrStoreReadOnly) {
			v.agentPath = ""
		}
		return fmt.Errorf("load automated-agent tally: %w", err)
	}
	v.agentWritable = true
	v.agentMu.Lock()
	v.agents = t
	if v.agents.Counts == nil {
		v.agents.Counts = map[string]int{}
	}
	v.agentMu.Unlock()
	if t.Total > 0 {
		log.Printf("restored automated-agent tally: %d total across %d model(s)", t.Total, len(t.Counts))
	}
	return nil
}

// AgentStatsText returns the six busiest claimed models or an empty string before the first catch.
func (v *Service) AgentStatsText(l i18n.Lang) string {
	v.agentMu.Lock()
	total := v.agents.Total
	counts := copyCounts(v.agents.Counts)
	v.agentMu.Unlock()
	if total == 0 {
		return ""
	}
	type kv struct {
		model string
		n     int
	}
	list := make([]kv, 0, len(counts))
	for m, n := range counts {
		list = append(list, kv{m, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].model < list[j].model
	})
	if len(list) > 6 { // keep the line short; the state file holds the full breakdown
		list = list[:6]
	}
	parts := make([]string, 0, len(list))
	for _, e := range list {
		parts = append(parts, fmt.Sprintf("%s %d", e.model, e.n))
	}
	return v.messages.Verification.Admin.AgentStats.Render(l, total, strings.Join(parts, "、"))
}

// vfailRec drives both retry cooldowns and automatic bans.
type vfailRec struct {
	count int
	last  time.Time
}

// JSON uses a slice because pkey cannot be an object key.

// Bound retained strike and cooldown state under an exceptional identity flood.
const vfailMax = 50000

// Only sustained failures within this rolling window accumulate toward a ban.
const verifyFailWindow = 6 * time.Hour

// Caller holds v.mu. A record remains live while either its strike window or its group's retry
// cooldown remains active.
func (v *Service) pruneVerifyFailsLocked(now time.Time) {
	retryByGroup := make(map[int64]time.Duration)
	for key, rec := range v.vfail {
		if rec == nil || rec.count <= 0 {
			delete(v.vfail, key)
			continue
		}
		elapsed := now.Sub(rec.last)
		if elapsed < verifyFailWindow {
			continue
		}
		retry, known := retryByGroup[key.gid]
		if !known {
			if seconds := v.verifyRetrySeconds(key.gid); seconds > 0 {
				if duration, ok := settings.SecondsToDuration(seconds); ok {
					retry = duration
				}
			}
			retryByGroup[key.gid] = retry
		}
		if retry <= 0 || elapsed >= retry {
			delete(v.vfail, key)
		}
	}
}

// Caller holds v.mu. Remove the oldest records until the requested size is reached.
func (v *Service) evictOldestVerifyFailsLocked(target int) {
	remove := len(v.vfail) - target
	if remove <= 0 {
		return
	}
	if remove == 1 {
		var oldestKey pkey
		var oldest time.Time
		found := false
		for key, rec := range v.vfail {
			if !found || rec.last.Before(oldest) {
				oldestKey, oldest, found = key, rec.last, true
			}
		}
		if found {
			delete(v.vfail, oldestKey)
		}
		return
	}
	type entry struct {
		key  pkey
		last time.Time
	}
	entries := make([]entry, 0, len(v.vfail))
	for key, rec := range v.vfail {
		entries = append(entries, entry{key: key, last: rec.last})
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].last.Equal(entries[j].last) {
			return entries[i].last.Before(entries[j].last)
		}
		if entries[i].key.gid != entries[j].key.gid {
			return entries[i].key.gid < entries[j].key.gid
		}
		return entries[i].key.uid < entries[j].key.uid
	})
	for _, item := range entries[:remove] {
		delete(v.vfail, item.key)
	}
}

func (v *Service) loadVerifyFails() error {
	if v.vfailPath == "" || v.stateStore == nil {
		return nil
	}
	recs, err := v.stateStore.LoadFailures(v.vfailPath)
	if err != nil {
		v.vfailWritable = false
		if errors.Is(err, ErrStoreReadOnly) {
			v.vfailPath = ""
		}
		return fmt.Errorf("load verification failures: %w", err)
	}
	v.vfailWritable = true
	v.mu.Lock()
	for _, r := range recs {
		if r.Count > 0 {
			v.vfail[pkey{r.GroupID, r.UserID}] = &vfailRec{count: r.Count, last: time.Unix(r.Last, 0)}
		}
	}
	n := len(v.vfail)
	v.mu.Unlock()
	if n > 0 {
		log.Printf("restored %d verification-strike record(s)", n)
	}
	return nil
}

func (v *Service) saveVerifyFails() {
	if v.vfailPath == "" || v.stateStore == nil || !v.vfailWritable {
		return
	}
	err := retryStoreWrite(v.rollbackObserver, func() error {
		return v.stateStore.SaveFailures(v.vfailPath, func() []FailureRecord {
			v.mu.Lock()
			defer v.mu.Unlock()
			v.pruneVerifyFailsLocked(v.wallNow())
			v.evictOldestVerifyFailsLocked(vfailMax)
			recs := make([]FailureRecord, 0, len(v.vfail))
			for k, r := range v.vfail {
				if r.count > 0 {
					recs = append(recs, FailureRecord{
						GroupID: k.gid, UserID: k.uid, Count: r.count, Last: r.last.Unix(),
					})
				}
			}
			return recs
		})
	})
	if err != nil {
		log.Printf("verification: save verification failures: %v", err)
	}
}

// Caller holds v.mu.
func (v *Service) recordVerifyFailLocked(gid, uid int64, failedAt time.Time) int {
	v.pruneVerifyFailsLocked(failedAt)
	key := pkey{gid, uid}
	r := v.vfail[key]
	if r == nil {
		if len(v.vfail) >= vfailMax {
			v.evictOldestVerifyFailsLocked(vfailMax - 1)
		}
		r = &vfailRec{}
		v.vfail[key] = r
	}
	if r.count > 0 && failedAt.Sub(r.last) >= verifyFailWindow {
		r.count = 0
	}
	r.count++
	r.last = failedAt
	return r.count
}

// Strikes persist across restarts; a negative threshold disables automatic bans.
func (v *Service) recordVerifyFail(gid, uid int64, failedAt time.Time) (count int, ban bool) {
	v.mu.Lock()
	count = v.recordVerifyFailLocked(gid, uid, failedAt)
	v.mu.Unlock()
	v.saveVerifyFails()
	max := v.verifyMaxFails(gid)
	return count, max > 0 && count >= max
}

// Group removal and strike recording serialize on v.mu, so a cancelled settlement cannot charge a strike.
func (v *Service) recordPendingVerifyFail(gid, uid int64, p *pending, failedAt time.Time) (count int, ban, recorded bool) {
	v.mu.Lock()
	if p.removed || !v.settings.IsGroup(gid) {
		v.mu.Unlock()
		return 0, false, false
	}
	count = v.recordVerifyFailLocked(gid, uid, failedAt)
	v.mu.Unlock()
	v.saveVerifyFails()
	max := v.verifyMaxFails(gid)
	return count, max > 0 && count >= max, true
}

// Successful verification clears prior strikes.
func (v *Service) clearVerifyFails(gid, uid int64) {
	v.mu.Lock()
	_, had := v.vfail[pkey{gid, uid}]
	delete(v.vfail, pkey{gid, uid})
	v.mu.Unlock()
	if had {
		v.saveVerifyFails()
	}
}

func (v *Service) verifyMaxFails(groupID int64) int {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return 0
	}
	return group.VerifyMaxFails().Value
}

func (v *Service) verifyRetrySeconds(groupID int64) int {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return 0
	}
	return group.VerifyRetrySeconds().Value
}

// verifyCooldownRemaining returns zero when the applicant may reapply.
func (v *Service) verifyCooldownRemaining(gid, uid int64) time.Duration {
	secs := v.verifyRetrySeconds(gid)
	if secs <= 0 {
		return 0
	}
	v.mu.Lock()
	var count int
	var last time.Time
	if r := v.vfail[pkey{gid, uid}]; r != nil {
		count, last = r.count, r.last // copy under the lock — r is a pointer shared with recordVerifyFail
	}
	v.mu.Unlock()
	if count == 0 {
		return 0
	}
	cooldown, ok := settings.SecondsToDuration(secs)
	if !ok {
		return 0
	}
	if elapsed := v.wallNow().Sub(last); elapsed < cooldown {
		return cooldown - elapsed
	}
	return 0
}

func (v *Service) wallNow() time.Time {
	if v.timeNow == nil {
		return time.Now()
	}
	return v.timeNow()
}

func (v *Service) now() time.Time { return v.wallNow().In(v.loc) }

func (v *Service) recordDecision(approve bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	today := v.now().Format("2006-01-02")
	if v.statDate != today {
		v.statDate, v.approved, v.declined = today, 0, 0
	}
	if approve {
		v.approved++
	} else {
		v.declined++
	}
}

// Stats returns today's approve and decline counters in the configured statistics timezone.
func (v *Service) Stats() (date string, approved, declined int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	today := v.now().Format("2006-01-02")
	if v.statDate != today {
		return today, 0, 0
	}
	return v.statDate, v.approved, v.declined
}

func restoredChallengeNeedsRenotify(persisted, renotify bool) bool {
	return persisted && renotify
}

func (v *Service) load(bot Gateway) error {
	if v.stateUnavailable(v.statePath) {
		return nil
	}
	lastOnline, records, err := v.loadRecoveryState()
	if err != nil {
		return err
	}
	now := v.wallNow()
	downtime := recoveryDowntime(now, lastOnline)
	longOutage := downtime > outageRecovery
	refresh := make([]renotifyItem, 0)
	for _, record := range records {
		if item, renotify := v.restorePendingRecord(bot, record, now, longOutage); renotify {
			refresh = append(refresh, item)
		}
	}
	if len(records) > 0 {
		log.Printf("restored %d pending verification(s)", len(records))
	}
	v.renotifyRestored(bot, refresh, downtime)
	return nil
}

const (
	heartbeatInterval     = 25 * time.Second // how often the bot pings Telegram to confirm it is reachable
	heartbeatProbeTimeout = 10 * time.Second // per-probe timeout for the GetMe liveness call
	offlineThreshold      = 70 * time.Second // no successful contact within this => treat as offline (defer expiries)
	outageRecovery        = 90 * time.Second // an outage longer than this triggers fresh windows + a re-notify on recovery
	renotifyCap           = 30               // most applicants to re-notify per recovery, so a big backlog can't become a message storm
)

const deferredExpiryReason = "deferral-cap"

// Verification deferral is capped at 48 hours so an outage cannot hold a pending slot indefinitely.
const maxVerificationDeferral = 48 * time.Hour

// Seeded online; only stale successful contact marks the bot offline.
func (v *Service) offlineNow() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return !v.lastOnline.IsZero() && v.wallNow().Sub(v.lastOnline) > offlineThreshold
}

// Probe at expiry to cover heartbeat detection lag at outage onset.
// Tests without a probe remain reachable.
func (v *Service) reachable(c context.Context) bool {
	if v.probe == nil {
		return true
	}
	pc, cancel := context.WithTimeout(c, heartbeatProbeTimeout)
	defer cancel()
	return v.probe.Probe(pc) == nil
}

// armExpiry now records only a durable deadline. The scanner owns expiry work, so no callback
// can settle an applicant after graceful shutdown or disappear across a restart.
func (v *Service) armExpiry(_ Gateway, p *pending, _ int64, _ int64, delay time.Duration, _ string) {
	now := v.wallNow()
	if delay <= 0 {
		delay = noFaultGrace
		p.deadline = now.Add(delay)
	}
	if !p.deferredSince.IsZero() && !p.deferralCapReached {
		remaining := p.deferredSince.Add(maxVerificationDeferral).Sub(now)
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		if delay > remaining {
			delay = remaining
			p.deadline = now.Add(delay)
		}
	}
	p.epoch++
}

// Unreachable expiries receive fresh windows until the deferral cap, then short settlement retries.
// Online settlement still requires the captured nonce and epoch.
func (v *Service) onExpiry(c context.Context, bot Gateway, gid, uid int64, nonce string, epoch uint64, reason string) {
	valid, capped, newlyCapped := v.deferralCapState(gid, uid, nonce, epoch)
	if !valid {
		return
	}
	if newlyCapped {
		logDeferralCapReached(gid, uid)
	}
	if v.offlineNow() || !v.reachable(c) {
		v.deferExpiry(bot, gid, uid, nonce, epoch, reason)
		return
	}
	if capped {
		reason = deferredExpiryReason
	}
	p, ok := v.claimExpiredPending(gid, uid, nonce, epoch, reason)
	if !ok {
		return
	}
	// Somebody who never opened the direct chat never had the required-channel gate read for
	// them, so a gate that broke after the challenge went out would look like their failure.
	// Read it once here, before the timeout is charged to anyone.
	if reason == "timeout" && v.RequiredChannelID(gid) != 0 && !p.channelUnreadable {
		v.isChannelMember(c, bot, gid, uid, v.groupLanguage(gid))
	}
	if v.retryPassingExpiry(c, bot, gid, uid, p) {
		return
	}
	outcome, banned := v.finishDecline(c, bot, gid, uid, p, reason)
	if outcome == declineUnsettled && !v.claimSettlePendingNotice(gid, uid, p) {
		return // one notice per applicant: a settlement the bot keeps retrying must not DM them each round
	}
	voice := v.voice(p.gate)
	text := v.declineResultText(outcome, p.lang, p.gate, func() string {
		switch {
		case capped:
			return voice.DeferralExpired.For(p.lang)
		case reason == challengeExpiryReason(false):
			// The applicant never saw a question; telling them they ran out of time is not true.
			return voice.Undelivered.For(p.lang)
		default:
			return v.timeoutResultText(gid, uid, p.lang, p.gate, banned)
		}
	})
	_, _ = sendText(c, bot, uid, text)
}

func (v *Service) claimExpiredPending(gid, uid int64, nonce string, epoch uint64, reason string) (*pending, bool) {
	p, ok, err := v.claimPendingExpiry(gid, uid, nonce, epoch, reason)
	if err != nil {
		log.Printf("verification: claim expired challenge for group %d user %d: %v", gid, uid, err)
		return nil, false
	}
	return p, ok
}

func (v *Service) retryPassingExpiry(c context.Context, bot Gateway, gid, uid int64, p *pending) bool {
	if !p.passing {
		return false
	}
	// A passing applicant needs an admission retry, never a decline.
	if v.executeApprove(c, bot, gid, uid, p) == approveConfirmed {
		_, _ = sendText(c, bot, uid, v.voice(p.gate).Passed.For(p.lang))
	}
	return true
}

// claimSettlePendingNotice spends the one-shot "still being settled" notice for this pending.
func (v *Service) claimSettlePendingNotice(gid, uid int64, p *pending) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	cur, ok := v.pend[key]
	if !ok || cur != p || p.settlePendingSaid {
		return false
	}
	p.settlePendingSaid = true
	return v.persistPendingLocked(key, p, p.epoch)
}

// deferralCapState also claims the one-time warning marker while the pending is locked.
func (v *Service) deferralCapState(gid, uid int64, nonce string, epoch uint64) (valid, capped, newlyCapped bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || p.nonce != nonce || p.epoch != epoch {
		return false, false, false
	}
	if p.deferralCapReached {
		return true, true, false
	}
	if p.deferredSince.IsZero() || v.wallNow().Before(p.deferredSince.Add(maxVerificationDeferral)) {
		return true, false, false
	}
	p.deferralCapReached = true
	if !v.persistPendingLocked(key, p, p.epoch) {
		return false, false, false
	}
	return true, true, true
}

func logDeferralCapReached(gid, uid int64) {
	log.Printf("WARNING: verification deferral cap reached: group=%d applicant=%d; settling without a strike", gid, uid)
}

// Keep the original reason before the cap; capped requests retry settlement after no-fault grace.
func (v *Service) deferExpiry(bot Gateway, gid, uid int64, nonce string, epoch uint64, reason string) {
	now := v.wallNow()
	newlyCapped := false
	v.mu.Lock()
	key := pkey{gid, uid}
	p, ok := v.pend[key]
	if !ok || p.done || p.nonce != nonce || p.epoch != epoch {
		v.mu.Unlock()
		return
	}
	if p.deferredSince.IsZero() {
		p.deferredSince = now
	}
	delay := v.expiryDelay(gid, p.gate, reason)
	remaining := p.deferredSince.Add(maxVerificationDeferral).Sub(now)
	if p.deferralCapReached || remaining <= 0 {
		if !p.deferralCapReached {
			p.deferralCapReached = true
			newlyCapped = true
		}
		delay = noFaultGrace
		reason = deferredExpiryReason
	} else if delay > remaining {
		delay = remaining
	}
	p.deadline = now.Add(delay)
	v.armExpiry(bot, p, gid, uid, delay, reason)
	persisted := v.persistPendingLocked(key, p, epoch)
	v.mu.Unlock()
	if !persisted {
		return
	}
	if newlyCapped {
		logDeferralCapReached(gid, uid)
	}
}

// Shared challenge rendering returns zero and alerts admins on delivery failure.
func (v *Service) postGroupChallenge(c context.Context, bot Gateway, gid, uid int64, name string, l i18n.Lang, voice challengeVoice) int {
	gidStr, uidStr := strconv.FormatInt(gid, 10), strconv.FormatInt(uid, 10)
	mention := joinerLabel(uid, name, v.NameSpoilerOn(gid))
	link := ""
	if v.botUsername != "" {
		link = "https://t.me/" + v.botUsername + "?start=verify_" + gidStr
	}
	// Keep channel navigation inside the DM verification flow.
	group := &(*v.messages).Verification.Group
	admin := &(*v.messages).Verification.Admin
	channelHint := ""
	if v.RequiredChannelID(gid) != 0 {
		channelHint = group.ChannelHint.Render(l, html.EscapeString(v.channelDisplay(gid)))
	}
	linkText := ""
	if link != "" {
		linkText = group.LinkText.Render(l, link)
	}
	template := group.Body
	switch {
	case voice.gate != gateMute:
	case voice.invited:
		template = group.BodyInvited
	default:
		template = group.BodyJoined
	}
	var body string
	if voice.recovered {
		body = group.BodyRecovered.Render(l, mention, linkText, windowText(v.messages, l, voice.window), channelHint)
	} else {
		body = template.Render(l, mention, linkText, int(v.gateTimeout(gid, voice.gate)/time.Second), channelHint)
	}
	var rows [][]Button
	if link != "" {
		rows = append(rows, []Button{{Text: group.VerifyButton.For(l), URL: link}})
	}
	rows = append(rows, []Button{
		{Text: adminPassLabel(admin, l, voice.gate), CallbackData: AdminCallbackPrefix + "pass:" + gidStr + ":" + uidStr + ":" + voice.nonce},
		{Text: adminRejectLabel(admin, l, voice.gate), CallbackData: AdminCallbackPrefix + "ban:" + gidStr + ":" + uidStr + ":" + voice.nonce},
	})
	messageID, err := sendHTML(c, bot, gid, body, rows)
	if err != nil {
		log.Printf("join %d in %d: post challenge failed: %v", uid, gid, err)
		v.adminAlert(c, bot, gid, v.adminSays(voice.gate).ChallengePostFailed.Render(l, gid, uid, err))
		return 0
	}
	return messageID
}

// The administrator buttons do different things on the two gates, so they say different things.
func adminPassLabel(admin *i18n.VerificationAdminCatalog, l i18n.Lang, gate string) string {
	if gate == gateMute {
		return admin.ReleaseButton.For(l)
	}
	return admin.ApproveButton.For(l)
}

func adminRejectLabel(admin *i18n.VerificationAdminCatalog, l i18n.Lang, gate string) string {
	if gate == gateMute {
		return admin.RemoveButton.For(l)
	}
	return admin.BanButton.For(l)
}

// Persist reachability so restart recovery can estimate downtime.
func (v *Service) saveHeartbeat() {
	if v.hbPath == "" || v.stateStore == nil || !v.heartbeatWritable {
		return
	}
	v.mu.Lock()
	t := v.lastOnline
	v.mu.Unlock()
	if t.IsZero() {
		return
	}
	record := HeartbeatRecord{LastOnline: t.Unix()}
	if err := retryStoreWrite(v.rollbackObserver, func() error {
		return v.stateStore.SaveHeartbeat(v.hbPath, record)
	}); err != nil {
		log.Printf("verification: save heartbeat: %v", err)
	}
}

// Missing heartbeat state returns zero time; read failures disable heartbeat writes.
func (v *Service) loadHeartbeat() (time.Time, error) {
	if v.hbPath == "" || v.stateStore == nil {
		return time.Time{}, nil
	}
	r, err := v.stateStore.LoadHeartbeat(v.hbPath)
	if err != nil {
		v.heartbeatWritable = false
		if errors.Is(err, ErrStoreReadOnly) {
			v.hbPath = ""
		}
		return time.Time{}, fmt.Errorf("load heartbeat: %w", err)
	}
	v.heartbeatWritable = true
	if r.LastOnline == 0 {
		return time.Time{}, nil
	}
	return time.Unix(r.LastOnline, 0), nil
}

// RunHeartbeat probes the gateway until ctx is cancelled and refreshes pending challenges after outages.
func (v *Service) RunHeartbeat(ctx context.Context, probe LiveProbe) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !v.heartbeatTick(ctx, probe) && ctx.Err() != nil {
				return
			}
		}
	}
}

// Successful probes advance reachability and refresh pendings after a real outage.
func (v *Service) heartbeatTick(ctx context.Context, probe LiveProbe) bool {
	pc, cancel := context.WithTimeout(ctx, heartbeatProbeTimeout)
	err := probe.Probe(pc)
	cancel()
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("heartbeat: cannot reach Telegram (%v) — verification timeouts are paused until contact resumes", err)
		}
		return false
	}
	now := v.wallNow()
	v.mu.Lock()
	prev := v.lastOnline
	v.lastOnline = now
	v.mu.Unlock()
	v.saveHeartbeat()
	if !prev.IsZero() && now.Sub(prev) > outageRecovery {
		log.Printf("heartbeat: back online after ~%s offline — refreshing in-progress verifications", now.Sub(prev).Round(time.Second))
		v.onRecovery(ctx, v.gateway, now.Sub(prev))
	}
	return true
}

// Recovery grants each pending a fresh strike-free window without extending the deferral cap.
// Re-notification is capped and suppressed during flapping, capped settlement, or shutdown.
func (v *Service) onRecovery(c context.Context, bot Gateway, outage time.Duration) {
	now := v.wallNow()
	v.mu.Lock()
	if v.shuttingDown {
		v.mu.Unlock()
		return
	}
	var items []renotifyItem
	var newlyCapped []pkey
	refreshed := 0
	for k, p := range v.pend {
		if p == nil || p.done {
			continue
		}
		expectedEpoch := p.epoch
		delay := v.gateTimeout(k.gid, p.gate)
		reason := "recovered"
		if p.deferralCapReached || !p.deferredSince.IsZero() &&
			!now.Before(p.deferredSince.Add(maxVerificationDeferral)) {
			if !p.deferralCapReached {
				p.deferralCapReached = true
				newlyCapped = append(newlyCapped, k)
			}
			delay = noFaultGrace
			reason = deferredExpiryReason
		}
		p.deadline = now.Add(delay)
		v.armExpiry(bot, p, k.gid, k.uid, delay, reason)
		if !v.persistPendingLocked(k, p, expectedEpoch) {
			continue
		}
		refreshed++
		if p.deferralCapReached {
			continue
		}
		if !p.lastRenotify.IsZero() && now.Sub(p.lastRenotify) < delay {
			continue
		}
		p.lastRenotify = now
		items = append(items, renotifyItem{k.gid, k.uid, p.name, p.messages(), p})
	}
	v.mu.Unlock()
	for _, k := range newlyCapped {
		logDeferralCapReached(k.gid, k.uid)
	}
	if refreshed == 0 {
		return
	}
	capped := 0
	if len(items) > renotifyCap {
		capped = len(items) - renotifyCap
		items = items[:renotifyCap]
	}
	for _, it := range items {
		v.renotifyPending(c, bot, it.gid, it.uid, it.name, it.oldMessages, it.p, outage)
	}
	log.Printf("recovery: refreshed %d verification(s), re-notified %d after ~%s offline%s", refreshed, len(items), outage.Round(time.Second), capNote(capped))
}

// An administrator can settle a join request by hand while the bot is offline. Telegram has
// no way to list the requests it still holds, but confirmed membership answers the question
// that matters: the applicant is already in, so no challenge can apply to them. A failed
// lookup keeps the pending, because verification must never be skipped on uncertainty.
func (v *Service) dropIfAlreadyJoined(c context.Context, bot Gateway, gid, uid int64, p *pending, messages challengeMessages) bool {
	if p.gate == gateMute {
		// A held member is a member by definition, so membership proves nothing here. What ends
		// this verification is them leaving, and Telegram reports that as a departure the bot
		// acts on separately; recovery must keep the pending and re-notify.
		return false
	}
	if !v.isChatMember(c, bot, gid, uid) {
		return false
	}
	log.Printf("recovery: applicant %d already joined %d while the bot was offline — dropping the stale verification", uid, gid)
	v.discardPending(gid, uid, p)
	v.deleteChallenges(c, bot, gid, uid, messages)
	return true
}

// discardPending conditionally marks a challenge superseded before removing its local timer.
func (v *Service) discardPending(gid, uid int64, p *pending) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if v.pend[key] != p {
		v.releaseTerminalLocked(key, p)
		return
	}
	v.supersedePendingLocked(key, p)
}

// Re-notify without holding v.mu, replacing working challenges only after a confirmed current send.
func (v *Service) renotifyPending(
	c context.Context,
	bot Gateway,
	gid, uid int64,
	name string,
	oldMessages challengeMessages,
	p *pending,
	outage time.Duration,
) {
	if v.dropIfAlreadyJoined(c, bot, gid, uid, p, oldMessages) {
		return
	}
	ul := p.lang
	notice := v.messages.Verification.Recovery.Renotify.Render(ul, outageText(v.messages, ul, outage))
	_, _ = sendHTML(c, bot, uid, notice, nil)
	delivery := v.deliverPendingChallenge(c, bot, gid, uid, name, p)
	if !delivery.active || !delivery.delivered {
		return
	}
	v.mu.Lock()
	key := pkey{gid, uid}
	cur, current := v.pend[key]
	current = current && cur == p && !p.done
	if current {
		p.groupMsgID = delivery.messages.groupMsgID
		p.challengeDelivered = true
		current = v.persistPendingLocked(key, p, p.epoch)
	}
	v.mu.Unlock()
	if !current {
		v.deleteChallenges(c, bot, gid, uid, delivery.messages)
		return
	}
	v.deleteChallenges(c, bot, gid, uid, oldMessages)
}

// windowText renders how long the applicant now has, reusing the outage wording so a duration
// reads the same everywhere. Seconds are never the right unit for a recovery window.
func windowText(messages *i18n.Catalog, l i18n.Lang, d time.Duration) string {
	recovery := &messages.Verification.Recovery
	switch {
	case d >= time.Hour:
		return recovery.OutageHours.Render(l, int(d.Hours()))
	case d >= time.Minute:
		return recovery.OutageMinutes.Render(l, int(d.Minutes()))
	default:
		return recovery.OutageSeconds.Render(l, int(d.Seconds()))
	}
}

// Render whole seconds, minutes, or hours in the applicant's locale.
func outageText(messages *i18n.Catalog, l i18n.Lang, d time.Duration) string {
	recovery := &messages.Verification.Recovery
	switch {
	case d < time.Minute:
		return recovery.OutageSeconds.Render(l, int(d.Seconds()))
	case d < time.Hour:
		return recovery.OutageMinutes.Render(l, int(d.Minutes()))
	default:
		return recovery.OutageHours.Render(l, int(d.Hours()))
	}
}

func capNote(capped int) string {
	if capped <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%d more refreshed silently, over the re-notify cap)", capped)
}
