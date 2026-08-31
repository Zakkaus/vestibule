package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	ownerClaimLifetime  = 10 * time.Minute
	enrollmentLifetime  = 10 * time.Minute
	registrationPending = 10 * time.Minute
	setupReportCooldown = 10 * time.Minute
)

var (
	errEnrollmentInvalid       = errors.New("enrollment nonce is invalid")
	errBotMembershipUnreadable = errors.New("bot membership is unreadable")
	errBotMembershipIneligible = errors.New("bot is not an eligible group member")
	errRuntimeGroupNotFound    = errors.New("runtime-registered group was not found")
	errUnknownLeaveSuperseded  = errors.New("unknown-group cleanup was superseded by registration")
)

type botMembershipState uint8

const (
	botMembershipIneligible botMembershipState = iota
	botMembershipMember
	botMembershipAdmin
)

type registrationTransition struct {
	mu   sync.Mutex
	refs int
}

type registrationTransitionLocks struct {
	mu     sync.Mutex
	groups map[int64]*registrationTransition
}

func (l *registrationTransitionLocks) lock(groupID int64) func() {
	l.mu.Lock()
	if l.groups == nil {
		l.groups = make(map[int64]*registrationTransition)
	}
	transition := l.groups[groupID]
	if transition == nil {
		transition = &registrationTransition{}
		l.groups[groupID] = transition
	}
	transition.refs++
	l.mu.Unlock()

	transition.mu.Lock()
	return func() {
		transition.mu.Unlock()
		l.mu.Lock()
		transition.refs--
		if transition.refs == 0 {
			delete(l.groups, groupID)
		}
		l.mu.Unlock()
	}
}

type registrationService struct {
	root                context.Context
	bot                 *telego.Bot
	settings            *store.Settings
	cfg                 *config.Config
	username            string
	selfID              int64
	now                 func() time.Time
	onOwnerClaimed      func(context.Context)
	onRegistered        func(context.Context, int64)
	onMembershipChanged func(context.Context, int64)
	onUnregistered      func(int64)

	transitions registrationTransitionLocks
	background  sync.WaitGroup
	waitingMu   sync.Mutex
	waiting     map[int64]time.Time
	reportMu    sync.Mutex
	reportAfter map[int64]time.Time
}
type registrationRoute struct {
	name       string
	handler    th.Handler
	predicates []th.Predicate
}

func newRegistrationService(
	root context.Context,
	bot *telego.Bot,
	settings *store.Settings,
	cfg *config.Config,
	username string,
	selfID int64,
	onRegistered func(context.Context, int64),
	onMembershipChanged func(context.Context, int64),
	onUnregistered func(int64),
) *registrationService {
	s := &registrationService{
		root:                root,
		bot:                 bot,
		settings:            settings,
		cfg:                 cfg,
		username:            username,
		selfID:              selfID,
		now:                 time.Now,
		onRegistered:        onRegistered,
		onMembershipChanged: onMembershipChanged,
		onUnregistered:      onUnregistered,
		waiting:             make(map[int64]time.Time),
		reportAfter:         make(map[int64]time.Time),
	}
	state := settings.Registrations()
	for _, pending := range state.PendingRegistrations {
		s.scheduleUnknownLeave(pending.GroupID, pending.Title, time.Unix(pending.ExpiresAt, 0))
	}
	for _, leave := range state.UnknownGroupLeaves {
		s.scheduleUnknownLeave(leave.GroupID, leave.Title, time.Unix(leave.ExpiresAt, 0))
	}
	return s
}

// Registration owns Telegram registration routes and their shutdown timers.
type Registration = registrationService

// NewRegistration wires the existing registration protocol to its callbacks.
func NewRegistration(
	root context.Context,
	bot *telego.Bot,
	settings *store.Settings,
	cfg *config.Config,
	username string,
	selfID int64,
	onOwnerClaimed func(context.Context),
	onRegistered func(context.Context, int64),
	onMembershipChanged func(context.Context, int64),
	onUnregistered func(int64),
) *Registration {
	service := newRegistrationService(
		root,
		bot,
		settings,
		cfg,
		username,
		selfID,
		onRegistered,
		onMembershipChanged,
		onUnregistered,
	)
	service.onOwnerClaimed = onOwnerClaimed
	return service
}

// Wait blocks until every registration timer has observed shutdown.
func (s *registrationService) Wait() {
	s.background.Wait()
}

func (s *registrationService) Register(handler *th.BotHandler) {
	for _, route := range s.handlerRoutes() {
		handler.Handle(route.handler, route.predicates...)
	}
}

