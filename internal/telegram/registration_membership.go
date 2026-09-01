package telegram

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func (s *registrationService) onMyChatMember(ctx *th.Context, update telego.Update) error {
	membership := update.MyChatMember
	if membership == nil {
		return nil
	}
	actor := membership.From
	if actor.ID <= 0 || actor.IsBot || !s.actorIsAdmin(ctx.Context(), membership.Chat.ID, actor.ID) {
		s.leaveUnknown(ctx.Context(), membership.Chat, actor.ID, "membership actor is not a current human administrator")
		return nil
	}
	now := s.now()
	state := s.settings.Registrations()
	if pending, ok := pendingRegistration(state, membership.Chat.ID); ok {
		s.handlePendingMembership(ctx.Context(), *membership, actor, pending, now)
		return nil
	}
	if state.OwnerID != 0 && actor.ID == state.OwnerID &&
		s.handleOwnerMembership(ctx.Context(), *membership, actor, now) {
		return nil
	}
	if membership.NewChatMember.MemberStatus() == telego.MemberStatusMember && state.OwnerID != 0 &&
		s.handleUnknownMembership(ctx.Context(), *membership, actor, now) {
		return nil
	}
	s.leaveUnknown(ctx.Context(), membership.Chat, actor.ID, "owner or enrollment authorization is required")
	return nil
}

func (s *registrationService) handlePendingMembership(
	ctx context.Context,
	membership telego.ChatMemberUpdated,
	actor telego.User,
	pending settings.PendingRegistration,
	now time.Time,
) {
	if now.Unix() >= pending.ExpiresAt {
		if err := s.expirePending(membership.Chat.ID, pending); err != nil {
			log.Printf("pending registration cleanup failed: group=%d error=%v", membership.Chat.ID, err)
		}
		s.leaveUnknown(ctx, membership.Chat, actor.ID, "pending registration expired")
		return
	}
	status, completed, err := s.completePending(ctx, membership.Chat.ID, pending)
	if err != nil {
		if errors.Is(err, errBotMembershipUnreadable) || errors.Is(err, errEnrollmentInvalid) {
			s.leaveUnknown(ctx, membership.Chat, actor.ID, err.Error())
		} else {
			s.registrationPersistenceFailure(ctx, membership.Chat, actor)
		}
		return
	}
	switch {
	case completed:
		s.cancelUnknownLeave(membership.Chat.ID)
		s.registrationCompleted(ctx, membership.Chat, actor)
	case status == botMembershipMember:
		s.scheduleUnknownLeave(membership.Chat.ID, pending.Title, time.Unix(pending.ExpiresAt, 0))
	default:
		s.leaveUnknown(ctx, membership.Chat, actor.ID, "pending group has ineligible bot status")
	}
}

func (s *registrationService) handleOwnerMembership(
	ctx context.Context,
	membership telego.ChatMemberUpdated,
	actor telego.User,
	now time.Time,
) bool {
	status, pending, err := s.authorizeOwnerMembership(
		ctx, membership.Chat.ID, actor.ID, groupTitle(membership.Chat), now,
	)
	if err != nil {
		if errors.Is(err, errBotMembershipUnreadable) || errors.Is(err, errBotMembershipIneligible) {
			s.leaveUnknown(ctx, membership.Chat, actor.ID, err.Error())
		} else {
			s.registrationPersistenceFailure(ctx, membership.Chat, actor)
		}
		return true
	}
	if status == botMembershipAdmin {
		s.cancelUnknownLeave(membership.Chat.ID)
		s.registrationCompleted(ctx, membership.Chat, actor)
		return true
	}
	if status != botMembershipMember {
		return false
	}
	s.scheduleUnknownLeave(membership.Chat.ID, pending.Title, time.Unix(pending.ExpiresAt, 0))
	s.sendRegistrationText(ctx, actor.ID,
		i18n.Messages.Bot.Registration.RegistrationPending.Render(
			i18n.FromRequester(actor.LanguageCode, s.groupLanguage(membership.Chat.ID)), groupTitle(membership.Chat)))
	return true
}

