package verification

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"log"
	"math/big"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

const (
	// AnswerCallbackPrefix identifies applicant quiz-answer callbacks.
	AnswerCallbackPrefix = "v:" // v:<gid>:<uid>:<nonce>:<idx>
	// AdminCallbackPrefix identifies administrator verification callbacks.
	AdminCallbackPrefix = "adm:" // adm:<action>:<gid>:<uid>[:<nonce>]
	// ChannelRecheckCallbackPrefix identifies required-channel recheck callbacks.
	ChannelRecheckCallbackPrefix = "ch:" // ch:<gid>:<uid>
)

// Bound queues and mutable counters before adversarial traffic can exhaust memory.
const (
	pendingGlobalCap        = 2000
	pendingPerGroupCap      = 500
	pendingCapAlertCooldown = 10 * time.Minute
)

type pendingStartStatus uint8

const (
	pendingStarted pendingStartStatus = iota
	pendingBlockedCapacity
	pendingBlockedTerminal
	pendingDuplicateArrival
)

type pkey struct{ gid, uid int64 }

type challengeMessages struct {
	groupMsgID   int
	privateMsgID int
}

// A verification either holds a join request the applicant has not entered on yet, or holds
// someone who is already inside the group and muted until they pass. The two settle through
// entirely different Telegram calls, so every pending records which one it is.
const (
	gateRequest = ""     // default, and what every pre-existing stored record means
	gateMute    = "mute" // the applicant is already a member; passing lifts the restriction
)

type pending struct {
	gate               string // gateRequest or gateMute
	invited            bool   // somebody else added this member, so the group notice tells administrators they can vouch
	held               bool   // a verification mute is actually in place, so passing has something to lift
	holdUntil          int64  // Unix expiry of the mute this verification placed, so another one is recognisable
	passing            bool   // the applicant answered correctly; a retry must complete the approval, never decline
	groupMsgID         int
	privateMsgID       int
	challengeDelivered bool
	mode               string    // challenge type this applicant got: settings.ModeKernel (typed answer) or settings.ModeQuiz (buttons)
	lang               i18n.Lang // applicant locale from Telegram; every applicant message uses it
	storedLang         string    // original persisted tag retained for byte-stable legacy round trips
	preserveStoredLang bool
	qText              string
	qOpts              []string
	correctIdx         int
	tries              int      // kernel mode: replies used so far (kernelMaxTries before the decline)
	hinted             bool     // kernel mode: the "no Linux installed yet" fallback was already offered
	prompted           bool     // kernel mode: the question has actually been DM'd, so a reply can be graded as an answer
	fallbackPending    bool     // fallback question prepared but not yet confirmed delivered; replies remain ungraded
	sampleBounced      bool     // kernel mode: the "you sent back our own example" nudge was already spent
	noLinuxReminded    bool     // kernel mode: the "no-Linux replies need the current minute" reminder was already spent
	osClarified        bool     // kernel mode: the "you named another OS but sent a real kernel version" clarification was already spent
	fbAnswers          []string // kernel mode: once the short-answer fallback replaced the kernel question, the answers it is graded against
	nonce              string   // per-pending token; a quiz button only counts if its nonce matches
	name               string   // applicant display name, kept so a post-outage re-notify can address them
	deadline           time.Time
	deferredSince      time.Time      // first unreachable expiry; retained across recovery and restart
	epoch              uint64         // bumped on every durable deadline replacement so a stale scanner claim cannot settle a newer window
	claimedState       ChallengeState // terminal state claimed in storage while its gateway action runs
	persistedPath      string         // store namespace containing this pending; empty means a test-seeded record
	lastRenotify       time.Time      // last post-outage re-notify, so repeated recoveries don't re-message the same applicant every cycle
	failedAt           time.Time      // claim time for rolling-window strike accounting
	deferralCapReached bool           // persisted so the operator warning and short settlement retries survive restart
	settleFailures     int            // consecutive unconfirmed settlements; persisted so the retry stays bounded across restarts
	settlePendingSaid  bool           // the "still being settled" notice was already sent; retries stay silent
	channelUnreadable  bool           // the last required-channel reading failed, so a failure here is not the applicant's
	actionID           string         // durable action created with the terminal transition
	actionOwner        string         // worker lease owner assigned in that same transition
	actionAttempts     int            // failed durable executions observed by this process
	done               bool
	removed            bool // group removal cancels an in-flight settlement without charging the applicant
}

func (p *pending) persistedLang() string {
	if p.preserveStoredLang {
		return p.storedLang
	}
	return p.lang.String()
}

func (p *pending) messages() challengeMessages {
	return challengeMessages{groupMsgID: p.groupMsgID, privateMsgID: p.privateMsgID}
}

// renotifyItem is one applicant to re-notify after an outage — snapshotted under the lock, then
// messaged outside it. Shared by the runtime-recovery (onRecovery) and restart-recovery (load) paths.
type renotifyItem struct {
	gid, uid    int64
	name        string
	oldMessages challengeMessages
	p           *pending
}

// Per-pending randomness makes stale quiz buttons unable to answer replacements.
func newNonce() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36) // fallback; uniqueness is what matters
	}
	return hex.EncodeToString(b[:])
}

// Service owns verification challenges, pending settlement, recovery, and verification state.
type Service struct {
	cfg               *settings.Config
	botUsername       string
	botID             int64
	statePath         string
	loc               *time.Location
	messages          *i18n.Catalog
	mu                sync.Mutex
	pend              map[pkey]*pending
	terminal          map[pkey]*pending // claimed terminal actions remain here until their gateway call returns
	shuttingDown      bool              // set at graceful shutdown; consumeNonce refuses so a firing timeout cannot settle after the freeze
	statDate          string
	approved          int
	declined          int
	chanAlert         map[int64]time.Time
	pendingCapAlertAt time.Time
	challengeAt       map[pkey]time.Time
	vfail             map[pkey]*vfailRec
	vfailPath         string
	vfailWritable     bool
	agentMu           sync.Mutex
	agents            AgentTally
	agentPath         string
	agentWritable     bool
	settings          *settings.Store
	gateway           Gateway
	stateStore        Store
	actionOwner       string
	lastOnline        time.Time
	hbPath            string
	heartbeatWritable bool
	probe             LiveProbe
	passed            map[pkey]time.Time
	timeNow           func() time.Time
}

func loadStatsLoc(name string) *time.Location {
	if name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return time.FixedZone("UTC+8", 8*3600)
}

// Identity is the verified bot identity used in challenge links and access checks.
type Identity struct {
	// ID is the bot's user ID.
	ID int64
	// Username is the bot username without a leading at sign.
	Username string
}

// New constructs verification with explicit state, transport, configuration, catalogue, and
// liveness dependencies.
func New(
	settings *settings.Store,
	gateway Gateway,
	stateStore Store,
	cfg *settings.Config,
	messages *i18n.Catalog,
	probe LiveProbe,
	identity Identity,
	stateDir string,
) (*Service, error) {
	v := newService(settings, gateway, cfg, messages)
	v.stateStore = stateStore
	v.botID = identity.ID
	v.botUsername = identity.Username
	v.probe = probe
	if stateDir == "" || stateStore == nil {
		return v, nil
	}
	v.hbPath = filepath.Join(stateDir, "heartbeat.json")
	v.statePath = filepath.Join(stateDir, "pending.json")
	v.vfailPath = filepath.Join(stateDir, "verifyfail.json")
	v.agentPath = filepath.Join(stateDir, "agents.json")
	if err := v.loadVerifyFails(); err != nil {
		return nil, fmt.Errorf("restore verification failures: %w", err)
	}
	if err := v.loadAgents(); err != nil {
		return nil, fmt.Errorf("restore automated-agent tally: %w", err)
	}
	if err := v.load(gateway); err != nil {
		return nil, fmt.Errorf("restore pending verifications: %w", err)
	}
	return v, nil
}

func newService(settings *settings.Store, gateway Gateway, cfg *settings.Config, messages *i18n.Catalog) *Service {
	if settings == nil {
		panic("verification: settings must not be nil")
	}
	return &Service{
		cfg:               cfg,
		loc:               loadStatsLoc(cfg.StatsTimezone),
		messages:          messages,
		pend:              make(map[pkey]*pending),
		terminal:          make(map[pkey]*pending),
		chanAlert:         map[int64]time.Time{},
		challengeAt:       map[pkey]time.Time{},
		vfail:             map[pkey]*vfailRec{},
		vfailWritable:     true,
		agentWritable:     true,
		heartbeatWritable: true,
		settings:          settings,
		gateway:           gateway,
		actionOwner:       "verification-" + newNonce(),
		lastOnline:        time.Now(),
		timeNow:           time.Now,
	}
}

func (v *Service) gatewayFor(gateway Gateway) Gateway {
	if gateway != nil {
		return gateway
	}
	return v.gateway
}

func (v *Service) groupSettings(groupID int64) (settings.GroupView, bool) {
	return v.settings.Settings(groupID)
}

// FirstChatID returns a deterministic chat for legacy DM-only status commands.
func (v *Service) FirstChatID() int64 {
	chatIDs := v.settings.ChatIDs()
	if len(chatIDs) == 0 {
		return 0
	}
	return chatIDs[0]
}

func (v *Service) updateGroupSettings(groupID int64, update func(settings.GroupView, *settings.GroupOverrides)) error {
	group, ok := v.settings.Settings(groupID)
	if !ok {
		return fmt.Errorf("%w: %d", settings.ErrUnknownGroup, groupID)
	}
	overrides := group.Overrides()
	update(group, &overrides)
	_, err := v.settings.Update(groupID, group.Revision(), overrides)
	return err
}

// SetAutoDelete updates one group's lookup cleanup policy.
func (v *Service) SetAutoDelete(groupID int64, ttl time.Duration, on bool) error {
	return v.updateGroupSettings(groupID, func(group settings.GroupView, overrides *settings.GroupOverrides) {
		if ttl <= 0 && on && group.LookupTTLSeconds().Value <= 0 {
			ttl = 3 * time.Minute
		}
		if ttl > 0 {
			seconds := int(ttl / time.Second)
			overrides.LookupTTLSeconds = &seconds
		}
		overrides.LookupAutoDeleteEnabled = &on
	})
}

// DMOrGroup reports whether a chat is guarded or is a private conversation.
func (v *Service) DMOrGroup(chatID int64, private bool) bool {
	return v.settings.IsGroup(chatID) || private
}

// IsEnabled reports whether automated join verification is enabled for one group.
func (v *Service) IsEnabled(groupID int64) bool {
	group, ok := v.groupSettings(groupID)
	return ok && group.Enabled().Value
}

// DeliveryMode returns one group's effective challenge delivery mode.
func (v *Service) DeliveryMode(groupID int64) string {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return settings.DeliveryBoth
	}
	return group.DeliveryMode().Value
}

// SetEnabled updates automated join verification for one group.
func (v *Service) SetEnabled(groupID int64, enabled bool) error {
	if err := v.updateGroupSettings(groupID, func(_ settings.GroupView, overrides *settings.GroupOverrides) {
		overrides.Enabled = &enabled
	}); err != nil {
		return err
	}
	if !enabled {
		// Verifications already running would otherwise keep their timers and settle after the
		// administrator turned verification off: applicants declined, members removed, failures
		// recorded, all for a rule that no longer applies.
		v.cancelGroupVerifications(groupID)
	}
	return nil
}

// cancelGroupVerifications abandons every verification in one group without settling or striking
// it, and lifts the holds it placed. Nobody is punished for a rule that was withdrawn.
func (v *Service) cancelGroupVerifications(groupID int64) {
	type releaseTarget struct {
		uid      int64
		messages challengeMessages
		held     bool
	}
	var targets []releaseTarget
	v.mu.Lock()
	for key, p := range v.pend {
		if key.gid != groupID || p == nil {
			continue
		}
		targets = append(targets, releaseTarget{uid: key.uid, messages: p.messages(), held: p.gate == gateMute && p.held})
		p.removed = true
		v.supersedePendingLocked(key, p)
	}
	v.mu.Unlock()
	if len(targets) == 0 {
		return
	}
	bot := v.handlerBot()
	if bot == nil {
		log.Printf("verification disabled for %d: cancelled %d verification(s); no Telegram handle to clean up with", groupID, len(targets))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelCleanupTimeout)
	defer cancel()
	for _, target := range targets {
		v.deleteChallenges(ctx, bot, groupID, target.uid, target.messages)
		if target.held {
			if err := v.releaseMember(ctx, bot, groupID, target.uid, nil); err != nil {
				log.Printf("verification disabled for %d: could not lift the hold on %d: %v", groupID, target.uid, err)
			}
		}
	}
	log.Printf("verification disabled for %d: cancelled %d verification(s) without settling them", groupID, len(targets))
}