func (s *registrationService) handlerRoutes() []registrationRoute {
	return []registrationRoute{
		{name: "registration.owner_claim", handler: s.onOwnerClaim, predicates: []th.Predicate{th.And(th.CommandEqual("start"), startPayloadPrefix("owner_"), privateMessage)}},
		{name: "registration.enrollment_start", handler: s.onEnrollmentStart, predicates: []th.Predicate{th.And(th.CommandEqual("start"), startPayloadPrefix("enroll_"))}},
		{name: "registration.enrollment_command", handler: s.onEnrollmentCommand, predicates: []th.Predicate{th.And(th.CommandEqual("enroll"), privateMessage)}},
		{name: "registration.unregister_command", handler: s.onUnregisterCommand, predicates: []th.Predicate{th.And(th.CommandEqual("unregister"), privateMessage)}},
		{name: "registration.unknown_membership", handler: s.onMyChatMember, predicates: []th.Predicate{s.registrationMembershipUpdate}},
		{name: "registration.effective_membership", handler: s.onEffectiveMembershipUpdate, predicates: []th.Predicate{s.effectiveMembershipUpdate}},
	}
}

func privateMessage(_ context.Context, update telego.Update) bool {
	return update.Message != nil && update.Message.Chat.Type == telego.ChatTypePrivate
}

func startPayloadPrefix(prefix string) th.Predicate {
	return func(_ context.Context, update telego.Update) bool {
		return strings.HasPrefix(startPayload(update.Message), prefix)
	}
}

func startPayload(message *telego.Message) string {
	if message == nil {
		return ""
	}
	fields := strings.Fields(message.Text)
	if len(fields) != 2 {
		return ""
	}
	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if command != "start" {
		return ""
	}
	return fields[1]
}

func commandArguments(message *telego.Message, expected string) []string {
	if message == nil {
		return nil
	}
	fields := strings.Fields(message.Text)
	if len(fields) == 0 {
		return nil
	}
	command := strings.TrimPrefix(strings.ToLower(fields[0]), "/")
	if at := strings.IndexByte(command, '@'); at >= 0 {
		command = command[:at]
	}
	if command != expected {
		return nil
	}
	return fields[1:]
}

func (s *registrationService) registrationMembershipUpdate(_ context.Context, update telego.Update) bool {
	membership := update.MyChatMember
	if membership == nil || membership.NewChatMember == nil {
		return false
	}
	if membership.Chat.Type != telego.ChatTypeGroup && membership.Chat.Type != telego.ChatTypeSupergroup {
		return false
	}
	status := membership.NewChatMember.MemberStatus()
	if status == telego.MemberStatusLeft || status == telego.MemberStatusBanned {
		return false
	}
	return !s.cfg.IsKnownChat(membership.Chat.ID) && !s.settings.IsKnownChat(membership.Chat.ID)
}

func (s *registrationService) effectiveMembershipUpdate(_ context.Context, update telego.Update) bool {
	membership := update.MyChatMember
	if membership == nil || membership.OldChatMember == nil || membership.NewChatMember == nil {
		return false
	}
	if membership.Chat.Type != telego.ChatTypeGroup && membership.Chat.Type != telego.ChatTypeSupergroup {
		return false
	}
	return s.settings.IsGroup(membership.Chat.ID) &&
		!reflect.DeepEqual(membership.OldChatMember, membership.NewChatMember)
}

func (s *registrationService) EnsureOwnerClaim() error {
	now := s.now()
	nonce, _, err := s.settings.EnsureOwnerClaim(now, s.cfg.OwnerClaimLifetime())
	if err != nil {
		return err
	}
	if nonce == "" {
		return nil
	}
	state := s.settings.Registrations()
	link := fmt.Sprintf("https://t.me/%s?start=owner_%s", s.username, nonce)
	log.Printf("OWNER UNCLAIMED: claim link is private, one use, and expires %s: %s",
		time.Unix(state.OwnerClaimExpiresAt, 0).UTC().Format(time.RFC3339), link)
	return nil
}

func (s *registrationService) onOwnerClaim(ctx *th.Context, update telego.Update) error {
	message := update.Message
	if message == nil || message.From == nil {
		return nil
	}
	payload := startPayload(message)
	nonce := strings.TrimPrefix(payload, "owner_")
	l := i18n.FromRequester(message.From.LanguageCode, i18n.LangEN)
	if s.cfg.OwnerClaimUserID != 0 && message.From.ID != s.cfg.OwnerClaimUserID {
		_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID),
			i18n.Messages.Bot.Registration.OwnerClaimRefused.For(l)))
		log.Printf("owner claim refused: user=%d error=claim is pinned to another user", message.From.ID)
		return nil
	}
	if err := s.settings.ClaimOwner(message.From.ID, nonce, s.now()); err != nil {
		text := i18n.Messages.Bot.Registration.OwnerClaimRefused.For(l)
		if !errors.Is(err, store.ErrOwnerClaimInvalid) {
			text = i18n.Messages.Bot.Registration.OwnerClaimSaveFailed.For(l)
		}
		_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID), text))
		log.Printf("owner claim refused: user=%d error=%v", message.From.ID, err)
		return nil
	}
	_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID),
		i18n.Messages.Bot.Registration.OwnerClaimed.For(l)))
	log.Printf("owner claimed: user=%d", message.From.ID)
	if s.onOwnerClaimed != nil {
		s.onOwnerClaimed(ctx.Context())
	}
	return nil
}

