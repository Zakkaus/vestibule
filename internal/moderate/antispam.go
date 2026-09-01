package moderate

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/ids"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func (s *Service) antispamEnabled(groupID int64) bool {
	group, ok := s.settings.Settings(groupID)
	return ok && group.AntispamEnabled().Value
}

func (s *Service) channelWhitelisted(groupID, senderID int64) bool {
	group, ok := s.settings.Settings(groupID)
	if !ok {
		return false
	}
	for _, allowedID := range group.ChannelWhitelist().Value {
		if allowedID == senderID {
			return true
		}
	}
	return false
}

func (s *Service) isKnownChat(chatID int64) bool {
	if s.settings.IsKnownChat(chatID) {
		return true
	}
	for _, feed := range s.cfg.Feeds {
		if feed.ChatID == chatID {
			return true
		}
	}
	return false
}

func (s *Service) toggleAntispam(groupID int64) (bool, error) {
	group, ok := s.settings.Settings(groupID)
	if !ok {
		return false, fmt.Errorf("%w: %d", settings.ErrUnknownGroup, groupID)
	}
	enabled := !group.AntispamEnabled().Value
	overrides := group.Overrides()
	overrides.AntispamEnabled = &enabled
	_, err := s.settings.Update(groupID, group.Revision(), overrides)
	return enabled, err
}

// UpdateChannelWhitelist applies the shared whitelist bound and unbans newly allowed senders.
func (s *Service) UpdateChannelWhitelist(ctx context.Context, groupID, senderID int64, allow bool) (unbanErr, updateErr error) {
	group, ok := s.settings.Settings(groupID)
	if !ok {
		return nil, fmt.Errorf("%w: %d", settings.ErrUnknownGroup, groupID)
	}
	whitelist := ids.UpdateChannelWhitelist(group.ChannelWhitelist().Value, senderID, allow)
	overrides := group.Overrides()
	overrides.ChannelWhitelist = &whitelist
	if _, err := s.settings.Update(groupID, group.Revision(), overrides); err != nil {
		return nil, err
	}
	if allow {
		return s.telegram.UnbanSenderChat(ctx, groupID, senderID), nil
	}
	return nil, nil
}

// ChannelSenderMessage is the platform-neutral part of a sender-chat message.
type ChannelSenderMessage struct {
	ChatID           int64
	MessageID        int
	SenderChatID     int64
	SenderChatTitle  string
	AutomaticForward bool
}

// FilterChannelSender deletes and, when safely possible, bans an untrusted sender-chat post.
// It reports whether the Telegram adapter must stop routing the update.
func (s *Service) FilterChannelSender(ctx context.Context, message ChannelSenderMessage) bool {
	if !s.settings.IsGroup(message.ChatID) || !s.antispamEnabled(message.ChatID) ||
		message.SenderChatID == 0 ||
		message.SenderChatID == message.ChatID || // Anonymous group admins post as the group itself.
		message.AutomaticForward || // Telegram auto-forwards linked discussion-channel posts.
		s.isKnownChat(message.SenderChatID) ||
		s.channelWhitelisted(message.ChatID, message.SenderChatID) {
		return false
	}

	l := s.groupLanguage(message.ChatID)
	// A discussion group's own channel replying to a comment is not an impersonator.
	linked, linkKnown := s.telegram.LinkedChat(ctx, message.ChatID)
	if linkKnown && linked != 0 && linked == message.SenderChatID {
		return false
	}
	s.telegram.Delete(ctx, message.ChatID, message.MessageID)
	banned := false
	if !linkKnown {
		// A sender-chat ban is permanent and only an administrator can lift it. Never
		// impose one on a guess: delete the advert, and leave the ban for a reading that
		// can rule out this group's own channel.
		log.Printf("antispam: linked channel for %d unknown; deleted message from %d without banning", message.ChatID, message.SenderChatID)
	} else if err := s.telegram.BanSenderChat(ctx, message.ChatID, message.SenderChatID); err != nil {
		log.Printf("antispam: ban sender_chat %d in %d: %v", message.SenderChatID, message.ChatID, err)
	} else {
		banned = true
	}
	s.telegram.Alert(ctx, s.adminLogChatID(message.ChatID),
		tgfmt.ChannelSenderAlert(l, banned, message.SenderChatTitle, message.SenderChatID, message.ChatID))
	log.Printf("antispam: channel sender %d (%q) in group %d deleted, banned=%v", message.SenderChatID, message.SenderChatTitle, message.ChatID, banned)
	return true
}

// ChannelSenderCommand is the platform-neutral data required by /bc.
type ChannelSenderCommand struct {
	ChatID    int64
	MessageID int
	CallerID  int64
	Text      string
}

// BlockChannel handles per-group sender-chat filtering and whitelist changes.
func (s *Service) BlockChannel(ctx context.Context, command ChannelSenderCommand) {
	if command.CallerID == 0 || !s.settings.IsGroup(command.ChatID) {
		return
	}
	l := s.groupLanguage(command.ChatID)
	defer s.telegram.Delete(ctx, command.ChatID, command.MessageID)
	if admin, err := s.isGroupAdmin(ctx, command.ChatID, command.CallerID); !admin {
		s.notify(ctx, command.ChatID, callerRefusal(l, err, i18n.Messages.Moderate.Common.CommandAdminOnly.Render(l, "/bc")))
		return
	}

	fields := strings.Fields(commandArg(command.Text))
	switch {
	case len(fields) == 0:
		enabled, err := s.toggleAntispam(command.ChatID)
		if err != nil {
			s.notifySettingsFailure(ctx, command.ChatID, l, err)
			return
		}
		if enabled {
			s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.Enabled.For(l))
		} else {
			s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.Disabled.For(l))
		}
	case (fields[0] == "allow" || fields[0] == "deny") && len(fields) >= 2:
		senderID, ok := ids.ParseChannelID(fields[1])
		if !ok {
			s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.InvalidChannelID.For(l))
			return
		}
		allow := fields[0] == "allow"
		unbanErr, err := s.UpdateChannelWhitelist(ctx, command.ChatID, senderID, allow)
		if err != nil {
			s.notifySettingsFailure(ctx, command.ChatID, l, err)
			return
		}
		if !allow {
			s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.Removed.Render(l, senderID))
			return
		}
		if unbanErr != nil {
			log.Printf("/bc allow: unban sender_chat %d in %d: %v", senderID, command.ChatID, unbanErr)
			s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.AllowedUnbanFailed.Render(l, senderID))
			return
		}
		s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.Allowed.Render(l, senderID))
	default:
		s.notify(ctx, command.ChatID, i18n.Messages.Moderate.Antispam.Usage.For(l))
	}
}