func (s *registrationService) handleUnknownMembership(
	ctx context.Context,
	membership telego.ChatMemberUpdated,
	actor telego.User,
	now time.Time,
) bool {
	leave, status, err := s.persistUnknownLeaveIfMember(
		ctx, membership.Chat.ID, groupTitle(membership.Chat), now.Add(registrationPending),
	)
	if errors.Is(err, errUnknownLeaveSuperseded) {
		return true
	}
	if err != nil {
		log.Printf("unknown-group leave persistence failed: group=%d error=%v", membership.Chat.ID, err)
		s.leaveUnknown(ctx, membership.Chat, actor.ID, "unknown-group leave persistence failed")
		return true
	}
	if status != botMembershipMember {
		return false
	}
	s.scheduleUnknownLeave(leave.GroupID, leave.Title, time.Unix(leave.ExpiresAt, 0))
	log.Printf("unknown group awaiting enrollment payload: group=%d actor=%d", membership.Chat.ID, actor.ID)
	return true
}

func (s *registrationService) onEffectiveMembershipUpdate(ctx *th.Context, update telego.Update) error {
	membership := update.MyChatMember
	if membership == nil || s.onMembershipChanged == nil {
		return nil
	}
	now := s.now()
	s.reportMu.Lock()
	if after, ok := s.reportAfter[membership.Chat.ID]; ok && now.Before(after) {
		s.reportMu.Unlock()
		return nil
	}
	s.reportAfter[membership.Chat.ID] = now.Add(setupReportCooldown)
	s.reportMu.Unlock()
	s.onMembershipChanged(ctx.Context(), membership.Chat.ID)
	return nil
}

func (s *registrationService) onUnregisterCommand(ctx *th.Context, update telego.Update) error {
	message := update.Message
	if message == nil || message.From == nil {
		return nil
	}
	l := i18n.FromRequester(message.From.LanguageCode, i18n.LangEN)
	state := s.settings.Registrations()
	if state.OwnerID == 0 || message.From.ID != state.OwnerID {
		s.sendRegistrationText(ctx.Context(), message.Chat.ID,
			i18n.Messages.Bot.Registration.UnregisterOwnerOnly.For(l))
		return nil
	}
	arguments := commandArguments(message, "unregister")
	if len(arguments) != 1 {
		s.sendRegistrationText(ctx.Context(), message.Chat.ID,
			i18n.Messages.Bot.Registration.UnregisterRefused.For(l))
		return nil
	}
	groupID, err := strconv.ParseInt(arguments[0], 10, 64)
	if err != nil || groupID == 0 {
		s.sendRegistrationText(ctx.Context(), message.Chat.ID,
			i18n.Messages.Bot.Registration.UnregisterRefused.For(l))
		return nil
	}
	title, err := s.unregisterGroup(groupID)
	if err != nil {
		text := i18n.Messages.Bot.Registration.UnregisterSaveFailed.For(l)
		if errors.Is(err, errRuntimeGroupNotFound) {
			text = i18n.Messages.Bot.Registration.UnregisterRefused.For(l)
		}
		s.sendRegistrationText(ctx.Context(), message.Chat.ID, text)
		log.Printf("group unregister refused: group=%d owner=%d error=%v", groupID, message.From.ID, err)
		return nil
	}
	if s.onUnregistered != nil {
		s.onUnregistered(groupID)
	}
	s.leaveUnknown(ctx.Context(), telego.Chat{
		ID: groupID, Type: telego.ChatTypeSupergroup, Title: title,
	}, message.From.ID, "owner unregistered group")
	s.sendRegistrationText(ctx.Context(), message.Chat.ID,
		i18n.Messages.Bot.Registration.GroupUnregistered.Render(l, title))
	log.Printf("group unregistered: group=%d owner=%d", groupID, message.From.ID)
	return nil
}

func (s *registrationService) actorIsAdmin(ctx context.Context, groupID, actorID int64) bool {
	member, err := s.bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: tu.ID(groupID), UserID: actorID})
	if err != nil || member == nil {
		return false
	}
	status := member.MemberStatus()
	return status == telego.MemberStatusAdministrator || status == telego.MemberStatusCreator
}

func (s *registrationService) currentBotMembership(ctx context.Context, groupID int64) (botMembershipState, error) {
	member, err := s.bot.GetChatMember(ctx, &telego.GetChatMemberParams{
		ChatID: tu.ID(groupID),
		UserID: s.selfID,
	})
	if err != nil || member == nil {
		return botMembershipIneligible, errBotMembershipUnreadable
	}
	switch member.MemberStatus() {
	case telego.MemberStatusAdministrator, telego.MemberStatusCreator:
		return botMembershipAdmin, nil
	case telego.MemberStatusMember:
		return botMembershipMember, nil
	default:
		return botMembershipIneligible, nil
	}
}