// cancelCleanupTimeout bounds the cleanup that follows switching verification off.
const cancelCleanupTimeout = 30 * time.Second

// handlerBot returns the gateway supplied at assembly time.
func (v *Service) handlerBot() Gateway {
	return v.gateway
}

// RemoveGroup cancels every verification owned by an unregistered group without settling or striking it.
func (v *Service) RemoveGroup(groupID int64) {
	v.mu.Lock()
	for key, p := range v.pend {
		if key.gid != groupID {
			continue
		}
		if p == nil {
			delete(v.pend, key)
			continue
		}
		p.removed = true
		v.supersedePendingLocked(key, p)
	}
	for key, p := range v.terminal {
		if key.gid != groupID {
			continue
		}
		if p != nil {
			p.removed = true
		}
		delete(v.terminal, key)
	}
	for key := range v.challengeAt {
		if key.gid == groupID {
			delete(v.challengeAt, key)
		}
	}
	v.mu.Unlock()
}

// ToggleRich flips rich-message delivery for one chat.
func (v *Service) ToggleRich(groupID int64) (bool, error) {
	var enabled bool
	err := v.updateGroupSettings(groupID, func(group settings.GroupView, overrides *settings.GroupOverrides) {
		enabled = !group.RichMessages().Value
		overrides.RichMessages = &enabled
	})
	return enabled, err
}

// NameSpoilerOn reports whether applicant names are hidden in one group.
func (v *Service) NameSpoilerOn(groupID int64) bool {
	group, ok := v.groupSettings(groupID)
	return ok && group.NameSpoiler().Value
}

// ToggleNameSpoiler flips applicant-name hiding for one group.
func (v *Service) ToggleNameSpoiler(groupID int64) (bool, error) {
	var enabled bool
	err := v.updateGroupSettings(groupID, func(group settings.GroupView, overrides *settings.GroupOverrides) {
		enabled = !group.NameSpoiler().Value
		overrides.NameSpoiler = &enabled
	})
	return enabled, err
}
func (v *Service) timeout(groupID int64) time.Duration {
	return v.gateTimeout(groupID, gateRequest)
}

// livePending returns the unfinished verification for this exact group and member, if any.
func (v *Service) livePending(gid, uid int64) (*pending, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done {
		return nil, false
	}
	return p, true
}

// removeDuringCooldown takes a member out again without starting a verification they are not yet
// allowed to attempt. The notice is throttled: a rejoin loop must not become a direct-message
// loop, and the removal alone already conveys the answer.
func (v *Service) removeDuringCooldown(c context.Context, bot Gateway, gid, uid int64, l i18n.Lang, wait time.Duration) {
	if _, err := v.removeMember(c, bot, gid, uid); err != nil {
		log.Printf("post-join verify: cannot remove %d from %d during cooldown: %v", uid, gid, err)
		v.settlementAlert(c, bot, gid, err, v.adminSays(gateMute).BanFailed.Render(v.groupLanguage(gid), uid, gid, err))
		return
	}
	log.Printf("post-join verify: %d rejoined %d during the failure cooldown (%s left); removed again", uid, gid, wait.Round(time.Second))
	if !v.challengeResendOK(gid, uid) {
		return
	}
	text := v.messages.Verification.Held.CooldownActive.Render(l, durationSecondsCeil(wait))
	_, _ = sendText(c, bot, uid, text)
}

// verifyInvited reports whether a member somebody else added still has to verify here.
func (v *Service) verifyInvited(groupID int64) bool {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return v.cfg.VerifyInvitedMembers()
	}
	return group.VerifyInvited().Value
}

// postJoinTimeout is the default window for someone verified after joining. An applicant waiting
// outside is watching for the challenge; a member who is already in the group may not notice it
// for a while, and the hold keeps them harmless in the meantime, so they get longer.
const postJoinTimeout = 10 * time.Minute

// gateTimeout returns the verification window. timeout_seconds describes how long an applicant
// waits outside, so the post-join window keeps its own longer default; an administrator who sets
// the timeout in the panel is making a deliberate choice and it applies to both.
func (v *Service) gateTimeout(groupID int64, gate string) time.Duration {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return 0
	}
	setting := group.TimeoutSeconds()
	if gate == gateMute && setting.Source != settings.SourceChatOverride {
		return postJoinTimeout
	}
	duration, ok := settings.SecondsToDuration(setting.Value)
	if !ok {
		return 0
	}
	return duration
}

func (v *Service) verificationBanDuration(groupID int64) int {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return 0
	}
	return group.BanSeconds().Value
}

func (v *Service) applyVerificationBan(ctx context.Context, bot Gateway, groupID, userID int64, seconds int, revoke bool) error {
	return v.gatewayFor(bot).Ban(ctx, groupID, userID, seconds, revoke)
}

// holdMember mutes a member for the whole verification window. Telegram only restricts members
// of supergroups, so a basic group cannot hold anyone: the challenge still runs, and failing it
// still removes them, but they can speak in the meantime.
func (v *Service) holdMember(ctx context.Context, bot Gateway, groupID, userID int64, supergroup bool, p *pending) {
	if !supergroup {
		return
	}
	seconds := int(v.gateTimeout(groupID, gateMute)/time.Second) + muteGraceSeconds
	until := v.wallNow().Add(time.Duration(seconds) * time.Second).Unix()
	if err := v.gatewayFor(bot).Mute(ctx, groupID, userID, seconds); err != nil {
		log.Printf("post-join verify: cannot mute %d in %d (%v); the challenge continues unheld", userID, groupID, err)
		return
	}
	if p == nil {
		return
	}
	v.mu.Lock()
	key := pkey{groupID, userID}
	if v.pend[key] == p && !p.done {
		p.held = true
		p.holdUntil = until
		v.persistPendingLocked(key, p, p.epoch)
	}
	v.mu.Unlock()
}

// muteGraceSeconds keeps the hold slightly longer than the window so a mute cannot expire in the
// instant between the deadline and the settlement that acts on it.
const muteGraceSeconds = 60

// releaseMember lifts the verification hold by restoring the group's own default permissions.
// Restoring defaults would also lift a restriction somebody else added, so the hold is only
// lifted while the one in force is still the one this verification placed.
func (v *Service) releaseMember(ctx context.Context, bot Gateway, groupID, userID int64, p *pending) error {
	if p != nil && !v.holdStillOurs(ctx, bot, groupID, userID, p) {
		log.Printf("post-join verify: the restriction on %d in %d is no longer the one verification placed; leaving it", userID, groupID)
		return nil
	}
	return v.gatewayFor(bot).Unmute(ctx, groupID, userID)
}

// holdStillOurs reports that the restriction Telegram currently reports is the verification hold.
// An unreadable answer counts as ours: failing to release would silence somebody indefinitely,
// which is worse than lifting a restriction an administrator can simply re-apply.
func (v *Service) holdStillOurs(ctx context.Context, bot Gateway, groupID, userID int64, p *pending) bool {
	v.mu.Lock()
	until := p.holdUntil
	v.mu.Unlock()
	if until == 0 {
		return true // placed before this was recorded, or by an older build
	}
	cm, err := bot.Member(ctx, groupID, userID)
	if err != nil || cm == nil {
		return true
	}
	restrictedUntil, comparable := cm.RestrictionUntil()
	if !comparable {
		return true // not restricted any more, or restricted in a shape we cannot compare
	}
	return restrictedUntil == until
}

// removeMember takes a failed applicant out of the group without keeping them out: banning and
// immediately unbanning removes them while leaving the invite link usable.
//
// The unban is the dangerous half. An administrator who banned this person moments ago would
// find their ban quietly lifted, so anyone Telegram already reports as banned or gone is left
// exactly as they are: whoever put them there outranks this settlement.
// It reports whether the member was left banned, which happens when the ban lands but the unban
// that follows it does not.
func (v *Service) removeMember(ctx context.Context, bot Gateway, groupID, userID int64) (stillBanned bool, err error) {
	if settled, known := v.alreadyOut(ctx, bot, groupID, userID); known && settled {
		log.Printf("post-join verify: %d is already out of %d; leaving that decision alone", userID, groupID)
		return false, nil
	}
	transport := v.gatewayFor(bot)
	if err := transport.Ban(ctx, groupID, userID, 0, false); err != nil {
		return false, err
	}
	if err := transport.Unban(ctx, groupID, userID, true); err != nil {
		// The removal worked, but they are still banned. Telling them to come back would be
		// false, so the caller is told to word it as a ban and operators are told to lift it.
		log.Printf("post-join verify: removed %d from %d but could not lift the ban: %v", userID, groupID, err)
		return true, nil
	}
	return false, nil
}

// alreadyOut reports that somebody is banned or has left. known=false means the answer could not
// be read, and the caller proceeds rather than guessing.
func (v *Service) alreadyOut(ctx context.Context, bot Gateway, groupID, userID int64) (out, known bool) {
	cm, err := bot.Member(ctx, groupID, userID)
	if err != nil || cm == nil {
		return false, false
	}
	switch cm.MemberStatus() {
	case MemberStatusBanned, MemberStatusLeft:
		return true, true
	default:
		return false, true
	}
}

// RequiredChannelID returns the channel applicants must join for one group.
func (v *Service) RequiredChannelID(groupID int64) int64 {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return 0
	}
	return group.RequiredChannelID().Value
}

func (v *Service) channelDisplay(groupID int64) string {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return ""
	}
	return group.ChannelDisplay().Value
}

func (v *Service) channelInviteURL(groupID int64) string {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return ""
	}
	return group.ChannelInviteURL().Value
}

func (v *Service) trustedGroups(groupID int64) []int64 {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return nil
	}
	return group.TrustedMemberGroupIDs().Value
}

func (v *Service) groupLanguage(groupID int64) i18n.Lang {
	group, ok := v.groupSettings(groupID)
	if !ok {
		return i18n.LangZH
	}
	return i18n.FromStored(group.Lang().Value)
}

func (v *Service) applicantLanguage(groupID, userID int64, telegramCode string) i18n.Lang {
	v.mu.Lock()
	defer v.mu.Unlock()
	if p := v.pend[pkey{groupID, userID}]; p != nil && !p.done {
		return p.lang
	}
	return i18n.FromTelegram(telegramCode)
}

// chatMemberState separates "confirmed not a member" from "could not read the answer".
// Settling a request the bot did not get to handle needs that distinction: an unreadable
// answer must not become a claim about where the applicant ended up.
func (v *Service) chatMemberState(c context.Context, bot Gateway, chatID, uid int64) (member, known bool) {
	cm, err := bot.Member(c, chatID, uid)
	if err != nil {
		log.Printf("exempt: get member(chat=%d user=%d): %v", chatID, uid, err)
		return false, false
	}
	if cm == nil {
		log.Printf("exempt: get member(chat=%d user=%d) returned no member", chatID, uid)
		return false, false
	}
	switch cm.MemberStatus() {
	case MemberStatusCreator, MemberStatusAdministrator, MemberStatusMember:
		return true, true
	default:
		return cm.MemberIsMember(), true
	}
}

// Membership lookup errors fail safe into normal verification, never bypass it.
func (v *Service) isChatMember(c context.Context, bot Gateway, chatID, uid int64) bool {
	member, known := v.chatMemberState(c, bot, chatID, uid)
	return known && member
}

// Confirmed members of trusted chats bypass verification and cooldowns.
// Lookup failure is untrusted and follows normal verification.
// Approval failure returns trusted=true so normal verification runs without cooldown rejection.
// trustedMember reports confirmed membership of any group this one trusts. An unreadable lookup
// is not trust, so verification proceeds.
func (v *Service) trustedMember(c context.Context, bot Gateway, gid, uid int64) bool {
	for _, src := range v.trustedGroups(gid) {
		if src == 0 || src == gid {
			continue
		}
		if v.isChatMember(c, bot, src, uid) {
			return true
		}
	}
	return false
}