func (s *registrationService) onEnrollmentCommand(ctx *th.Context, update telego.Update) error {
	message := update.Message
	if message == nil || message.From == nil {
		return nil
	}
	l := i18n.FromRequester(message.From.LanguageCode, i18n.LangEN)
	nonce, err := s.settings.IssueEnrollmentNonce(message.From.ID, s.now(), enrollmentLifetime)
	if err != nil {
		text := i18n.Messages.Bot.Registration.RegistrationSaveFailed.For(l)
		if errors.Is(err, store.ErrRegistrationOwnerOnly) {
			text = i18n.Messages.Bot.Registration.EnrollmentOwnerOnly.For(l)
		}
		_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID), text))
		log.Printf("enrollment link refused: user=%d error=%v", message.From.ID, err)
		return nil
	}
	link := fmt.Sprintf("https://t.me/%s?startgroup=enroll_%s", s.username, nonce.Nonce)
	text := i18n.Messages.Bot.Registration.EnrollmentLink.Render(l, int(enrollmentLifetime/time.Minute), link)
	_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID), text))
	log.Printf("enrollment link issued: owner=%d expires=%s", message.From.ID,
		time.Unix(nonce.ExpiresAt, 0).UTC().Format(time.RFC3339))
	return nil
}

type enrollmentResult struct {
	registered bool
	pending    store.PendingRegistration
}

func (s *registrationService) onEnrollmentStart(ctx *th.Context, update telego.Update) error {
	message := update.Message
	if message == nil || message.From == nil {
		return nil
	}
	if message.Chat.Type != telego.ChatTypeGroup && message.Chat.Type != telego.ChatTypeSupergroup {
		l := i18n.FromRequester(message.From.LanguageCode, i18n.LangEN)
		_, _ = s.bot.SendMessage(ctx.Context(), tu.Message(tu.ID(message.Chat.ID),
			i18n.Messages.Bot.Registration.EnrollmentRefused.For(l)))
		log.Printf("enrollment payload refused outside group: user=%d chat=%d", message.From.ID, message.Chat.ID)
		return nil
	}
	nonce := strings.TrimPrefix(startPayload(message), "enroll_")
	if message.From.IsBot || !s.actorIsAdmin(ctx.Context(), message.Chat.ID, message.From.ID) {
		s.refuseEnrollment(ctx.Context(), message.Chat, message.From.ID, "actor is not a current administrator")
		return nil
	}

	result, err := s.consumeEnrollment(
		ctx.Context(), message.Chat.ID, message.From.ID, groupTitle(message.Chat), nonce,
	)
	if err != nil {
		if errors.Is(err, errEnrollmentInvalid) || errors.Is(err, errBotMembershipUnreadable) ||
			errors.Is(err, errBotMembershipIneligible) {
			s.refuseEnrollment(ctx.Context(), message.Chat, message.From.ID, err.Error())
		} else {
			s.registrationPersistenceFailure(ctx.Context(), message.Chat, *message.From)
		}
		return nil
	}
	if result.registered {
		s.cancelUnknownLeave(message.Chat.ID)
		s.registrationCompleted(ctx.Context(), message.Chat, *message.From)
		return nil
	}
	expires := time.Unix(result.pending.ExpiresAt, 0)
	s.scheduleUnknownLeave(message.Chat.ID, result.pending.Title, expires)
	s.sendRegistrationText(ctx.Context(), message.Chat.ID,
		i18n.Messages.Bot.Registration.RegistrationPending.Render(
			i18n.FromRequester(message.From.LanguageCode, s.groupLanguage(message.Chat.ID)), groupTitle(message.Chat)))
	log.Printf("group registration pending promotion: group=%d actor=%d expires=%s",
		message.Chat.ID, message.From.ID, expires.UTC().Format(time.RFC3339))
	return nil
}

func (s *registrationService) consumeEnrollment(
	ctx context.Context,
	groupID int64,
	actorID int64,
	title string,
	nonce string,
) (enrollmentResult, error) {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	var result enrollmentResult
	_, err := s.mutateRegistrationsWithMembership(
		ctx,
		groupID,
		func(state *store.RegistrationState, membership botMembershipState) (bool, error) {
			if membership != botMembershipAdmin && membership != botMembershipMember {
				return false, errBotMembershipIneligible
			}
			now := s.now()
			index := -1
			for i, candidate := range state.EnrollmentNonces {
				if candidate.Nonce == nonce && candidate.IssuedBy == state.OwnerID &&
					now.Unix() < candidate.ExpiresAt {
					index = i
					break
				}
			}
			if index < 0 {
				return false, errEnrollmentInvalid
			}
			state.EnrollmentNonces = append(state.EnrollmentNonces[:index], state.EnrollmentNonces[index+1:]...)
			result = enrollmentResult{registered: membership == botMembershipAdmin}
			if result.registered {
				s.addRegisteredGroup(state, groupID, actorID, title)
			} else {
				result.pending = store.PendingRegistration{
					GroupID:      groupID,
					RegisteredBy: actorID,
					Title:        title,
					ExpiresAt:    now.Add(registrationPending).Unix(),
				}
				s.putPendingRegistration(state, result.pending)
			}
			return true, nil
		},
	)
	return result, err
}