func (s *registrationService) authorizeOwnerMembership(
	ctx context.Context,
	groupID int64,
	actorID int64,
	title string,
	now time.Time,
) (botMembershipState, settings.PendingRegistration, error) {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	pending := settings.PendingRegistration{
		GroupID:      groupID,
		RegisteredBy: actorID,
		Title:        title,
		ExpiresAt:    now.Add(registrationPending).Unix(),
	}
	status, err := s.mutateRegistrationsWithMembership(
		ctx,
		groupID,
		func(state *settings.RegistrationState, membership botMembershipState) (bool, error) {
			if membership != botMembershipAdmin && membership != botMembershipMember {
				return false, errBotMembershipIneligible
			}
			if membership == botMembershipAdmin {
				s.addRegisteredGroup(state, groupID, actorID, title)
			} else {
				s.putPendingRegistration(state, pending)
			}
			return true, nil
		},
	)
	return status, pending, err
}

func (s *registrationService) addRegisteredGroup(state *settings.RegistrationState, groupID, actorID int64, title string) {
	registered := false
	for _, group := range state.RegisteredGroups {
		if group.ID == groupID {
			registered = true
			break
		}
	}
	if !registered {
		state.RegisteredGroups = append(state.RegisteredGroups, settings.RegisteredGroup{
			ID:           groupID,
			RegisteredBy: actorID,
			Title:        title,
		})
	}
	removePendingRegistration(state, groupID)
	removeUnknownGroupLeave(state, groupID)
}

func (s *registrationService) putPendingRegistration(state *settings.RegistrationState, pending settings.PendingRegistration) {
	removeUnknownGroupLeave(state, pending.GroupID)
	for i := range state.PendingRegistrations {
		if state.PendingRegistrations[i].GroupID == pending.GroupID {
			state.PendingRegistrations[i] = pending
			return
		}
	}
	state.PendingRegistrations = append(state.PendingRegistrations, pending)
}

func pendingRegistration(state settings.RegistrationState, groupID int64) (settings.PendingRegistration, bool) {
	for _, pending := range state.PendingRegistrations {
		if pending.GroupID == groupID {
			return pending, true
		}
	}
	return settings.PendingRegistration{}, false
}

func removePendingRegistration(state *settings.RegistrationState, groupID int64) {
	for i, pending := range state.PendingRegistrations {
		if pending.GroupID == groupID {
			state.PendingRegistrations = append(state.PendingRegistrations[:i], state.PendingRegistrations[i+1:]...)
			return
		}
	}
}

func (s *registrationService) completePending(
	ctx context.Context,
	groupID int64,
	pending settings.PendingRegistration,
) (botMembershipState, bool, error) {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	completed := false
	status, err := s.mutateRegistrationsWithMembership(
		ctx,
		groupID,
		func(state *settings.RegistrationState, membership botMembershipState) (bool, error) {
			completed = false
			if membership != botMembershipAdmin {
				return false, nil
			}
			current, ok := pendingRegistration(*state, groupID)
			if !ok || current.ExpiresAt != pending.ExpiresAt || s.now().Unix() >= current.ExpiresAt {
				return false, errEnrollmentInvalid
			}
			s.addRegisteredGroup(state, groupID, current.RegisteredBy, current.Title)
			completed = true
			return true, nil
		},
	)
	return status, completed, err
}

func (s *registrationService) expirePending(groupID int64, expected settings.PendingRegistration) error {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	return s.mutateRegistrations(func(state *settings.RegistrationState) error {
		current, ok := pendingRegistration(*state, groupID)
		if !ok || current.ExpiresAt != expected.ExpiresAt {
			return nil
		}
		removePendingRegistration(state, groupID)
		s.putUnknownGroupLeave(state, settings.UnknownGroupLeave{
			GroupID:   groupID,
			Title:     current.Title,
			ExpiresAt: current.ExpiresAt,
		})
		return nil
	})
}

func unknownGroupLeave(state settings.RegistrationState, groupID int64) (settings.UnknownGroupLeave, bool) {
	for _, leave := range state.UnknownGroupLeaves {
		if leave.GroupID == groupID {
			return leave, true
		}
	}
	return settings.UnknownGroupLeave{}, false
}