func (v *Service) tryTrustedBypass(c context.Context, bot Gateway, gid, uid int64) (handled, trusted bool) {
	for _, src := range v.trustedGroups(gid) {
		if src == 0 || src == gid {
			continue // ignore a blank or self-referential entry
		}
		if !v.isChatMember(c, bot, src, uid) { // fail-closed: error / non-member / unreadable => not trusted
			continue
		}
		// Trusted membership takes priority over failure cooldown.
		if err := bot.ApproveJoin(c, gid, uid); err != nil {
			if gatewayFailureHas(err, FailureJoinRequestGone) {
				// Already settled in Telegram's own interface; a challenge now would be noise.
				log.Printf("trusted-bypass: join request from %d in %d is already gone: %v", uid, gid, err)
				return true, true
			}
			log.Printf("trusted-bypass: approve %d in %d failed (%v) — falling back to normal verification", uid, gid, err)
			alert := v.messages.Verification.Admin.TrustedBypassFailed.Render(v.groupLanguage(gid), uid, src, gid, err)
			v.adminAlert(c, bot, gid, alert)
			return false, true
		}
		v.clearVerifyFails(gid, uid) // a now-trusted member starts with a clean slate
		v.recordDecision(true)
		v.notePassed(gid, uid)
		log.Printf("verify: trusted-bypass auto-approved %d in %d (already a member of trusted group %d)", uid, gid, src)
		return true, true
	}
	return false, false
}

// Trusted membership is evaluated before the failed-applicant cooldown.
func (v *Service) joinGate(c context.Context, bot Gateway, gid, uid int64, applicantLang i18n.Lang) (done bool) {
	handled, trusted := v.tryTrustedBypass(c, bot, gid, uid)
	if handled {
		return true
	}
	if trusted {
		return false // confirmed trusted member, approve failed -> normal verification, skip the cooldown
	}
	// Early retries are declined without posting another challenge.
	if wait := v.verifyCooldownRemaining(gid, uid); wait > 0 {
		if err := bot.DeclineJoin(c, gid, uid); err != nil {
			if gatewayFailureHas(err, FailureJoinRequestGone) {
				log.Printf("verify cooldown: join request from %d in %d is already gone: %v", uid, gid, err)
				return true
			}
			log.Printf("verify cooldown: decline %d in %d failed: %v", uid, gid, err)
			v.settlementAlert(c, bot, gid, err, v.adminSays(gateRequest).DeclineFailed.Render(v.groupLanguage(gid), uid, gid, err))
			_, _ = sendText(c, bot, uid, v.messages.Verification.Result.DeclinePending.For(applicantLang))
			return true
		}
		seconds := durationSecondsCeil(v.verifyCooldownRemaining(gid, uid))
		log.Printf("verify cooldown: declined early re-apply from %d in %d (%ds left)", uid, gid, seconds)
		// The decline is the answer; repeating the explanation on every re-apply would turn a
		// determined applicant into a direct-message loop. Same throttle the post-join gate uses.
		if v.challengeResendOK(gid, uid) {
			_, _ = sendText(c, bot, uid, v.messages.Verification.Result.CooldownActive.Render(applicantLang, seconds))
		}
		return true
	}
	return false
}

// Caller holds v.mu; replacements do not grow either queue cap.
func (v *Service) pendingCapacityOKLocked(gid int64) bool {
	if len(v.pend) >= pendingGlobalCap {
		return false
	}
	groupN := 0
	for k := range v.pend {
		if k.gid == gid {
			groupN++
		}
	}
	return groupN < pendingPerGroupCap
}

// Only a confirmed challenge delivery may expire as an applicant-caused timeout.
func challengeExpiryReason(delivered bool) string {
	if !delivered {
		return "challenge-post-failed"
	}
	return "timeout"
}

// Delivery has its own no-fault deadline; the applicant's answer window starts only after confirmation.
const pendingDeliveryTimeout = 60 * time.Second

func (v *Service) expiryDelay(gid int64, gate, reason string) time.Duration {
	if reason == challengeExpiryReason(false) {
		return pendingDeliveryTimeout
	}
	return v.gateTimeout(gid, gate)
}

// Reserve capacity and persist before delivery. The open-challenge index, not the in-memory
// map, decides whether this arrival already has a live challenge.
func (v *Service) startPending(bot Gateway, gid, uid int64, p *pending) (oldMessages challengeMessages, status pendingStartStatus, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	old, replacing := v.pend[key]
	if inFlight := v.terminal[key]; inFlight != nil || replacing && old.done {
		return challengeMessages{}, pendingBlockedTerminal, nil
	}
	if !replacing && !v.pendingCapacityOKLocked(gid) {
		return challengeMessages{}, pendingBlockedCapacity, nil
	}
	if v.stateUnavailable(v.statePath) && replacing {
		// Persistence-disabled tests and legacy embeddings have no database arbiter.
		return challengeMessages{}, pendingDuplicateArrival, nil
	}
	delay := pendingDeliveryTimeout
	p.deadline = v.wallNow().Add(delay)
	v.armExpiry(bot, p, gid, uid, delay, challengeExpiryReason(false))
	inserted := true
	if !v.stateUnavailable(v.statePath) {
		record := pendingRecord(key, p)
		inserted, err = retryStoreChange(func() (bool, error) {
			return v.stateStore.InsertPending(v.statePath, record)
		})
	}
	if err != nil || !inserted {
		if err != nil {
			return challengeMessages{}, pendingStarted, err
		}
		return challengeMessages{}, pendingDuplicateArrival, nil
	}
	p.persistedPath = v.statePath
	if replacing {
		old.done = true
		oldMessages = old.messages()
	}
	v.pend[key] = p
	return oldMessages, pendingStarted, nil
}

// Start a full window after delivery while preserving no-fault status on send failure.
func (v *Service) finishPendingChallenge(
	bot Gateway,
	gid, uid int64,
	p *pending,
	messages challengeMessages,
	delivered bool,
) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if cur, ok := v.pend[key]; !ok || cur != p || p.done {
		return false
	}
	expectedEpoch := p.epoch
	p.groupMsgID = messages.groupMsgID
	p.privateMsgID = messages.privateMsgID
	p.challengeDelivered = delivered
	delay := v.gateTimeout(gid, p.gate)
	p.deadline = v.wallNow().Add(delay)
	v.armExpiry(bot, p, gid, uid, delay, challengeExpiryReason(delivered))
	return v.persistPendingLocked(key, p, expectedEpoch)
}

// One process-wide throttle prevents a multi-group flood from spamming operator alerts.
func (v *Service) alertPendingCap(c context.Context, bot Gateway, gid int64, gate string) {
	now := time.Now()
	v.mu.Lock()
	if !v.pendingCapAlertAt.IsZero() && now.Sub(v.pendingCapAlertAt) < pendingCapAlertCooldown {
		v.mu.Unlock()
		return
	}
	v.pendingCapAlertAt = now
	v.mu.Unlock()
	v.failAlert(c, bot, gid, v.adminSays(gate).PendingCap.Render(v.groupLanguage(gid), pendingGlobalCap, pendingPerGroupCap, gid))
}

type challengeDeliveryResult struct {
	messages             challengeMessages
	delivered            bool
	active               bool
	modeLabel            string
	replacedPrivateMsgID int
}

// deliverPendingChallenge is the single delivery-mode decision for initial and recovery challenges.
// gateOf reads the gate of the pending a delivery belongs to; a delivery with no owner follows
// the join-request wording, which is what a user-triggered resend of a request challenge wants.
func gateOf(owner *pending) challengeVoice {
	if owner == nil {
		return challengeVoice{gate: gateRequest}
	}
	return challengeVoice{gate: owner.gate, invited: owner.invited, nonce: owner.nonce}
}

// voiceFor is gateOf plus the one thing only the service can answer: whether this pending is
// carrying an extended window from outage recovery. Deriving that from the deadline rather than a
// flag means a restart between recovery and delivery cannot lose it.
func (v *Service) voiceFor(gid int64, owner *pending) challengeVoice {
	voice := gateOf(owner)
	if owner == nil {
		return voice
	}
	remaining := owner.deadline.Sub(v.wallNow())
	if remaining > v.gateTimeout(gid, owner.gate) {
		voice.recovered = true
		voice.window = remaining.Round(time.Minute)
	}
	return voice
}

// challengeVoice selects the public challenge wording: applying, arriving on their own, or
// being brought in by somebody who can vouch for them.
type challengeVoice struct {
	gate    string
	invited bool
	nonce   string
	// recovered marks a challenge re-posted after an outage. The applicant could not have
	// answered while the bot was down, so both the wording and the window differ.
	recovered bool
	window    time.Duration
}

// An applicant whose challenge was interrupted by an outage applied hours ago; a four-minute
// window asks them to be holding their phone at the moment the bot happens to come back. The
// failure was the bot's, so the second chance is a real one.
const recoveryWindow = 24 * time.Hour

func (v *Service) deliverPendingChallenge(
	c context.Context,
	bot Gateway,
	gid, uid int64,
	name string,
	owner *pending,
) challengeDeliveryResult {
	result := challengeDeliveryResult{active: true, modeLabel: v.DeliveryMode(gid)}
	groupLang := v.groupLanguage(gid)
	switch result.modeLabel {
	case settings.DeliveryGroup:
		result.messages.groupMsgID = v.postGroupChallenge(c, bot, gid, uid, name, groupLang, v.voiceFor(gid, owner))
		result.delivered = result.messages.groupMsgID != 0
	case settings.DeliveryBoth:
		result.messages.groupMsgID = v.postGroupChallenge(c, bot, gid, uid, name, groupLang, v.voiceFor(gid, owner))
		result.delivered = result.messages.groupMsgID != 0
		outcome := v.attemptPrivateChallenge(c, bot, gid, uid, owner)
		result.messages.privateMsgID = outcome.privateMsgID
		result.replacedPrivateMsgID = outcome.replacedPrivateMsgID
		switch outcome.result {
		case privateDelivered:
			result.delivered = true
			result.modeLabel = "both-delivered"
		case privateRejected:
			result.modeLabel = "group-private-rejected"
		case privateUncertain:
			result.modeLabel = "group-private-uncertain"
		case privateGone:
			result.active = false
			v.deleteChallenges(c, bot, gid, uid, result.messages)
		}
	case settings.DeliveryDM:
		outcome := v.attemptPrivateChallenge(c, bot, gid, uid, owner)
		result.messages.privateMsgID = outcome.privateMsgID
		result.replacedPrivateMsgID = outcome.replacedPrivateMsgID
		switch outcome.result {
		case privateDelivered:
			result.delivered = true
			result.modeLabel = "private"
		case privateRejected:
			result.modeLabel = "group-fallback"
			result.messages.groupMsgID = v.postGroupChallenge(c, bot, gid, uid, name, groupLang, v.voiceFor(gid, owner))
			result.delivered = result.messages.groupMsgID != 0
		case privateUncertain:
			result.modeLabel = "private-uncertain"
		case privateGone:
			result.active = false
		}
	}
	return result
}

