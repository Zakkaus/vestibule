package moderate

import (
	"context"
	"strconv"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

func moderationRights(command string) verification.GroupRights {
	return verification.GroupRights{
		CanRestrictMembers: true,
		CanDeleteMessages:  command == "/ban" || command == "/sb" || command == "/mute",
	}
}

func (s *Service) targetFailure(ctx context.Context, groupID, targetID int64, check bool, l i18n.Lang) string {
	if !check {
		return ""
	}
	admin, err := s.telegram.FreshAdmin(ctx, groupID, targetID)
	if err != nil {
		return i18n.Messages.Moderate.Common.TargetAdminCheckFailed.For(l)
	}
	if admin {
		return i18n.Messages.Moderate.Common.TargetIsAdmin.For(l)
	}
	return ""
}

func moderationReplyTarget(msg *telego.Message) *telego.Message {
	reply := msg.ReplyToMessage
	if reply != nil && ((msg.IsTopicMessage && reply.ForumTopicCreated != nil) ||
		(msg.MessageThreadID != 0 && reply.MessageID == msg.MessageThreadID)) {
		return nil
	}
	return reply
}

func (s *Service) runModeration(ctx *th.Context, update telego.Update, command string) error {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.From.ID <= 0 || msg.From.IsBot {
		return nil
	}
	inGroup := s.settings.IsGroup(msg.Chat.ID)
	if inGroup && moderationReplyTarget(msg) != nil {
		return s.runGroupModeration(ctx, msg, command)
	}
	if group, ok := s.settings.ControlGroup(msg.Chat.ID); ok && command != "/sb" {
		return s.runControlModeration(ctx, msg, group.ID(), command)
	}
	if inGroup {
		return s.runGroupModeration(ctx, msg, command)
	}
	return nil
}

func (s *Service) runGroupModeration(ctx *th.Context, msg *telego.Message, command string) error {
	requestCtx := ctx.Context()
	groupID := msg.Chat.ID
	defer s.telegram.Delete(requestCtx, groupID, msg.MessageID)
	l := s.groupLanguage(groupID)
	if !s.requireRights(requestCtx, groupID, msg.From.ID, command, l, moderationRights(command)) {
		return nil
	}
	target := s.warnPrecheck(requestCtx, msg, command, command != "/unmute", l)
	if target == nil {
		return nil
	}
	seconds := s.actionSeconds(groupID, command)
	if command == "/mute" {
		if arg := strings.TrimSpace(commandArg(msg.Text)); arg != "" {
			parsed, ok := parseBanDuration(arg)
			if !ok || parsed <= 0 {
				s.notify(requestCtx, groupID, i18n.Messages.Moderate.Mute.Usage.Render(l, tgfmt.ModerationBanDurationStatus(l, seconds)))
				return nil
			}
			seconds = parsed
		}
	}
	result := s.executeModeration(requestCtx, groupID, msg.From, target, command, seconds, msg.ReplyToMessage.MessageID)
	return s.deliverModeration(ctx, msg, groupID, result, false)
}

func parseControlArguments(text, command string) (int64, string, bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return 0, "", false
	}
	args := fields[1:]
	if len(args) == 0 || len(args) > 2 || (command != "/mute" && len(args) != 1) {
		return 0, "", false
	}
	for _, digit := range args[0] {
		if digit < '0' || digit > '9' {
			return 0, "", false
		}
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", false
	}
	duration := ""
	if len(args) == 2 {
		duration = args[1]
	}
	return id, duration, true
}

func (s *Service) runControlModeration(ctx *th.Context, msg *telego.Message, groupID int64, command string) error {
	l := s.groupLanguage(groupID)
	id, duration, valid := parseControlArguments(msg.Text, command)
	seconds := s.actionSeconds(groupID, command)
	if command == "/mute" && duration != "" {
		var ok bool
		seconds, ok = parseBanDuration(duration)
		valid = valid && ok && seconds > 0
	}
	if !valid {
		usage := i18n.Messages.Moderate.Common.ControlUsage
		if command == "/mute" {
			usage = i18n.Messages.Moderate.Common.ControlMuteUsage
		}
		return s.deliverModeration(ctx, msg, groupID, moderationResult{
			text: usage.Render(l, command),
		}, true)
	}
	notice := s.rightsFailure(ctx.Context(), groupID, msg.From.ID, command, l, verification.GroupRights{CanRestrictMembers: true})
	if notice == "" {
		notice = s.targetFailure(ctx.Context(), groupID, id, command != "/unmute", l)
	}
	if notice != "" {
		return s.deliverModeration(ctx, msg, groupID, moderationResult{text: notice}, true)
	}
	target := &telego.User{ID: id, FirstName: strconv.FormatInt(id, 10)}
	result := s.executeModeration(ctx.Context(), groupID, msg.From, target, command, seconds, 0)
	return s.deliverModeration(ctx, msg, groupID, result, true)
}

func (s *Service) deliverModeration(ctx *th.Context, msg *telego.Message, groupID int64, result moderationResult, remote bool) error {
	var err error
	if remote {
		_, err = ctx.Bot().SendMessage(ctx.Context(), tu.Message(tu.ID(msg.Chat.ID), result.text).
			WithReplyParameters(&telego.ReplyParameters{MessageID: msg.MessageID}))
	} else {
		s.notify(ctx.Context(), groupID, result.text)
	}
	if result.audit != "" {
		s.telegram.AuditLog(ctx.Context(), s.adminLogChatID(groupID), result.audit)
	}
	if result.alert != "" {
		s.telegram.FailAlert(ctx.Context(), s.adminLogChatID(groupID), groupID, result.alert)
	}
	return err
}

func (s *Service) actionSeconds(groupID int64, command string) int {
	switch command {
	case "/ban", "/sb":
		return s.banDuration(groupID)
	case "/mute":
		return s.muteSeconds(groupID)
	default:
		return 0
	}
}
