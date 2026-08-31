package telegram

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

func (s *registrationService) registrationCompleted(ctx context.Context, chat telego.Chat, actor telego.User) {
	l := i18n.FromRequester(actor.LanguageCode, s.groupLanguage(chat.ID))
	text := i18n.Messages.Bot.Registration.GroupRegistered.Render(l, groupTitle(chat))
	if _, err := s.bot.SendMessage(ctx, tu.Message(tu.ID(actor.ID), text)); err != nil {
		_, _ = s.bot.SendMessage(ctx, tu.Message(tu.ID(chat.ID), text))
	}
	log.Printf("group registered: group=%d actor=%d", chat.ID, actor.ID)
	if s.onRegistered != nil {
		go s.onRegistered(s.root, chat.ID)
	}
}

func (s *registrationService) registrationPersistenceFailure(ctx context.Context, chat telego.Chat, actor telego.User) {
	l := i18n.FromRequester(actor.LanguageCode, s.groupLanguage(chat.ID))
	_, _ = s.bot.SendMessage(ctx, tu.Message(tu.ID(chat.ID),
		i18n.Messages.Bot.Registration.RegistrationSaveFailed.For(l)))
	s.leaveUnknown(ctx, chat, actor.ID, "registration persistence failed")
}

func (s *registrationService) refuseEnrollment(ctx context.Context, chat telego.Chat, actorID int64, reason string) {
	l := s.groupLanguage(chat.ID)
	_, _ = s.bot.SendMessage(ctx, tu.Message(tu.ID(chat.ID),
		i18n.Messages.Bot.Registration.EnrollmentRefused.For(l)))
	s.leaveUnknown(ctx, chat, actorID, reason)
}

func (s *registrationService) sendRegistrationText(ctx context.Context, chatID int64, text string) {
	_, _ = s.bot.SendMessage(ctx, tu.Message(tu.ID(chatID), text))
}

func (s *registrationService) leaveUnknown(ctx context.Context, chat telego.Chat, actorID int64, reason string) bool {
	unlock := s.transitions.lock(chat.ID)
	left := s.leaveUnknownLocked(ctx, chat, actorID, reason)
	unlock()
	if left {
		s.notifyUnauthorizedLeave(ctx, chat)
	}
	return left
}

func (s *registrationService) leaveUnknownLocked(
	ctx context.Context,
	chat telego.Chat,
	actorID int64,
	reason string,
) bool {
	if s.settings.IsKnownChat(chat.ID) || s.cfg.IsKnownChat(chat.ID) {
		_ = s.clearUnknownGroupLeave(chat.ID)
		return false
	}
	log.Printf("group registration refused: group=%d actor=%d reason=%s", chat.ID, actorID, reason)
	if err := s.bot.LeaveChat(ctx, &telego.LeaveChatParams{ChatID: tu.ID(chat.ID)}); err != nil {
		log.Printf("group registration refusal leave failed: group=%d error=%v", chat.ID, err)
		return false
	}
	if err := s.clearUnknownGroupLeave(chat.ID); err != nil {
		log.Printf("unknown-group leave cleanup commit failed: group=%d error=%v", chat.ID, err)
	}
	return true
}

func (s *registrationService) clearUnknownGroupLeave(groupID int64) error {
	if _, ok := unknownGroupLeave(s.settings.Registrations(), groupID); !ok {
		return nil
	}
	return s.mutateRegistrations(func(state *store.RegistrationState) error {
		removeUnknownGroupLeave(state, groupID)
		return nil
	})
}

func (s *registrationService) notifyUnauthorizedLeave(ctx context.Context, chat telego.Chat) {
	if s.cfg.AdminLogChatID == 0 {
		return
	}
	l := i18n.FromStored(s.cfg.LangForGroup(0))
	_, _ = s.bot.SendMessage(ctx, tu.Message(tu.ID(s.cfg.AdminLogChatID),
		i18n.Messages.Bot.Lifecycle.UnauthorizedChat.Render(l, chat.Title, chat.ID, chat.Type)))
}

func (s *registrationService) cancelUnknownLeave(groupID int64) {
	s.waitingMu.Lock()
	delete(s.waiting, groupID)
	s.waitingMu.Unlock()
}

func (s *registrationService) scheduleUnknownLeave(groupID int64, title string, deadline time.Time) {
	s.waitingMu.Lock()
	if current, ok := s.waiting[groupID]; ok && !deadline.After(current) {
		s.waitingMu.Unlock()
		return
	}
	s.waiting[groupID] = deadline
	s.waitingMu.Unlock()

	s.background.Add(1)
	go func() {
		defer s.background.Done()
		delay := time.Until(deadline)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-s.root.Done():
			return
		case <-timer.C:
		}
		s.waitingMu.Lock()
		current, ok := s.waiting[groupID]
		if !ok || !current.Equal(deadline) {
			s.waitingMu.Unlock()
			return
		}
		delete(s.waiting, groupID)
		s.waitingMu.Unlock()
		s.handleUnknownLeaveDeadline(groupID, title)
	}()
}

func (s *registrationService) handleUnknownLeaveDeadline(groupID int64, title string) {
	unlock := s.transitions.lock(groupID)
	state := s.settings.Registrations()
	now := s.now()
	if pending, ok := pendingRegistration(state, groupID); ok {
		if now.Unix() < pending.ExpiresAt {
			unlock()
			s.scheduleUnknownLeave(groupID, pending.Title, time.Unix(pending.ExpiresAt, 0))
			return
		}
		if err := s.mutateRegistrations(func(next *store.RegistrationState) error {
			current, exists := pendingRegistration(*next, groupID)
			if !exists || current.ExpiresAt != pending.ExpiresAt {
				return nil
			}
			removePendingRegistration(next, groupID)
			s.putUnknownGroupLeave(next, store.UnknownGroupLeave{
				GroupID: groupID, Title: current.Title, ExpiresAt: current.ExpiresAt,
			})
			return nil
		}); err != nil {
			log.Printf("pending registration expiry commit failed: group=%d error=%v", groupID, err)
		}
		state = s.settings.Registrations()
	}
	if leave, ok := unknownGroupLeave(state, groupID); ok {
		if now.Unix() < leave.ExpiresAt {
			unlock()
			s.scheduleUnknownLeave(groupID, leave.Title, time.Unix(leave.ExpiresAt, 0))
			return
		}
		if leave.Title != "" {
			title = leave.Title
		}
	}
	chat := telego.Chat{ID: groupID, Type: telego.ChatTypeSupergroup, Title: title}
	left := s.leaveUnknownLocked(context.Background(), chat, 0, "registration grace period expired")
	unlock()
	if left {
		s.notifyUnauthorizedLeave(context.Background(), chat)
	}
}

func (s *registrationService) groupLanguage(groupID int64) i18n.Lang {
	if group, ok := s.settings.Group(groupID); ok {
		return i18n.FromStored(group.Lang().Value)
	}
	return i18n.FromStored(s.cfg.LangForGroup(groupID))
}

func groupTitle(chat telego.Chat) string {
	if chat.Title != "" {
		return chat.Title
	}
	return strconv.FormatInt(chat.ID, 10)
}