func (s *registrationService) putUnknownGroupLeave(state *settings.RegistrationState, leave settings.UnknownGroupLeave) {
	for i := range state.UnknownGroupLeaves {
		if state.UnknownGroupLeaves[i].GroupID == leave.GroupID {
			if leave.ExpiresAt > state.UnknownGroupLeaves[i].ExpiresAt {
				state.UnknownGroupLeaves[i] = leave
			}
			return
		}
	}
	state.UnknownGroupLeaves = append(state.UnknownGroupLeaves, leave)
}

func removeUnknownGroupLeave(state *settings.RegistrationState, groupID int64) {
	for i, leave := range state.UnknownGroupLeaves {
		if leave.GroupID == groupID {
			state.UnknownGroupLeaves = append(state.UnknownGroupLeaves[:i], state.UnknownGroupLeaves[i+1:]...)
			return
		}
	}
}

func (s *registrationService) persistUnknownLeaveIfMember(
	ctx context.Context,
	groupID int64,
	title string,
	deadline time.Time,
) (settings.UnknownGroupLeave, botMembershipState, error) {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	leave := settings.UnknownGroupLeave{GroupID: groupID, Title: title, ExpiresAt: deadline.Unix()}
	status, err := s.mutateRegistrationsWithMembership(
		ctx,
		groupID,
		func(state *settings.RegistrationState, membership botMembershipState) (bool, error) {
			if membership != botMembershipMember {
				return false, nil
			}
			if _, ok := pendingRegistration(*state, groupID); ok {
				return false, errUnknownLeaveSuperseded
			}
			for _, registered := range state.RegisteredGroups {
				if registered.ID == groupID {
					return false, errUnknownLeaveSuperseded
				}
			}
			s.putUnknownGroupLeave(state, leave)
			if current, ok := unknownGroupLeave(*state, groupID); ok {
				leave = current
			}
			return true, nil
		},
	)
	return leave, status, err
}

func (s *registrationService) unregisterGroup(groupID int64) (string, error) {
	unlock := s.transitions.lock(groupID)
	defer unlock()
	title := ""
	err := s.mutateRegistrations(func(state *settings.RegistrationState) error {
		index := -1
		for i, group := range state.RegisteredGroups {
			if group.ID == groupID {
				index = i
				title = group.Title
				break
			}
		}
		if index < 0 {
			return errRuntimeGroupNotFound
		}
		state.RegisteredGroups = append(state.RegisteredGroups[:index], state.RegisteredGroups[index+1:]...)
		removePendingRegistration(state, groupID)
		s.putUnknownGroupLeave(state, settings.UnknownGroupLeave{
			GroupID:   groupID,
			Title:     title,
			ExpiresAt: s.now().Unix(),
		})
		return nil
	})
	if title == "" {
		title = strconv.FormatInt(groupID, 10)
	}
	if err == nil {
		s.reportMu.Lock()
		delete(s.reportAfter, groupID)
		s.reportMu.Unlock()
	}
	return title, err
}

func (s *registrationService) mutateRegistrations(mutate func(*settings.RegistrationState) error) error {
	for {
		current := s.settings.Registrations()
		next := cloneRegistrationMutation(current)
		if err := mutate(&next); err != nil {
			return err
		}
		if _, err := s.settings.CommitRegistrations(current.Revision, next); errors.Is(err, settings.ErrSettingsConflict) {
			continue
		} else {
			return err
		}
	}
}

func (s *registrationService) mutateRegistrationsWithMembership(
	ctx context.Context,
	groupID int64,
	mutate func(*settings.RegistrationState, botMembershipState) (bool, error),
) (botMembershipState, error) {
	for {
		current := s.settings.Registrations()
		next := cloneRegistrationMutation(current)
		membership, err := s.currentBotMembership(ctx, groupID)
		if err != nil {
			return membership, err
		}
		commit, err := mutate(&next, membership)
		if err != nil || !commit {
			return membership, err
		}
		if _, err := s.settings.CommitRegistrations(current.Revision, next); errors.Is(err, settings.ErrSettingsConflict) {
			continue
		} else {
			return membership, err
		}
	}
}

func cloneRegistrationMutation(current settings.RegistrationState) settings.RegistrationState {
	next := current
	next.RegisteredGroups = append([]settings.RegisteredGroup(nil), current.RegisteredGroups...)
	next.EnrollmentNonces = append([]settings.EnrollmentNonce(nil), current.EnrollmentNonces...)
	next.PendingRegistrations = append([]settings.PendingRegistration(nil), current.PendingRegistrations...)
	next.UnknownGroupLeaves = append([]settings.UnknownGroupLeave(nil), current.UnknownGroupLeaves...)
	return next
}
