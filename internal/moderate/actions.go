package moderate

import (
	"context"
	"log"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
)

type moderationResult struct {
	text  string
	audit string
	alert string
}

// executeModeration applies both reply-target and control-chat actions to the protected group.
func (s *Service) executeModeration(ctx context.Context, groupID int64, actor, target *telego.User, command string, seconds, evidenceID int) moderationResult {
	switch command {
	case "/ban", "/sb":
		return s.applyBan(ctx, groupID, actor, target, command, seconds, evidenceID)
	case "/mute":
		return s.applyMute(ctx, groupID, actor, target, seconds, evidenceID)
	case "/unmute":
		return s.applyUnmute(ctx, groupID, actor, target)
	case "/warn":
		return s.applyWarn(ctx, groupID, actor, target)
	}
	return moderationResult{}
}

func (s *Service) applyBan(ctx context.Context, groupID int64, actor, target *telego.User, command string, seconds, evidenceID int) moderationResult {
	l := s.groupLanguage(groupID)
	if err := s.telegram.Ban(ctx, groupID, target.ID, seconds, command == "/sb"); err != nil {
		log.Printf("%s ban user=%d in %d: %v", command, target.ID, groupID, err)
		return moderationResult{
			text:  i18n.Messages.Moderate.Ban.Failed.For(l),
			alert: i18n.Messages.Moderate.Ban.FailureAlert.Render(l, command, groupID, target.ID, tgfmt.DisplayName(target), tgfmt.DisplayName(actor)),
		}
	}
	if evidenceID != 0 {
		s.telegram.Delete(ctx, groupID, evidenceID)
	}
	verb := i18n.Messages.Moderate.Ban.Verb.For(l)
	if command == "/sb" {
		verb = i18n.Messages.Moderate.Ban.PurgeVerb.For(l)
	}
	action := i18n.Messages.Moderate.Ban.Action.Render(l, verb, tgfmt.ModerationBanDurationStatus(l, seconds))
	log.Printf("%s by admin=%d target=%d group=%d ban_secs=%d", command, actor.ID, target.ID, groupID, seconds)
	return moderationResult{
		text:  i18n.Messages.Moderate.Ban.Applied.Render(l, action, tgfmt.DisplayName(target), target.ID, tgfmt.DisplayName(actor)),
		audit: i18n.Messages.Moderate.Ban.Alert.Render(l, command, action, groupID, target.ID, tgfmt.DisplayName(target), tgfmt.DisplayName(actor)),
	}
}

func (s *Service) applyMute(ctx context.Context, groupID int64, actor, target *telego.User, seconds, evidenceID int) moderationResult {
	l := s.groupLanguage(groupID)
	if err := s.telegram.Mute(ctx, groupID, target.ID, seconds); err != nil {
		log.Printf("/mute user=%d in %d: %v", target.ID, groupID, err)
		failure := i18n.Messages.Moderate.Mute.Failed.For(l)
		alert := failure + "\n" + i18n.Messages.Moderate.Mute.Alert.Render(
			l, tgfmt.ModerationBanDurationStatus(l, seconds), groupID, target.ID, tgfmt.DisplayName(target), tgfmt.DisplayName(actor))
		return moderationResult{text: failure, alert: alert}
	}
	if evidenceID != 0 {
		s.telegram.Delete(ctx, groupID, evidenceID)
	}
	log.Printf("/mute by admin=%d target=%d group=%d secs=%d", actor.ID, target.ID, groupID, seconds)
	return moderationResult{
		text: i18n.Messages.Moderate.Mute.Applied.Render(l,
			tgfmt.DisplayName(target), target.ID, tgfmt.ModerationBanDurationStatus(l, seconds), tgfmt.DisplayName(actor)),
		audit: i18n.Messages.Moderate.Mute.Alert.Render(l, tgfmt.ModerationBanDurationStatus(l, seconds), groupID, target.ID, tgfmt.DisplayName(target), tgfmt.DisplayName(actor)),
	}
}

func (s *Service) applyUnmute(ctx context.Context, groupID int64, actor, target *telego.User) moderationResult {
	l := s.groupLanguage(groupID)
	if err := s.telegram.Unmute(ctx, groupID, target.ID); err != nil {
		log.Printf("/unmute user=%d in %d: %v", target.ID, groupID, err)
		return moderationResult{text: i18n.Messages.Moderate.Mute.UnmuteFailed.For(l)}
	}
	log.Printf("/unmute by admin=%d target=%d group=%d", actor.ID, target.ID, groupID)
	return moderationResult{text: i18n.Messages.Moderate.Mute.Unmuted.Render(l, tgfmt.DisplayName(target), target.ID, tgfmt.DisplayName(actor))}
}

func (s *Service) applyWarn(ctx context.Context, groupID int64, actor, target *telego.User) moderationResult {
	l := s.groupLanguage(groupID)
	limit := s.warnLimit(groupID)
	count := s.warnings.increment(groupID, target.ID)
	// Persist before a limit kick so a failed kick survives restart.
	if err := s.warnings.save(); err != nil {
		log.Printf("moderate: warning state save failed for group %d: %v", groupID, err)
	}
	if count >= limit {
		rejoinable, err := s.warnKick(ctx, groupID, target.ID)
		if err != nil {
			log.Printf("/warn kick %d in %d: %v", target.ID, groupID, err)
			return moderationResult{
				text:  i18n.Messages.Moderate.Warning.LimitKickFailed.For(l),
				alert: i18n.Messages.Moderate.Warning.LimitKickAlert.Render(l, tgfmt.DisplayName(target), limit, tgfmt.DisplayName(actor)),
			}
		}
		s.warnings.clear(groupID, target.ID)
		if err := s.warnings.save(); err != nil {
			log.Printf("moderate: warning state save failed for group %d: %v", groupID, err)
		}
		outcome := i18n.Messages.Moderate.Warning.KickRejoinable.For(l)
		if !rejoinable {
			outcome = i18n.Messages.Moderate.Warning.KickUnbanFailed.For(l)
		}
		log.Printf("/warn-kick user=%d group=%d by=%d", target.ID, groupID, actor.ID)
		return moderationResult{
			text:  i18n.Messages.Moderate.Warning.LimitReached.Render(l, tgfmt.DisplayName(target), limit, outcome, tgfmt.DisplayName(actor)),
			audit: i18n.Messages.Moderate.Warning.KickAlert.Render(l, groupID, target.ID, tgfmt.DisplayName(target), tgfmt.DisplayName(actor)),
		}
	}
	log.Printf("/warn user=%d group=%d count=%d by=%d", target.ID, groupID, count, actor.ID)
	return moderationResult{text: i18n.Messages.Moderate.Warning.Issued.Render(l, tgfmt.DisplayName(target), count, limit, limit, tgfmt.DisplayName(actor))}
}