// OnJoinRequest starts verification for one eligible group join request.
func (v *Service) OnJoinRequest(ctx *HandlerContext, update Update) error {
	jr := update.ChatJoinRequest
	if jr == nil || !v.settings.IsGroup(jr.Chat.ID) {
		return nil
	}
	if !v.IsEnabled(jr.Chat.ID) {
		log.Printf("verification disabled — leaving join request from %d for manual review", jr.From.ID)
		return nil
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	gid := jr.Chat.ID
	uid := jr.From.ID
	// Applicant and group surfaces resolve independently at the update boundary.
	applicantLang := i18n.FromTelegram(jr.From.LanguageCode)
	// Trusted bypass precedes cooldown enforcement.
	// Untrusted applicants then face the retry cooldown.
	if v.joinGate(c, bot, gid, uid, applicantLang) {
		return nil
	}
	mode, text, opts, correctIdx := v.newChallenge(gid, applicantLang)
	name := jr.From.DisplayName()
	p := &pending{mode: mode, lang: applicantLang, qText: text, qOpts: opts, correctIdx: correctIdx,
		nonce: newNonce(), name: name}
	oldMessages, status, err := v.startPending(bot, gid, uid, p)
	if err != nil {
		return fmt.Errorf("persist pending challenge for user %d in group %d: %w", uid, gid, err)
	}
	switch status {
	case pendingBlockedCapacity:
		log.Printf("join %d in group %d: pending cap reached; left for manual review", uid, gid)
		v.alertPendingCap(c, bot, gid, gateRequest)
		return nil
	case pendingDuplicateArrival:
		log.Printf("join %d in group %d: repeat of an arrival already challenged; keeping the challenge on screen", uid, gid)
		return nil
	case pendingBlockedTerminal:
		log.Printf("join %d in group %d: terminal action still in flight; deferred re-application", uid, gid)
		return nil
	}
	v.deleteChallenges(c, bot, gid, uid, oldMessages)

	delivery := v.deliverPendingChallenge(c, bot, gid, uid, name, p)
	if !delivery.active {
		v.deleteUnexposedPending(gid, uid, p)
		return nil
	}
	if !v.finishPendingChallenge(bot, gid, uid, p, delivery.messages, delivery.delivered) {
		v.deleteChallenges(c, bot, gid, uid, delivery.messages)
		return nil // another action handled or replaced this request while delivery was in flight
	}
	log.Printf("join %d (@%s) in group %d: pending (%s challenge), delivery=%s, group message=%d, private message=%d",
		uid, jr.From.Username, gid, mode, delivery.modeLabel, delivery.messages.groupMsgID, delivery.messages.privateMsgID)
	return nil
}

// recentPassWindow suppresses a second verification for someone the bot itself just let in:
// approving a join request produces a membership update that would otherwise read as a fresh
// arrival needing a challenge.
const recentPassWindow = 5 * time.Minute

func (v *Service) notePassed(gid, uid int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.passed == nil {
		v.passed = make(map[pkey]time.Time)
	}
	now := v.wallNow()
	for key, at := range v.passed {
		if now.Sub(at) > recentPassWindow {
			delete(v.passed, key)
		}
	}
	v.passed[pkey{gid, uid}] = now
}

func (v *Service) recentlyPassed(gid, uid int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	at, ok := v.passed[pkey{gid, uid}]
	return ok && v.wallNow().Sub(at) <= recentPassWindow
}

func (v *Service) startPostJoinChallenge(
	c context.Context,
	bot Gateway,
	gid, uid int64,
	p *pending,
) (challengeMessages, bool, error) {
	oldMessages, status, err := v.startPending(bot, gid, uid, p)
	if err != nil {
		return challengeMessages{}, false, fmt.Errorf("persist post-join challenge for user %d in group %d: %w", uid, gid, err)
	}
	switch status {
	case pendingBlockedCapacity:
		// A blocked applicant waits outside; a blocked member would stay inside unverified.
		log.Printf("post-join verify: %d in %d skipped, pending cap reached; removing them until the queue drains", uid, gid)
		v.alertPendingCap(c, bot, gid, gateMute)
		if _, removeErr := v.removeMember(c, bot, gid, uid); removeErr != nil {
			log.Printf("post-join verify: cannot remove %d from %d at capacity: %v", uid, gid, removeErr)
		}
		return challengeMessages{}, false, nil
	case pendingDuplicateArrival:
		log.Printf("join %d in group %d: repeat of an arrival already challenged; keeping the challenge on screen", uid, gid)
		return challengeMessages{}, false, nil
	case pendingBlockedTerminal:
		log.Printf("post-join verify: %d in %d skipped, terminal action still in flight", uid, gid)
		return challengeMessages{}, false, nil
	default:
		return oldMessages, true, nil
	}
}

// OnMemberJoined verifies someone who is already inside a guarded group. Groups that ask people
// to apply never reach this for their applicants — the bot's own approval is remembered — but a
// group without join approval, and anyone an administrator adds directly, both arrive here.
func (v *Service) OnMemberJoined(ctx *HandlerContext, update Update) error {
	member := update.ChatMember
	if member == nil || !joinedNow(member) {
		return nil
	}
	gid := member.Chat.ID
	user := member.NewChatMember.MemberUser()
	uid := user.ID
	if !v.settings.IsGroup(gid) || !v.IsEnabled(gid) || user.IsBot || uid == v.botID {
		return nil
	}
	if v.recentlyPassed(gid, uid) {
		return nil // the bot let them in a moment ago; that was the verification
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	applicantLang := i18n.FromTelegram(user.LanguageCode)
	if v.trustedMember(c, bot, gid, uid) {
		// Someone already inside the group needs no admitting: trusting them simply means not
		// asking. Calling approve here would fail — there is no join request — and the failure
		// would quietly put a trusted member through the challenge anyway.
		v.clearVerifyFails(gid, uid)
		v.recordDecision(true)
		v.notePassed(gid, uid)
		log.Printf("post-join verify: %d in %d is a member of a trusted group; not challenged", uid, gid)
		return nil
	}
	if v.isGroupAdmin(c, bot, gid, uid) {
		return nil // administrators are not challenged in their own group
	}
	invited := member.From.ID != uid
	if invited && !v.verifyInvited(gid) {
		log.Printf("post-join verify: %d was invited into %d and invited members are exempt here", uid, gid)
		return nil
	}
	// A member already being verified must not be re-challenged by a second arrival: rejoining
	// would otherwise post a fresh notice and question every time. Re-apply the hold, since
	// leaving the group clears it, and leave the running deadline alone so nobody can extend
	// their own window by walking in and out.
	if live, ok := v.livePending(gid, uid); ok {
		// A verification started while they were outside settles by declining a join request.
		// They are inside now, so that call would settle nothing and the member would simply
		// stay, unverified, forever. Move the running verification onto the gate that can
		// actually end it, keeping the progress they have made.
		v.adoptAsHeld(gid, uid, live)
		v.holdMember(c, bot, gid, uid, member.Chat.Type == ChatTypeSupergroup, live)
		log.Printf("post-join verify: %d entered %d while still being verified; kept the running challenge", uid, gid)
		return nil
	}
	// The bot tells a removed member how long to wait. Honour it, or that promise means nothing
	// and every rejoin costs a fresh notice and question. Somebody an administrator just added is
	// exempt: removing them seconds later would be the bot overruling the person who added them.
	// They still verify — being vouched for is not verification — they are just not thrown out
	// over an earlier failure.
	if wait := v.verifyCooldownRemaining(gid, uid); wait > 0 && !invited {
		v.removeDuringCooldown(c, bot, gid, uid, applicantLang, wait)
		return nil
	}
	supergroup := member.Chat.Type == ChatTypeSupergroup
	mode, text, opts, correctIdx := v.newChallenge(gid, applicantLang)
	name := user.DisplayName()
	p := &pending{gate: gateMute, invited: invited, mode: mode, lang: applicantLang,
		qText: text, qOpts: opts, correctIdx: correctIdx, nonce: newNonce(), name: name}
	oldMessages, started, err := v.startPostJoinChallenge(c, bot, gid, uid, p)
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	v.deleteChallenges(c, bot, gid, uid, oldMessages)
	// startPending committed the record before this externally visible hold.
	v.holdMember(c, bot, gid, uid, supergroup, p)

	delivery := v.deliverPendingChallenge(c, bot, gid, uid, name, p)
	if !delivery.active {
		v.deleteUnexposedPending(gid, uid, p)
		return nil
	}
	if !v.finishPendingChallenge(bot, gid, uid, p, delivery.messages, delivery.delivered) {
		v.deleteChallenges(c, bot, gid, uid, delivery.messages)
		return nil
	}
	log.Printf("post-join verify: %d (@%s) joined %d: pending (%s challenge), held=%v, delivery=%s",
		uid, user.Username, gid, mode, supergroup, delivery.modeLabel)
	return nil
}

// joinedNow reports a membership update where someone who was outside the group is now an
// ordinary member. Promotions, demotions and departures are not arrivals.
func joinedNow(member *ChatMemberUpdated) bool {
	if member.NewChatMember == nil || member.OldChatMember == nil {
		return false
	}
	if member.NewChatMember.MemberStatus() != MemberStatusMember {
		return false
	}
	switch member.OldChatMember.MemberStatus() {
	case MemberStatusLeft, MemberStatusBanned:
		return true
	case MemberStatusRestricted:
		return !member.OldChatMember.MemberIsMember()
	default:
		return false
	}
}

func (v *Service) hasPending(uid int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		if k.uid == uid && !p.done {
			return true
		}
	}
	return false
}

// Use one pending for legacy bare verification links; map iteration preserves the existing selection rule.
func (v *Service) firstPending(uid int64) (gid int64, ul i18n.Lang, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for k, p := range v.pend {
		if k.uid == uid && !p.done {
			return k.gid, p.lang, true
		}
	}
	return 0, 0, false
}

// Throttle each group's /start prompt independently so concurrent requests do not block one another.
func (v *Service) challengeResendOK(gid, uid int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := time.Now()
	key := pkey{gid, uid}
	if last, ok := v.challengeAt[key]; ok && now.Sub(last) < challengeResendCooldown {
		return false
	}
	if len(v.challengeAt) >= challengeResendMapMax {
		cutoff := now.Add(-challengeResendCooldown)
		for pendingKey, last := range v.challengeAt {
			if !last.After(cutoff) {
				delete(v.challengeAt, pendingKey)
			}
		}
		if len(v.challengeAt) >= challengeResendMapMax {
			v.challengeAt = map[pkey]time.Time{}
		}
	}
	v.challengeAt[key] = now
	return true
}

// Fifteen seconds limits prompt floods without materially delaying a user.
const challengeResendCooldown = 15 * time.Second

// Keep the resend throttle bounded independently of the root DM responder.
const challengeResendMapMax = 10000

type dmPrompt struct {
	gid             int64
	mode            string
	lang            i18n.Lang
	text            string
	opts            []string
	nonce           string
	tries           int
	fallback        bool
	fallbackPending bool
	pending         *pending
}

type dmSendResult struct {
	messageID         int
	replacedMessageID int
	current           bool
}

// owner binds a delivery to the pending it was started for. A recovery that took a snapshot
// must not write its message IDs into a replacement pending created meanwhile by a fresh
// application, or the replacement ends up with a full window and no visible challenge.
// A nil owner means "whatever pending is current", which is what a user-triggered resend wants.
func (v *Service) pendingDMChallenge(gid, uid int64, owner *pending) (dmPrompt, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	if !ok || p.done {
		return dmPrompt{}, false
	}
	if owner != nil && (p != owner || p.nonce != owner.nonce) {
		return dmPrompt{}, false
	}
	return dmPrompt{gid: gid, mode: p.mode, lang: p.lang, text: p.qText, opts: p.qOpts,
		nonce: p.nonce, tries: p.tries, fallback: len(p.fbAnswers) > 0,
		fallbackPending: p.fallbackPending, pending: p}, true
}

func (v *Service) pendingDMChallenges(uid int64) []dmPrompt {
	var prompts []dmPrompt
	v.mu.Lock()
	for k, p := range v.pend {
		if k.uid == uid && !p.done {
			prompts = append(prompts, dmPrompt{gid: k.gid, mode: p.mode, lang: p.lang, text: p.qText,
				opts: p.qOpts, nonce: p.nonce, tries: p.tries, fallback: len(p.fbAnswers) > 0,
				fallbackPending: p.fallbackPending, pending: p})
		}
	}
	v.mu.Unlock()
	return prompts
}

func definiteDMFailure(err error) bool {
	if err == nil {
		return false
	}
	code := gatewayFailureCode(err)
	return gatewayFailureHas(err, FailureCannotInitiateConversation) || gatewayFailureHas(err, FailureBlockedByUser) || code >= 400 && code < 500
}

func (v *Service) completeFailedDMDeliveryLocked(
	bot Gateway,
	uid int64,
	prompt dmPrompt,
	p *pending,
	sendErr error,
	question, resetExpiry bool,
	expectedEpoch uint64,
) (current, changed bool) {
	if !question || !prompt.fallbackPending || !p.fallbackPending || !definiteDMFailure(sendErr) {
		return true, false
	}
	p.qText = tgfmt.KernelQuestion(v.messages, p.lang)
	p.fbAnswers = nil
	p.fallbackPending = false
	p.hinted = false
	if resetExpiry {
		delay := v.gateTimeout(prompt.gid, prompt.pending.gate)
		p.deadline = v.wallNow().Add(delay)
		v.armExpiry(bot, p, prompt.gid, uid, delay, challengeExpiryReason(true))
	}
	return v.persistPendingLocked(pkey{prompt.gid, uid}, p, expectedEpoch), true
}

// Bind delivery completion to the pending pointer and nonce captured before the Bot API call.
func (v *Service) completeDMDelivery(
	bot Gateway,
	uid int64,
	prompt dmPrompt,
	messageID int,
	sendErr error,
	question bool,
	resetExpiry bool,
) (current, changed bool, oldPrivateMsgID int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{prompt.gid, uid}]
	if !ok || p != prompt.pending || p.done || p.nonce != prompt.nonce {
		return false, false, 0
	}
	expectedEpoch := p.epoch
	if sendErr != nil {
		current, changed = v.completeFailedDMDeliveryLocked(
			bot, uid, prompt, p, sendErr, question, resetExpiry, expectedEpoch,
		)
		return current, changed, 0
	}
	resetDeadline := false
	if question && prompt.fallbackPending && p.fallbackPending {
		p.fallbackPending = false
		resetDeadline = resetExpiry
		changed = true
	}
	if messageID != 0 && p.privateMsgID != messageID {
		oldPrivateMsgID = p.privateMsgID
		p.privateMsgID = messageID
		changed = true
	}
	if !p.challengeDelivered {
		p.challengeDelivered = true
		resetDeadline = resetExpiry
		changed = true
	}
	if resetDeadline {
		delay := v.gateTimeout(prompt.gid, prompt.pending.gate)
		p.deadline = v.wallNow().Add(delay)
		v.armExpiry(bot, p, prompt.gid, uid, delay, challengeExpiryReason(true))
	}
	if question && p.mode == settings.ModeKernel && !p.prompted {
		p.prompted = true
		changed = true
	}
	if changed && !v.persistPendingLocked(pkey{prompt.gid, uid}, p, expectedEpoch) {
		return false, true, oldPrivateMsgID
	}
	return true, changed, oldPrivateMsgID
}

func (v *Service) sendChannelPrompt(c context.Context, bot Gateway, gid, uid int64, ul i18n.Lang) (int, error) {
	channel := &v.messages.Verification.Channel
	if v.channelWasUnreadable(gid, uid) {
		// Asking someone to join a channel they may already be in is misleading when the bot
		// simply could not look. Say what actually happened instead.
		return sendHTML(c, bot, uid, channel.Unreadable.For(ul), nil)
	}
	var rows [][]Button
	if curl := v.channelURL(gid); curl != "" {
		rows = append(rows, []Button{{
			Text: channel.FollowButton.Render(ul, v.channelDisplay(gid)), URL: curl,
		}})
	}
	rows = append(rows, []Button{{Text: channel.ContinueButton.For(ul),
		CallbackData: ChannelRecheckCallbackPrefix + strconv.FormatInt(gid, 10) + ":" + strconv.FormatInt(uid, 10)}})
	return sendHTML(c, bot, uid, channel.FollowPrompt.Render(ul, v.channelLinkHTML(gid, ul)), rows)
}

func (v *Service) sendDMQuestionRetainingPrevious(
	c context.Context,
	bot Gateway,
	uid int64,
	prompt dmPrompt,
	resetExpiry bool,
) (dmSendResult, error) {
	var messageID int
	var err error
	if prompt.mode == settings.ModeKernel {
		left := kernelMaxTries - prompt.tries
		var rich, plain string
		if prompt.fallback {
			rich = tgfmt.FallbackPromptHTML(v.messages, prompt.lang, prompt.text, left, prompt.nonce, true)
			plain = tgfmt.FallbackPromptHTML(v.messages, prompt.lang, prompt.text, left, prompt.nonce, false)
		} else {
			held := gateOf(prompt.pending).gate == gateMute
			rich = tgfmt.KernelPromptHTML(v.messages, prompt.lang, prompt.text, left, prompt.nonce, true, held)
			plain = tgfmt.KernelPromptHTML(v.messages, prompt.lang, prompt.text, left, prompt.nonce, false, held)
		}
		messageID, err = v.sendVerifyDM(c, bot, uid, rich, plain)
	} else {
		gidStr, uidStr := strconv.FormatInt(prompt.gid, 10), strconv.FormatInt(uid, 10)
		rows := make([][]Button, 0, len(prompt.opts))
		for i, opt := range prompt.opts {
			rows = append(rows, []Button{{
				Text: opt, CallbackData: fmt.Sprintf("%s%s:%s:%s:%d", AnswerCallbackPrefix, gidStr, uidStr, prompt.nonce, i),
			}})
		}
		messageID, err = sendHTML(c, bot, uid,
			v.messages.Verification.Challenge.QuizPrompt.Render(prompt.lang, html.EscapeString(prompt.text)), rows)
	}
	current, _, oldPrivateMsgID := v.completeDMDelivery(bot, uid, prompt, messageID, err, true, resetExpiry)
	return dmSendResult{
		messageID:         messageID,
		replacedMessageID: oldPrivateMsgID,
		current:           current,
	}, err
}

func (v *Service) sendDMQuestion(c context.Context, bot Gateway, uid int64, prompt dmPrompt) (dmSendResult, error) {
	return v.sendDMQuestionRetainingPrevious(c, bot, uid, prompt, true)
}

// sendDMChallengeForGroup delivers only the named pending request and lets its caller clean up the replaced message.
func (v *Service) sendDMChallengeForGroup(
	c context.Context,
	bot Gateway,
	gid, uid int64,
	resetExpiry bool,
	owner *pending,
) (active bool, privateMsgID, replacedPrivateMsgID int, err error) {
	prompt, ok := v.pendingDMChallenge(gid, uid, owner)
	if !ok {
		return false, 0, 0, nil
	}
	if v.RequiredChannelID(gid) != 0 && !v.isChannelMember(c, bot, gid, uid, v.groupLanguage(gid)) {
		privateMsgID, err := v.sendChannelPrompt(c, bot, gid, uid, prompt.lang)
		current, _, oldPrivateMsgID := v.completeDMDelivery(bot, uid, prompt, privateMsgID, err, false, resetExpiry)
		if !current {
			return false, 0, 0, err
		}
		return true, privateMsgID, oldPrivateMsgID, err
	}
	result, err := v.sendDMQuestionRetainingPrevious(c, bot, uid, prompt, resetExpiry)
	if !result.current {
		return false, 0, 0, err
	}
	return true, result.messageID, result.replacedMessageID, err
}

type privateDeliveryResult uint8

const (
	privateDelivered privateDeliveryResult = iota
	privateRejected
	privateUncertain
	privateGone
)

type privateDeliveryOutcome struct {
	result               privateDeliveryResult
	privateMsgID         int
	replacedPrivateMsgID int
}

// attemptPrivateChallenge classifies private delivery without assuming that ambiguous failures rejected the send.
// rateLimitSendMargin leaves room for the retried send itself after a flood wait.
const rateLimitSendMargin = 10 * time.Second

// deliveryBudget is the time left before the pending this delivery belongs to expires.
func (v *Service) deliveryBudget(gid, uid int64, owner *pending) time.Duration {
	v.mu.Lock()
	defer v.mu.Unlock()
	p := owner
	if p == nil {
		p = v.pend[pkey{gid, uid}]
	}
	if p == nil {
		return pendingDeliveryTimeout
	}
	return p.deadline.Sub(v.wallNow())
}

func (v *Service) attemptPrivateChallenge(c context.Context, bot Gateway, gid, uid int64, owner *pending) privateDeliveryOutcome {
	active, privateMsgID, replacedPrivateMsgID, err := v.sendDMChallengeForGroup(c, bot, gid, uid, false, owner)
	if !active {
		return privateDeliveryOutcome{result: privateGone}
	}
	if err == nil {
		return privateDeliveryOutcome{
			result:               privateDelivered,
			privateMsgID:         privateMsgID,
			replacedPrivateMsgID: replacedPrivateMsgID,
		}
	}
	if gatewayFailureHas(err, FailureRateLimited) {
		wait := gatewayFailureRetryAfter(err)
		// Waiting out a flood limit is only worth it if the applicant's own window outlives the
		// wait. Sleeping past their deadline means they are declined before the question ever
		// arrives; falling through instead lets the group challenge carry the verification.
		if wait > 0 && wait < v.gateTimeout(gid, gateOf(owner).gate) && wait+rateLimitSendMargin <= v.deliveryBudget(gid, uid, owner) {
			log.Printf("join %d in %d: private challenge rate-limited; retrying after %s", uid, gid, wait)
			if !pace(c, wait) {
				return privateDeliveryOutcome{result: privateUncertain}
			}
			active, privateMsgID, replacedPrivateMsgID, err = v.sendDMChallengeForGroup(c, bot, gid, uid, false, owner)
			if !active {
				return privateDeliveryOutcome{result: privateGone}
			}
			if err == nil {
				return privateDeliveryOutcome{
					result:               privateDelivered,
					privateMsgID:         privateMsgID,
					replacedPrivateMsgID: replacedPrivateMsgID,
				}
			}
		}
	}
	switch {
	case gatewayFailureHas(err, FailureCannotInitiateConversation):
		return privateDeliveryOutcome{result: privateRejected}
	case gatewayFailureHas(err, FailureBlockedByUser):
		log.Printf("join %d in %d: private challenge rejected because the bot is blocked", uid, gid)
		return privateDeliveryOutcome{result: privateRejected}
	case gatewayFailureCode(err) >= 400 && gatewayFailureCode(err) < 500:
		log.Printf("join %d in %d: private challenge rejected by Telegram (%v)", uid, gid, err)
		return privateDeliveryOutcome{result: privateRejected}
	default:
		log.Printf("join %d in %d: private challenge delivery is uncertain (%v)", uid, gid, err)
		return privateDeliveryOutcome{result: privateUncertain}
	}
}

// SendDMChallenge sends or re-sends an applicant's private challenge. A zero group keeps legacy bare-link behavior.
func (v *Service) SendDMChallenge(c context.Context, uid int64, languageCode string, groupID int64) {
	bot := v.gateway
	channel := &v.messages.Verification.Channel
	if groupID != 0 {
		if _, ok := v.pendingDMChallenge(groupID, uid, nil); !ok {
			_, _ = sendText(c, bot, uid, channel.NoPending.For(i18n.FromTelegram(languageCode)))
			return
		}
		if !v.challengeResendOK(groupID, uid) {
			return
		}
		_, _, _, err := v.sendDMChallengeForGroup(c, bot, groupID, uid, true, nil)
		if err != nil {
			log.Printf("verify DM for %d in %d failed: %v", uid, groupID, err)
		}
		return
	}

	gid, ul, ok := v.firstPending(uid)
	if ok && !v.challengeResendOK(gid, uid) {
		return
	}
	if !ok {
		_, _ = sendText(c, bot, uid, channel.NoPending.For(i18n.FromTelegram(languageCode)))
		return
	}
	// Bare links preserve the legacy first-pending channel gate and all-pending fan-out.
	if v.RequiredChannelID(gid) != 0 && !v.isChannelMember(c, bot, gid, uid, v.groupLanguage(gid)) {
		if _, err := v.sendChannelPrompt(c, bot, gid, uid, ul); err != nil {
			log.Printf("verify DM channel prompt for %d in %d failed: %v", uid, gid, err)
		}
		return
	}
	v.sendQuizzes(c, bot, uid)
}

// DM every live challenge; kernel mode routes the next text DM as its answer.
func (v *Service) sendQuizzes(c context.Context, bot Gateway, uid int64) {
	for _, prompt := range v.pendingDMChallenges(uid) {
		_, _ = v.sendDMQuestion(c, bot, uid, prompt)
	}
}

func (v *Service) sendVerifyDM(c context.Context, bot Gateway, uid int64, rich, simpler string) (int, error) {
	return v.gatewayFor(bot).SendHTMLFallback(c, uid, rich, simpler)
}

// OnChannelRecheck continues verification after an applicant rechecks channel membership.
func (v *Service) OnChannelRecheck(ctx *HandlerContext, update Update) error {
	cq := update.CallbackQuery
	if cq == nil {
		return nil
	}
	bot := ctx.Gateway()
	c := ctx.Context()
	parts := strings.SplitN(strings.TrimPrefix(cq.Data, ChannelRecheckCallbackPrefix), ":", 2)
	if len(parts) != 2 {
		ackFast(c, bot, cq.ID)
		return nil
	}
	gid, _ := strconv.ParseInt(parts[0], 10, 64)
	uid, _ := strconv.ParseInt(parts[1], 10, 64)
	ul := v.applicantLanguage(gid, uid, cq.From.LanguageCode)
	groupLang := v.groupLanguage(gid)
	result := &(*v.messages).Verification.Result
	channel := &(*v.messages).Verification.Channel
	if cq.From.ID != uid {
		ackResult(c, bot, cq.ID, result.NotYours.For(ul), true)
		return nil
	}
	if !v.hasPending(uid) {
		ackResult(c, bot, cq.ID, result.AlreadyHandled.For(ul), false)
		return nil
	}
	if v.RequiredChannelID(gid) != 0 && !v.isChannelMember(c, bot, gid, uid, groupLang) {
		ackResult(c, bot, cq.ID, channel.NotFollowedYet.Render(ul, v.channelDisplay(gid)), true)
		return nil
	}
	// Acknowledge before sends; membership toasts remain result-driven and happen first.
	ackResult(c, bot, cq.ID, channel.ContinueOK.For(ul), false)
	v.sendQuizzes(c, bot, uid)
	return nil
}

func (v *Service) isGroupAdmin(ctx context.Context, bot Gateway, chatID, userID int64) bool {
	ok, err := v.gatewayFor(bot).FreshAdmin(ctx, chatID, userID)
	if err != nil {
		log.Printf("isGroupAdmin getChatMember chat=%d user=%d: %v", chatID, userID, err)
		return false
	}
	return ok
}

// Required-channel lookup also renders a throttled operator alert when the gate is unavailable.
// isChannelMember answers the required-channel gate and records, on the applicant's pending,
// whether the answer could be read at all. An unreadable gate still refuses entry, but the
// applicant must not carry a strike for a door the bot could not see through.
func (v *Service) isChannelMember(c context.Context, bot Gateway, gid, userID int64, groupLang i18n.Lang) bool {
	member, known := v.channelGate(c, bot, gid, userID, groupLang)
	v.markChannelReadable(gid, userID, known)
	return member
}

func (v *Service) channelGate(c context.Context, bot Gateway, gid, userID int64, groupLang i18n.Lang) (member, known bool) {
	rc := v.RequiredChannelID(gid)
	if rc == 0 {
		return true, true
	}
	cm, err := bot.Member(c, rc, userID)
	if err != nil {
		// If the bot cannot read its own membership, the gate is unenforceable.
		// Apply configured fail-open policy and alert admins instead of silently blocking everyone.
		if v.botID != 0 {
			if _, e2 := bot.Member(c, rc, v.botID); e2 != nil {
				open := true
				if group, ok := v.groupSettings(gid); ok {
					open = group.RequiredChannelFailOpen().Value
				}
				log.Printf("isChannelMember: bot cannot access required channel %d (%v) for applicant %d; fail_open=%v — make the bot an admin of that channel", rc, e2, userID, open)
				v.channelAccessAlert(c, bot, gid, groupLang, rc, open)
				return open, false
			}
		}
		log.Printf("getChatMember(channel=%d user=%d): %v", rc, userID, err)
		return false, false
	}
	switch cm.MemberStatus() {
	case MemberStatusCreator, MemberStatusAdministrator, MemberStatusMember:
		return true, true
	default:
		return cm.MemberIsMember(), true
	}
}

func (v *Service) channelWasUnreadable(gid, uid int64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	return ok && p.channelUnreadable
}

// markChannelReadable records the latest gate reading on the live pending. Clearing it on a
// readable answer matters as much as setting it: one transient failure must not exempt an
// applicant who genuinely never joined the channel.
func (v *Service) markChannelReadable(gid, uid int64, known bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if p, ok := v.pend[key]; ok {
		p.channelUnreadable = !known
		if !p.done {
			v.persistPendingLocked(key, p, p.epoch)
		}
	}
}

// Prefer an explicit private-channel invite; otherwise derive a public t.me URL.
func (v *Service) channelURL(gid int64) string {
	if u := v.channelInviteURL(gid); u != "" {
		return u
	}
	if d := v.channelDisplay(gid); strings.HasPrefix(d, "@") {
		return "https://t.me/" + d[1:]
	}
	return ""
}

func (v *Service) channelLinkHTML(gid int64, ul i18n.Lang) string {
	d := v.channelDisplay(gid)
	if d == "" {
		d = (*v.messages).Verification.Channel.FallbackName.For(ul) // unnamed channels still read naturally
	}
	if u := v.channelURL(gid); u != "" {
		return fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(u), html.EscapeString(d))
	}
	return html.EscapeString(d)
}

// Caller holds v.mu. A pointer match prevents an old action from releasing a newer claim.
func (v *Service) markTerminalLocked(key pkey, p *pending) {
	if v.terminal == nil {
		v.terminal = make(map[pkey]*pending)
	}
	v.terminal[key] = p
}

// Shutdown freezes pending settlement. Deadlines remain durable for the next scanner.
func (v *Service) stopForShutdown() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.shuttingDown = true
}

// Shutdown freezes pending settlement and flushes the remaining ancillary snapshots.
func (v *Service) Shutdown() {
	v.stopForShutdown()
	v.saveVerifyFails()
	v.saveHeartbeat()
}

func (v *Service) deleteChallenge(c context.Context, bot Gateway, gid int64, msgID int) {
	if err := v.gatewayFor(bot).Delete(c, gid, msgID); err != nil && !gatewayFailureHas(err, FailureMessageGone) {
		log.Printf("verification: delete message %d in %d: %v", msgID, gid, err)
	}
}

// Verification cleanup never deletes an applicant's private conversation. Group messages are
// public challenge evidence; their durable cleanup action owns retries after settlement.
func (v *Service) deleteChallenges(c context.Context, bot Gateway, gid, _ int64, messages challengeMessages) {
	v.deleteChallenge(c, bot, gid, messages.groupMsgID)
}

func (v *Service) adminLogChatID(groupID int64) int64 {
	if group, ok := v.groupSettings(groupID); ok {
		return group.AdminLogChatID().Value
	}
	return v.cfg.AdminLogChatID
}

func (v *Service) adminAlert(c context.Context, bot Gateway, groupID int64, text string) {
	v.gatewayFor(bot).Alert(c, v.adminLogChatID(groupID), text)
}

// adminRecord logs an action that happened once, so identical repeats must all appear.
func (v *Service) adminRecord(c context.Context, bot Gateway, groupID int64, text string) {
	v.gatewayFor(bot).AuditLog(c, v.adminLogChatID(groupID), text)
}

// A failure nobody can act on is not worth an operator notice. A deactivated applicant cannot be
// approved, declined, or settled by an administrator either, so the group hears nothing.
func (v *Service) settlementAlert(c context.Context, bot Gateway, gid int64, err error, text string) {
	if gatewayFailureHas(err, FailureApplicantGone) {
		return
	}
	v.failAlert(c, bot, gid, text)
}

// Failure notices fall back to the acting group when no admin-log chat is configured.
// This keeps optimistic callback acknowledgements from hiding rare network failures.
func (v *Service) failAlert(c context.Context, bot Gateway, gid int64, text string) {
	v.gatewayFor(bot).FailAlert(c, v.adminLogChatID(gid), gid, text)
}

// Throttle unreadable-channel alerts per channel to avoid flooding operators.
const channelAccessAlertCooldown = 10 * time.Minute

func (v *Service) channelAccessAlert(c context.Context, bot Gateway, groupID int64, l i18n.Lang, channelID int64, open bool) {
	v.mu.Lock()
	now := time.Now()
	if last, ok := v.chanAlert[channelID]; ok && now.Sub(last) < channelAccessAlertCooldown {
		v.mu.Unlock()
		return
	}
	cutoff := now.Add(-channelAccessAlertCooldown)
	for id, last := range v.chanAlert {
		if !last.After(cutoff) {
			delete(v.chanAlert, id)
		}
	}
	v.chanAlert[channelID] = now
	v.mu.Unlock()
	admin := &v.messages.Verification.Admin
	mode := admin.ChannelFailOpen.For(l)
	if !open {
		mode = admin.ChannelFailClosed.For(l)
	}
	v.adminAlert(c, bot, groupID, admin.ChannelAccessFailed.Render(l, channelID, mode))
}

// Claim before approval so its timeout cannot decline or strike concurrently.
// Callback handlers may acknowledge between claimPending and executeApprove.
func (v *Service) approve(c context.Context, bot Gateway, gid, uid int64) bool {
	p, ok, err := v.claimPending(gid, uid)
	if err != nil {
		log.Printf("verification: approve claim for group %d user %d: %v", gid, uid, err)
		return false
	}
	if !ok {
		return false
	}
	return v.executeApprove(c, bot, gid, uid, p) == approveConfirmed
}

// approveOutcome distinguishes a confirmed approval from a request that was settled
type approveOutcome int

const (
	approveFailed    approveOutcome = iota // unconfirmed; the request is kept for a retry
	approveConfirmed                       // the applicant is in the group
	approveGone                            // the request is gone and where it went is unknown
)

// Failed approval reopens the claimed pending instead of stranding the applicant.
func (v *Service) executeApprove(c context.Context, bot Gateway, gid, uid int64, p *pending) approveOutcome {
	if p.gate == gateMute {
		return v.executeRelease(c, bot, gid, uid, p)
	}
	if err := bot.ApproveJoin(c, gid, uid); err != nil {
		if !gatewayFailureHas(err, FailureJoinRequestGone) {
			log.Printf("approve %d in %d: %v", uid, gid, err)
			v.settlementAlert(c, bot, gid, err, v.adminSays(p.gate).ApproveFailed.Render(v.groupLanguage(gid), uid, gid, err))
			v.markPassing(gid, uid, p)
			v.stopRetrying(c, bot, gid, uid, p, "approve-retry", err)
			return approveFailed
		}
		// An administrator settled this request in Telegram's own interface. The same error
		// covers a manual approval and a manual rejection, so ask who is actually in the
		// group rather than announcing an outcome the bot did not produce.
		log.Printf("approve %d in %d: join request is already gone: %v", uid, gid, err)
		member, known := v.chatMemberState(c, bot, gid, uid)
		if known && member {
			v.notePassed(gid, uid)
			v.finishTerminal(gid, uid, p)
			v.cleanupSettledChallenge(c, bot, gid, uid, p)
			v.clearVerifyFails(gid, uid)
			v.recordDecision(true)
			return approveConfirmed
		}
		v.discardPending(gid, uid, p)
		v.cleanupSettledChallenge(c, bot, gid, uid, p)
		return approveGone
	}
	// Record the pass before the terminal marker is released. Approving produces a membership
	// update of its own, and it can be handled while the calls below are still in flight; until
	// the marker goes the terminal claim blocks a second verification, and from then on this
	// record does. Leaving a gap between them re-challenges the person just admitted.
	v.notePassed(gid, uid)
	v.finishTerminal(gid, uid, p)
	v.clearVerifyFails(gid, uid) // verified successfully — reset any failure strikes
	v.cleanupSettledChallenge(c, bot, gid, uid, p)
	v.recordDecision(true)
	log.Printf("approve user=%d group=%d", uid, gid)
	return approveConfirmed
}

// A settlement the bot cannot complete — it lost the rights to approve and decline — would
// otherwise retry every minute forever, DMing the applicant each round. Give up after this many
// attempts and leave the request for an administrator: Telegram still holds it, so nobody gets
// in without verification.
const maxSettleFailures = 10

// giveUpSettling reports a failure no retry can repair, so the attempt budget is spent at once.
func giveUpSettling(err error) bool {
	return gatewayFailureHas(err, FailureGroupUnreachable) || gatewayFailureHas(err, FailureApplicantGone)
}

// stopRetrying requeues a claimed durable action unless the error proves no attempt can succeed.
// Legacy in-memory stores retain the old reopen path for their focused compatibility tests.
func (v *Service) stopRetrying(c context.Context, bot Gateway, gid, uid int64, p *pending, reason string, err error) {
	if p.actionID != "" && !v.stateUnavailable(v.statePath) {
		if !v.retrySettlementAction(p, err) {
			v.abandonSettlement(c, bot, gid, uid, p, reason, err)
		}
		return
	}
	if giveUpSettling(err) {
		v.abandonSettlement(c, bot, gid, uid, p, reason, err)
		return
	}
	if !v.reopenPending(bot, gid, uid, p, reason) {
		v.releaseAbandonedHold(c, bot, gid, uid, p)
	}
}

// releaseAbandonedHold lifts a verification mute the bot has stopped trying to settle. Dropping
// the verification must not leave somebody silenced with nothing left to lift it.
func (v *Service) releaseAbandonedHold(c context.Context, bot Gateway, gid, uid int64, p *pending) {
	if p.gate != gateMute || !p.held {
		return
	}
	if err := v.releaseMember(c, bot, gid, uid, p); err != nil {
		log.Printf("post-join verify: gave up on %d in %d and could not lift the hold: %v", uid, gid, err)
	}
}

// adoptAsHeld moves a verification that began outside the group onto the gate that can settle it
// now that the applicant is inside. Their progress is untouched: same question, same attempts,
// same deadline.
func (v *Service) adoptAsHeld(gid, uid int64, p *pending) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if cur, ok := v.pend[key]; ok && cur == p && !p.done && p.gate != gateMute {
		log.Printf("post-join verify: %d in %d entered the group mid-verification; settling as a member from here", uid, gid)
		p.gate = gateMute
		v.persistPendingLocked(key, p, p.epoch)
	}
}

// abandonSettlement drops a pending the bot can never settle. The join request stays with
// Telegram for an administrator, so abandoning it admits nobody.
func (v *Service) abandonSettlement(c context.Context, bot Gateway, gid, uid int64, p *pending, reason string, err error) {
	log.Printf("WARNING: cannot settle verification for %d in %d (%s): %v; "+
		"the join request stays with Telegram for an administrator", uid, gid, reason, err)
	if p.gate == gateMute && p.held {
		// Dropping the verification must not leave somebody silenced with nothing left to lift
		// it. The restriction carries its own expiry, so a failure here only delays them.
		if releaseErr := v.releaseMember(c, bot, gid, uid, p); releaseErr != nil {
			log.Printf("post-join verify: giving up on %d in %d but could not lift the hold: %v", uid, gid, releaseErr)
		}
	}
	v.discardPending(gid, uid, p)
}

// Reopen a bot-caused failed settlement only if the same nonce, epoch, and claimed state
// remain current in storage. Re-arming increments epoch before the conditional rollback.
func (v *Service) reopenPending(bot Gateway, gid, uid int64, p *pending, reason string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid, uid}
	if cur, ok := v.pend[key]; !ok || cur != p || !p.done {
		return false
	}
	from := p.claimedState
	p.settleFailures++
	if p.settleFailures >= maxSettleFailures {
		log.Printf("WARNING: giving up on settling verification for %d in %d after %d attempts (%s); "+
			"the join request stays with Telegram for an administrator", uid, gid, p.settleFailures, reason)
		if from != "" {
			changed, err := v.transitionChallengeLocked(key, p, from, ChallengeSuperseded, "", 0, p.epoch)
			if err != nil {
				log.Printf("verification: supersede abandoned settlement for group %d user %d: %v", gid, uid, err)
				return false
			}
			if !changed {
				v.forgetPendingLocked(key, p)
				return false
			}
		}
		v.forgetPendingLocked(key, p)
		return false
	}
	expectedEpoch := p.epoch
	originalDeadline := p.deadline
	p.done = false
	// Keep the moment the applicant actually failed. Every caller here is retrying a settlement
	// for a failure that already happened, so stamping the strike with the retry time instead
	// would start their cooldown from however long the bot spent retrying.
	delay := p.deadline.Sub(v.wallNow())
	if delay < noFaultGrace {
		delay = noFaultGrace
		p.deadline = v.wallNow().Add(delay)
	}
	v.armExpiry(bot, p, gid, uid, delay, reason)
	changed := true
	var err error
	if from != "" {
		changed, err = v.transitionChallengeLocked(key, p, from, ChallengePending, "", 0, expectedEpoch)
	}
	if err != nil {
		log.Printf("verification: reopen settlement for group %d user %d: %v", gid, uid, err)
		p.done = true
		p.epoch = expectedEpoch
		p.deadline = originalDeadline
		return false
	}
	if !changed {
		v.forgetPendingLocked(key, p)
		return false
	}
	p.claimedState = ""
	v.releaseTerminalLocked(key, p)
	return true
}

// Wrong-answer feedback distinguishes automatic ban from cooldown retry.
// outcomeVoice is the applicant-facing wording for one gate. Passing a join request and lifting a
// hold are different events, and saying "your join request was declined" to somebody who is
// standing in the group is simply false.
type outcomeVoice struct {
	Passed          i18n.Text
	AlreadyHandled  i18n.Text
	SettlePending   i18n.Text
	DeferralExpired i18n.Text
	Undelivered     i18n.Text
	WrongNoWait     i18n.Text
	WrongRetry      i18n.Format
	WrongBanned     i18n.Format
	TimeoutNoWait   i18n.Text
	TimeoutRetry    i18n.Format
	TimeoutBanned   i18n.Format
	AICaught        i18n.Format
	AICaughtNoWait  i18n.Text
}

// markPassing records that the applicant already earned admission, so a settlement retry
// completes the approval instead of falling into the generic timeout path, which declines.
func (v *Service) markPassing(gid, uid int64, p *pending) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if cur, ok := v.pend[pkey{gid, uid}]; ok && cur == p {
		p.passing = true
	}
}

// pendingHasNonce reports that the live verification is the one a button was posted for.
func (v *Service) pendingHasNonce(gid, uid int64, nonce string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	p, ok := v.pend[pkey{gid, uid}]
	return ok && !p.done && p.nonce == nonce
}

// pendingGate reads the gate of a live pending; a missing one reads as the join-request gate.
func (v *Service) pendingGate(gid, uid int64) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	if p, ok := v.pend[pkey{gid, uid}]; ok {
		return p.gate
	}
	return gateRequest
}

// adminVoice is the operator-facing wording for one gate. Telling an administrator that a join
// request is still pending, when the person is standing in the group muted, sends them looking
// for a queue entry that does not exist.
type adminVoice struct {
	Approving           i18n.Text
	Banning             i18n.Format
	ApproveFailed       i18n.Format
	BanFailed           i18n.Format
	DeclineFailed       i18n.Format
	ActionFailed        i18n.Text
	CannotApprove       i18n.Text
	AlreadyHandled      i18n.Text
	ChallengePostFailed i18n.Format
	PendingCap          i18n.Format
}

func (v *Service) adminSays(gate string) adminVoice {
	a := &v.messages.Verification.Admin
	if gate == gateMute {
		return adminVoice{
			Approving: a.ApprovingHeld, Banning: a.BanningHeld, ApproveFailed: a.ApproveFailedHeld,
			BanFailed: a.BanFailedHeld, DeclineFailed: a.DeclineFailedHeld, ActionFailed: a.ActionFailedHeld,
			CannotApprove: a.CannotApproveHeld, AlreadyHandled: a.AlreadyHandledHeld,
			ChallengePostFailed: a.ChallengePostFailedHeld, PendingCap: a.PendingCapHeld,
		}
	}
	return adminVoice{
		Approving: a.Approving, Banning: a.Banning, ApproveFailed: a.ApproveFailed,
		BanFailed: a.BanFailed, DeclineFailed: a.DeclineFailed, ActionFailed: a.ActionFailed,
		CannotApprove: a.CannotApprove, AlreadyHandled: a.AlreadyHandled,
		ChallengePostFailed: a.ChallengePostFailed, PendingCap: a.PendingCap,
	}
}

func (v *Service) voice(gate string) outcomeVoice {
	if gate == gateMute {
		held := &v.messages.Verification.Held
		return outcomeVoice{
			Passed: held.Passed, AlreadyHandled: held.AlreadyHandled, SettlePending: held.SettlePending,
			DeferralExpired: held.DeferralExpired, Undelivered: held.Undelivered,
			WrongNoWait: held.WrongNoWait, WrongRetry: held.WrongRetry, WrongBanned: held.WrongBanned,
			TimeoutNoWait: held.TimeoutNoWait, TimeoutRetry: held.TimeoutRetry, TimeoutBanned: held.TimeoutBanned,
			AICaught: held.AICaught, AICaughtNoWait: held.AICaughtNoWait,
		}
	}
	result := &v.messages.Verification.Result
	return outcomeVoice{
		Passed: result.Approved, AlreadyHandled: result.AlreadyHandled, SettlePending: result.DeclinePending,
		DeferralExpired: result.DeferralExpired, Undelivered: result.Undelivered,
		WrongNoWait: result.WrongNoWait, WrongRetry: result.WrongRetry, WrongBanned: result.WrongBanned,
		TimeoutNoWait: result.TimeoutNoWait, TimeoutRetry: result.TimeoutRetry, TimeoutBanned: result.TimeoutBanned,
		AICaught: result.AICaught, AICaughtNoWait: result.AICaughtNoWait,
	}
}

func (v *Service) wrongAnswerText(groupID int64, l i18n.Lang, gate string, banned bool) string {
	voice := v.voice(gate)
	if banned {
		return v.bannedResultText(groupID, l, gate)
	}
	if seconds := v.verifyRetrySeconds(groupID); seconds > 0 {
		return voice.WrongRetry.Render(l, seconds)
	}
	return voice.WrongNoWait.For(l)
}

func (v *Service) agentCaughtText(groupID int64, l i18n.Lang, gate string, banned bool) string {
	voice := v.voice(gate)
	if banned {
		return v.bannedResultText(groupID, l, gate)
	}
	if seconds := v.verifyRetrySeconds(groupID); seconds > 0 {
		return voice.AICaught.Render(l, seconds)
	}
	return voice.AICaughtNoWait.For(l)
}

func (v *Service) bannedResultText(groupID int64, l i18n.Lang, gate string) string {
	duration := verificationBanDurationText(v.messages, l, v.verificationBanDuration(groupID))
	return v.voice(gate).WrongBanned.Render(l, duration)
}

func durationSecondsCeil(wait time.Duration) int {
	if wait <= 0 {
		return 0
	}
	return int((wait + time.Second - 1) / time.Second)
}

func verificationBanDurationText(messages *i18n.Catalog, l i18n.Lang, seconds int) string {
	duration := &messages.Verification.Duration
	if seconds <= 0 {
		return duration.Permanent.For(l)
	}
	switch {
	case seconds%86400 == 0:
		return duration.Days.Render(l, seconds/86400)
	case seconds%3600 == 0:
		return duration.Hours.Render(l, seconds/3600)
	case seconds%60 == 0:
		return duration.Minutes.Render(l, seconds/60)
	default:
		return duration.Seconds.Render(l, seconds)
	}
}

func joinerLabel(userID int64, name string, spoiler bool) string {
	escaped := html.EscapeString(name)
	if spoiler {
		return "<tg-spoiler>" + escaped + "</tg-spoiler>"
	}
	return fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", userID, escaped)
}

func (v *Service) timeoutResultText(groupID, userID int64, l i18n.Lang, gate string, banned bool) string {
	voice := v.voice(gate)
	if banned {
		duration := verificationBanDurationText(v.messages, l, v.verificationBanDuration(groupID))
		return voice.TimeoutBanned.Render(l, duration)
	}
	if seconds := durationSecondsCeil(v.verifyCooldownRemaining(groupID, userID)); seconds > 0 {
		return voice.TimeoutRetry.Render(l, seconds)
	}
	return voice.TimeoutNoWait.For(l)
}

// Bot-caused failures receive a meaningful strike-free retry window.
const noFaultGrace = 60 * time.Second

// Timeouts and wrong answers strike; delivery, settlement, restart, and recovery failures do not.
// declineResultText keeps every applicant-facing decline message inside what the bot knows:
// a definite result only when the request was actually settled, and a neutral "already handled"
// when it vanished and the applicant's fate could not be read.
func (v *Service) declineResultText(outcome declineOutcome, l i18n.Lang, gate string, settledText func() string) string {
	voice := v.voice(gate)
	switch outcome {
	case declineUnsettled:
		return voice.SettlePending.For(l)
	case declineGoneUnknown:
		return voice.AlreadyHandled.For(l)
	default:
		return settledText()
	}
}

// errRemovalLeftBanned names the state a failed unban leaves behind, for the operator alert.
var errRemovalLeftBanned = errors.New("removed but the ban could not be lifted; unban them manually")

// wrongAnswerReason marks a decline the applicant caused by failing the challenge.
const wrongAnswerReason = "wrong answer"

func storedDeclineReason(reason string) string {
	if reason == wrongAnswerReason {
		return "wrong_answer"
	}
	return "rejected"
}

// A request that vanished before the bot could reject it still leaves the applicant out of the
// group, so a wrong answer still counts. A timeout does not: the request may have been rejected
// by an administrator long before the window ran out, and nobody should carry a strike for that.
func (v *Service) strikesFor(outcome declineOutcome, reason string) bool {
	if !strikesUser(reason) {
		return false
	}
	return outcome == declineConfirmed || reason == wrongAnswerReason
}

func strikesUser(reason string) bool {
	switch reason {
	case "approve-retry", "ban-retry", "decline-retry", "restart-lapsed", "recovered",
		"challenge-post-failed", deferredExpiryReason:
		return false
	default:
		return true
	}
}

// executeRelease settles a held member by lifting the verification mute. There is no join request
// to lose here, so the only outcomes are success and a failure worth retrying.
func (v *Service) executeRelease(c context.Context, bot Gateway, gid, uid int64, p *pending) approveOutcome {
	// A basic group could never be held, so there is nothing to lift and nothing to retry.
	if p.held {
		if err := v.releaseMember(c, bot, gid, uid, p); err != nil {
			log.Printf("post-join verify: cannot lift the hold on %d in %d: %v", uid, gid, err)
			v.settlementAlert(c, bot, gid, err, v.adminSays(p.gate).ApproveFailed.Render(v.groupLanguage(gid), uid, gid, err))
			v.markPassing(gid, uid, p)
			v.stopRetrying(c, bot, gid, uid, p, "approve-retry", err)
			return approveFailed
		}
	}
	v.notePassed(gid, uid)
	v.finishTerminal(gid, uid, p)
	v.clearVerifyFails(gid, uid)
	v.cleanupSettledChallenge(c, bot, gid, uid, p)
	v.recordDecision(true)
	log.Printf("post-join verify: released user=%d group=%d", uid, gid)
	return approveConfirmed
}

// declineOutcome tells a caller how much the bot actually knows, so no message states an
// outcome Telegram never confirmed.
type declineOutcome int

const (
	declineNoPending   declineOutcome = iota // nothing matched the claim
	declineUnsettled                         // a transient failure; the request is kept for a retry
	declineConfirmed                         // Telegram confirmed the rejection
	declineGoneAndOut                        // the request is gone and the applicant is not in the group
	declineGoneUnknown                       // the request is gone and where it went cannot be read
)

// Live wrong answers use nonce claims; timeout settlement uses epoch claims so outages may defer it.
func (v *Service) decline(
	c context.Context,
	bot Gateway,
	gid, uid int64,
	nonce, reason string,
) (declineOutcome, bool, error) {
	p, ok, err := v.claimPendingNonceAs(gid, uid, nonce, ChallengeDeclined, storedDeclineReason(reason), 0)
	if err != nil || !ok {
		return declineNoPending, false, err
	}
	outcome, banned := v.finishDecline(c, bot, gid, uid, p, reason)
	return outcome, banned, nil
}

// settled reports the outcomes that let a caller state a definite verification result.
func (o declineOutcome) settled() bool {
	return o == declineConfirmed || o == declineGoneAndOut
}

// Settle an already-claimed decline, striking at claim time only after Telegram confirms rejection.
func (v *Service) finishDecline(c context.Context, bot Gateway, gid, uid int64, p *pending, reason string) (outcome declineOutcome, banned bool) {
	outcome = declineConfirmed
	leftBanned := false
	settle := func() error {
		if p.gate == gateMute {
			stranded, err := v.removeMember(c, bot, gid, uid)
			leftBanned = stranded
			return err
		}
		return bot.DeclineJoin(c, gid, uid)
	}
	if err := settle(); err != nil {
		// A vanished join request has no counterpart when the applicant is already a member:
		// removal either worked or is worth retrying.
		if p.gate == gateMute || !gatewayFailureHas(err, FailureJoinRequestGone) {
			log.Printf("decline %d in %d failed: %v", uid, gid, err)
			v.settlementAlert(c, bot, gid, err, v.adminSays(p.gate).DeclineFailed.Render(v.groupLanguage(gid), uid, gid, err))
			v.stopRetrying(c, bot, gid, uid, p, "decline-retry", err)
			return declineUnsettled, false
		}
		// Telegram no longer holds the request, so no retry can settle it. Whether the applicant
		// ended up inside the group decides both what to tell them and whether the failure is
		// theirs to carry, and only their membership can answer that.
		log.Printf("decline %d in %d: join request is already gone: %v", uid, gid, err)
		member, known := v.chatMemberState(c, bot, gid, uid)
		switch {
		case known && member:
			// An administrator let them in. Nothing here is the applicant's fault.
			v.cleanupSettledChallenge(c, bot, gid, uid, p)
			v.clearVerifyFails(gid, uid)
			v.discardPending(gid, uid, p)
			return declineGoneUnknown, false
		case !known:
			v.cleanupSettledChallenge(c, bot, gid, uid, p)
			v.discardPending(gid, uid, p)
			return declineGoneUnknown, false
		}
		// They are out, exactly as this decline intended, so settle it like a confirmed one.
		outcome = declineGoneAndOut
	}
	v.cleanupSettledChallenge(c, bot, gid, uid, p)
	var count int
	var doBan bool
	// A gate the bot could not read is the bot's problem; the applicant does not carry it.
	if v.strikesFor(outcome, reason) && !p.channelUnreadable {
		failedAt := p.failedAt
		if failedAt.IsZero() {
			failedAt = v.wallNow()
		}
		var recorded bool
		count, doBan, recorded = v.recordPendingVerifyFail(gid, uid, p, failedAt)
		if recorded {
			v.recordDecision(false)
		}
	}
	if doBan {
		secs := v.verificationBanDuration(gid)
		if err := v.applyVerificationBan(c, bot, gid, uid, secs, false); err != nil {
			log.Printf("verify auto-ban %d in %d: %v", uid, gid, err)
			admin := &v.messages.Verification.Admin
			v.settlementAlert(c, bot, gid, err, admin.AutoBanFailed.Render(v.groupLanguage(gid), uid, gid, count, err))
		} else {
			l := v.groupLanguage(gid)
			duration := verificationBanDurationText(v.messages, l, secs)
			v.adminRecord(c, bot, gid, v.messages.Verification.Admin.AutoBanned.Render(l, uid, gid, count, duration))
			banned = true
		}
		if banned {
			v.clearVerifyFails(gid, uid)
		}
	}
	if leftBanned {
		// The removal could not be undone, so they really are banned. Say that instead of
		// inviting them back, and tell operators, because only they can lift it.
		banned = true
		admin := &v.messages.Verification.Admin
		v.failAlert(c, bot, gid, admin.BanFailed.Render(v.groupLanguage(gid), uid, gid, errRemovalLeftBanned))
	}
	v.finishClaimedDecline(gid, uid, p, banned)
	log.Printf("decline user=%d group=%d (%s) fails=%d banned=%v", uid, gid, reason, count, banned)
	return outcome, banned
}

func (v *Service) finishClaimedDecline(gid, uid int64, p *pending, banned bool) {
	if banned {
		v.reclassifyClaimed(gid, uid, p, ChallengeBanned)
	}
	v.finishTerminal(gid, uid, p)
}

func (v *Service) finishTerminal(gid, uid int64, p *pending) {
	v.mu.Lock()
	key := pkey{gid, uid}
	if cur, ok := v.pend[key]; ok && cur == p {
		delete(v.pend, key)
	}
	v.releaseTerminalLocked(key, p)
	v.mu.Unlock()
}
func (v *Service) reclassifyClaimed(gid, uid int64, p *pending, to ChallengeState) {
	v.mu.Lock()
	defer v.mu.Unlock()
	from := p.claimedState
	if from == "" || from == to {
		return
	}
	key := pkey{gid, uid}
	changed, err := v.transitionChallengeLocked(key, p, from, to, "", 0, p.epoch)
	if err != nil {
		log.Printf("verification: transition settled challenge for group %d user %d from %s to %s: %v", gid, uid, from, to, err)
		return
	}
	if changed {
		p.claimedState = to
	}
}

// banApplicant preserves the request for retry when either required Telegram action is unconfirmed.
func (v *Service) banApplicant(c context.Context, bot Gateway, gid, uid int64) (handled, banned bool) {
	p, ok, err := v.consume(gid, uid)
	if err != nil {
		log.Printf("verification: ban claim for group %d user %d: %v", gid, uid, err)
		return false, false
	}
	if !ok {
		return false, false
	}
	return true, v.executeBan(c, bot, gid, uid, p)
}

// executeBan confirms the ban before declining so an unconfirmed ban retains the request and its evidence.
func (v *Service) executeBan(c context.Context, bot Gateway, gid, uid int64, p *pending) bool {
	if err := v.applyVerificationBan(c, bot, gid, uid, v.verificationBanDuration(gid), true); err != nil {
		log.Printf("banApplicant %d in %d: %v", uid, gid, err)
		v.settlementAlert(c, bot, gid, err, v.adminSays(p.gate).BanFailed.Render(v.groupLanguage(gid), uid, gid, err))
		v.stopRetrying(c, bot, gid, uid, p, "ban-retry", err)
		return false
	}
	if p.gate == gateMute {
		// The ban already removed them; there is no join request left to decline.
		v.cleanupSettledChallenge(c, bot, gid, uid, p)
		v.recordDecision(false)
		v.finishTerminal(gid, uid, p)
		log.Printf("banApplicant user=%d group=%d banned=true (admin report, held member)", uid, gid)
		return true
	}
	if err := bot.DeclineJoin(c, gid, uid); err != nil {
		if !gatewayFailureHas(err, FailureJoinRequestGone) {
			log.Printf("decline after ban %d in %d: %v", uid, gid, err)
			v.settlementAlert(c, bot, gid, err, v.adminSays(p.gate).DeclineFailed.Render(v.groupLanguage(gid), uid, gid, err))
			v.stopRetrying(c, bot, gid, uid, p, "ban-retry", err)
			return false
		}
		// The ban is confirmed and the request is gone; retrying the decline cannot help.
		log.Printf("decline after ban %d in %d: join request is already gone: %v", uid, gid, err)
	}
	v.cleanupSettledChallenge(c, bot, gid, uid, p)
	v.recordDecision(false)
	v.finishTerminal(gid, uid, p)
	log.Printf("banApplicant user=%d group=%d banned=true (admin report)", uid, gid)
	return true
}

// Challenge selection uses crypto/rand; failure degrades to deterministic index zero.
func cryptoIntn(n int) int {
	if n <= 1 {
		return 0
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func randomQuestion(questions []settings.Question) settings.Question {
	return questions[cryptoIntn(len(questions))]
}

// Shuffling prevents fixed-position clicks while preserving the correct option's new index.
func shuffledQuestion(q settings.Question) (text string, opts []string, correctIdx int) {
	order := make([]int, len(q.Options))
	for i := range order {
		order[i] = i
	}
	for i := len(order) - 1; i > 0; i-- {
		j := cryptoIntn(i + 1)
		order[i], order[j] = order[j], order[i]
	}
	opts = make([]string, len(order))
	for newPos, orig := range order {
		opts[newPos] = q.Options[orig]
		if orig == q.Answer {
			correctIdx = newPos
		}
	}
	return q.Q, opts, correctIdx
}
