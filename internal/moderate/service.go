package moderate

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Telegram is the caller-owned transport used for moderation and authorization.
type Telegram interface {
	Delete(ctx context.Context, chatID int64, messageID int)
	Notify(ctx context.Context, chatID int64, text string, ttlSeconds int)
	Alert(ctx context.Context, adminLogChatID int64, text string)
	AuditLog(ctx context.Context, adminLogChatID int64, text string)
	FailAlert(ctx context.Context, adminLogChatID, groupID int64, text string)
	// The architecture document forbids a cached lookup for the target of a sensitive
	// command, so this package cannot reach one: a revoked administrator must stop being
	// protected the moment Telegram says so, not up to a cache lifetime later.
	FreshAdmin(ctx context.Context, chatID, userID int64) (bool, error)
	FreshRights(ctx context.Context, chatID, userID int64) (verification.GroupRights, error)
	Ban(ctx context.Context, chatID, userID int64, seconds int, revokeMessages bool) error
	Unban(ctx context.Context, chatID, userID int64, onlyIfBanned bool) error
	Mute(ctx context.Context, chatID, userID int64, seconds int) error
	Unmute(ctx context.Context, chatID, userID int64) error
	LinkedChat(ctx context.Context, chatID int64) (linked int64, known bool)
	BanSenderChat(ctx context.Context, chatID, senderChatID int64) error
	UnbanSenderChat(ctx context.Context, chatID, senderChatID int64) error
}

// MemberLookup reads Telegram chat and membership records for startup permission diagnostics.
type MemberLookup interface {
	GetChat(ctx context.Context, params *telego.GetChatParams) (*telego.ChatFullInfo, error)
	GetChatMember(ctx context.Context, params *telego.GetChatMemberParams) (telego.ChatMember, error)
	SendMessage(ctx context.Context, params *telego.SendMessageParams) (*telego.Message, error)
}

// Service owns moderation handlers, policy, and warning state.
type Service struct {
	settings *settings.Store
	telegram Telegram
	cfg      *settings.Config
	warnings warningState
}

// New constructs a moderation service and restores its durable warning counters.
func New(settings *settings.Store, telegram Telegram, cfg *settings.Config, warningStore WarningStore) (*Service, error) {
	s := &Service{
		settings: settings,
		telegram: telegram,
		cfg:      cfg,
		warnings: newWarningState(warningStore),
	}
	if err := s.warnings.load(); err != nil {
		return nil, fmt.Errorf("restore warning counters: %w", err)
	}
	return s, nil
}

// SetupReport is one complete startup permission result for a guarded group.
type SetupReport struct {
	GroupID int64
	Ready   bool
	Text    string
}

// CheckGroupSetup checks every Telegram capability required by one guarded group.
func (s *Service) CheckGroupSetup(ctx context.Context, bot MemberLookup, selfID, groupID int64) SetupReport {
	l := s.groupLanguage(groupID)
	messages := i18n.Messages.Moderate.Setup
	title := strconv.FormatInt(groupID, 10)
	var missing []string
	if chat, err := bot.GetChat(ctx, &telego.GetChatParams{ChatID: tu.ID(groupID)}); err != nil || chat == nil {
		missing = append(missing, messages.GroupAccess.For(l))
	} else if chat.Title != "" {
		title = chat.Title
	}

	missingAllGroupRights := func() {
		missing = append(missing,
			messages.GroupAdmin.For(l),
			messages.ApproveJoinRequests.For(l),
			messages.BanUsers.For(l),
			messages.DeleteMessages.For(l),
		)
	}
	member, err := bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: tu.ID(groupID), UserID: selfID})
	if err != nil {
		missingAllGroupRights()
	} else {
		switch typed := member.(type) {
		case *telego.ChatMemberOwner:
		case *telego.ChatMemberAdministrator:
			if !typed.CanInviteUsers {
				missing = append(missing, messages.ApproveJoinRequests.For(l))
			}
			if !typed.CanRestrictMembers {
				missing = append(missing, messages.BanUsers.For(l))
			}
			if !typed.CanDeleteMessages {
				missing = append(missing, messages.DeleteMessages.For(l))
			}
		default:
			missingAllGroupRights()
		}
	}

	group, ok := s.settings.Settings(groupID)
	if ok {
		channelID := group.RequiredChannelID().Value
		if channelID != 0 {
			channelTitle := strconv.FormatInt(channelID, 10)
			if channel, channelErr := bot.GetChat(ctx, &telego.GetChatParams{ChatID: tu.ID(channelID)}); channelErr == nil && channel != nil && channel.Title != "" {
				channelTitle = channel.Title
			}
			channelMember, channelErr := bot.GetChatMember(ctx, &telego.GetChatMemberParams{ChatID: tu.ID(channelID), UserID: selfID})
			status := ""
			if channelMember != nil {
				status = channelMember.MemberStatus()
			}
			if channelErr != nil || (status != telego.MemberStatusCreator && status != telego.MemberStatusAdministrator) {
				missing = append(missing, messages.ChannelAdmin.Render(l, channelTitle, channelID))
			}
		}
	}

	if len(missing) == 0 {
		return SetupReport{GroupID: groupID, Ready: true, Text: messages.Ready.Render(l, title, groupID)}
	}
	text := messages.MissingHeader.Render(l, title, groupID) + "\n- " +
		strings.Join(missing, "\n- ") + "\n" + messages.Restart.For(l)
	return SetupReport{GroupID: groupID, Text: text}
}

// LogGroupSetup emits and delivers one actionable setup report for a guarded group.
func (s *Service) LogGroupSetup(ctx context.Context, bot MemberLookup, selfID, groupID int64) {
	report := s.CheckGroupSetup(ctx, bot, selfID, groupID)
	deliveredTo := int64(0)
	var deliveryErr error
	if !report.Ready {
		registrantID := int64(0)
		for _, group := range s.settings.Registrations().RegisteredGroups {
			if group.ID == groupID {
				registrantID = group.RegisteredBy
				break
			}
		}
		targets := []int64{registrantID, s.adminLogChatID(groupID), groupID}
		seen := make(map[int64]bool, len(targets))
		for _, target := range targets {
			if target == 0 || seen[target] {
				continue
			}
			seen[target] = true
			if _, err := bot.SendMessage(ctx, tu.Message(tu.ID(target), report.Text)); err == nil {
				deliveredTo = target
				deliveryErr = nil
				break
			} else {
				deliveryErr = err
			}
		}
	}
	log.Printf("group setup: group=%d ready=%t delivered_to=%d delivery_error=%v report=%q",
		groupID, report.Ready, deliveredTo, deliveryErr, report.Text)
}

// LogGroupAdmin emits and delivers exactly one actionable setup report per guarded group.
func (s *Service) LogGroupAdmin(ctx context.Context, bot MemberLookup, selfID int64) {
	for _, groupID := range s.settings.ChatIDs() {
		s.LogGroupSetup(ctx, bot, selfID, groupID)
	}
}

// Both moderation limits follow the group's own setting, falling back to the configured
// default for a chat the settings store does not know.
func (s *Service) warnLimit(groupID int64) int {
	if group, ok := s.settings.Settings(groupID); ok {
		return group.WarnLimit().Value
	}
	return s.cfg.WarnLimit
}

func (s *Service) muteSeconds(groupID int64) int {
	if group, ok := s.settings.Settings(groupID); ok {
		return group.MuteSeconds().Value
	}
	return s.cfg.MuteSeconds
}

func (s *Service) adminLogChatID(groupID int64) int64 {
	if group, ok := s.settings.Settings(groupID); ok {
		return group.AdminLogChatID().Value
	}
	return s.cfg.AdminLogChatID
}

func (s *Service) notify(ctx context.Context, chatID int64, text string) {
	s.telegram.Notify(ctx, chatID, text, s.cfg.NotifyTTLSeconds)
}

func (s *Service) groupLanguage(groupID int64) i18n.Lang {
	if s.settings != nil {
		if group, ok := s.settings.Settings(groupID); ok {
			return i18n.FromStored(group.Lang().Value)
		}
	}
	return i18n.FromStored(s.cfg.LangForGroup(groupID))
}

func (s *Service) warnPrecheck(ctx context.Context, msg *telego.Message, command string, checkTargetAdmin bool, l i18n.Lang) *telego.User {
	groupID := msg.Chat.ID
	reply := moderationReplyTarget(msg)
	if reply == nil || reply.From == nil {
		s.notify(ctx, groupID, i18n.Messages.Moderate.Common.ReplyUsage.Render(l, command))
		return nil
	}
	target := reply.From
	if notice := s.targetFailure(ctx, groupID, target.ID, checkTargetAdmin, l); notice != "" {
		s.notify(ctx, groupID, notice)
		return nil
	}
	return target
}

func (s *Service) warnKick(ctx context.Context, groupID, userID int64) (rejoinable bool, err error) {
	if err = s.telegram.Ban(ctx, groupID, userID, 0, false); err != nil {
		return false, err
	}
	if unbanErr := s.telegram.Unban(ctx, groupID, userID, true); unbanErr != nil {
		log.Printf("/warn unban %d in %d: %v", userID, groupID, unbanErr)
		return false, nil
	}
	return true, nil
}

// OnWarn increments the target's group-specific warning counter and kicks at the limit.
func (s *Service) OnWarn(ctx *th.Context, update telego.Update) error {
	return s.runModeration(ctx, update, "/warn")
}

// OnClearWarn clears the replied user's warning counter in the current group.
func (s *Service) OnClearWarn(ctx *th.Context, update telego.Update) error {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.From.ID <= 0 || msg.From.IsBot || !s.settings.IsGroup(msg.Chat.ID) {
		return nil
	}
	requestCtx := ctx.Context()
	groupID := msg.Chat.ID
	defer s.telegram.Delete(requestCtx, groupID, msg.MessageID)
	l := s.groupLanguage(groupID)
	if !s.requireRights(requestCtx, groupID, msg.From.ID, "/clearwarn", l,
		verification.GroupRights{CanRestrictMembers: true}) {
		return nil
	}
	target := s.warnPrecheck(requestCtx, msg, "/clearwarn", false, l)
	if target == nil {
		return nil
	}
	previous := s.warnings.clear(groupID, target.ID)
	if err := s.warnings.save(); err != nil {
		log.Printf("moderate: warning state save failed for group %d: %v", groupID, err)
	}
	s.notify(requestCtx, groupID, i18n.Messages.Moderate.Warning.Cleared.Render(l, tgfmt.DisplayName(target), previous, tgfmt.DisplayName(msg.From)))
	log.Printf("/clearwarn user=%d group=%d was=%d by=%d", target.ID, groupID, previous, msg.From.ID)
	return nil
}

// OnPurge handles /sb by banning the replied user and purging their messages.
func (s *Service) OnPurge(ctx *th.Context, update telego.Update) error {
	return s.runModeration(ctx, update, "/sb")
}

// OnBan bans the target and deletes replied-to evidence after success.
func (s *Service) OnBan(ctx *th.Context, update telego.Update) error {
	return s.runModeration(ctx, update, "/ban")
}

// OnMute handles a finite /mute duration, with an optional inline override.
func (s *Service) OnMute(ctx *th.Context, update telego.Update) error {
	return s.runModeration(ctx, update, "/mute")
}

// OnUnmute handles /unmute and fails closed when caller authorization is unavailable.
func (s *Service) OnUnmute(ctx *th.Context, update telego.Update) error {
	return s.runModeration(ctx, update, "/unmute")
}

// OnBanTime handles the group-specific /bantime policy command.
func (s *Service) OnBanTime(ctx *th.Context, update telego.Update) error {
	return s.runSettingsAdminCommand(ctx, update, func(groupID int64, l i18n.Lang) (string, error) {
		arg := strings.ToLower(strings.TrimSpace(commandArg(update.Message.Text)))
		usage := i18n.Messages.Moderate.BanTime.Usage.For(l)
		if arg == "" {
			seconds := s.banDuration(groupID)
			kind := i18n.Messages.Moderate.BanTime.PermanentDescription.For(l)
			if seconds > 0 {
				kind = i18n.Messages.Moderate.BanTime.TemporaryDescription.For(l)
			}
			return i18n.Messages.Moderate.BanTime.Current.Render(l, tgfmt.ModerationBanDurationStatus(l, seconds), kind, usage), nil
		}
		seconds, ok := parseBanDuration(arg)
		if !ok {
			return usage, nil
		}
		if err := s.setBanDuration(groupID, seconds); err != nil {
			return "", err
		}
		kind := i18n.Messages.Moderate.BanTime.PermanentDescription.For(l)
		if seconds > 0 {
			kind = i18n.Messages.Moderate.BanTime.TemporaryDescription.For(l)
		}
		return i18n.Messages.Moderate.BanTime.Set.Render(l, tgfmt.ModerationBanDurationStatus(l, seconds), kind), nil
	})
}

func (s *Service) runSettingsAdminCommand(ctx *th.Context, update telego.Update, run func(groupID int64, l i18n.Lang) (string, error)) error {
	msg := update.Message
	if msg == nil || msg.From == nil || msg.From.ID <= 0 || msg.From.IsBot || !s.settings.IsGroup(msg.Chat.ID) {
		return nil
	}
	requestCtx := ctx.Context()
	groupID := msg.Chat.ID
	l := s.groupLanguage(groupID)
	defer s.telegram.Delete(requestCtx, groupID, msg.MessageID)
	if !s.requireRights(requestCtx, groupID, msg.From.ID, "/bantime", l,
		verification.GroupRights{CanRestrictMembers: true}) {
		return nil
	}
	text, err := run(groupID, l)
	if err != nil {
		s.notifySettingsFailure(requestCtx, groupID, l, err)
		return nil
	}
	s.notify(requestCtx, groupID, text)
	return nil
}

func (s *Service) notifySettingsFailure(ctx context.Context, groupID int64, l i18n.Lang, err error) {
	log.Printf("moderation settings command in group %d failed: %v", groupID, err)
	var exceeded *settings.OwnerLimitsExceededError
	if errors.As(err, &exceeded) && len(exceeded.Violations) > 0 {
		violation := exceeded.Violations[0]
		s.notify(ctx, groupID, i18n.Messages.Panel.Settings.Error.LimitExceeded.Render(
			l, violation.Field, violation.Value, violation.Limit,
		))
		return
	}
	s.notify(ctx, groupID, i18n.Messages.Moderate.Common.SettingsSaveFailed.For(l))
}

func (s *Service) banDuration(groupID int64) int {
	group, _ := s.settings.Settings(groupID)
	return group.BanSeconds().Value
}

func (s *Service) setBanDuration(groupID int64, seconds int) error {
	group, ok := s.settings.Settings(groupID)
	if !ok {
		return fmt.Errorf("%w: %d", settings.ErrUnknownGroup, groupID)
	}
	overrides := group.Overrides()
	overrides.BanSeconds = &seconds
	_, err := s.settings.Update(groupID, group.Revision(), overrides, 0)
	return err
}

// parseBanDuration accepts permanent, seconds, or s/m/h/d suffixes.
func parseBanDuration(arg string) (seconds int, ok bool) {
	arg = strings.ToLower(strings.TrimSpace(arg))
	switch arg {
	case "":
		return 0, false
	case "0", "perm", "permanent", i18n.Messages.Moderate.Duration.PermanentInput.For(i18n.LangZH):
		return 0, true
	}
	multiplier := 1
	switch arg[len(arg)-1] {
	case 's':
		arg = arg[:len(arg)-1]
	case 'm':
		multiplier, arg = 60, arg[:len(arg)-1]
	case 'h':
		multiplier, arg = 3600, arg[:len(arg)-1]
	case 'd':
		multiplier, arg = 86400, arg[:len(arg)-1]
	}
	value, err := strconv.Atoi(arg)
	if err != nil || value < 0 || value > 1<<31 {
		return 0, false
	}
	return settings.ClampBanSeconds(value * multiplier), true
}

func commandArg(text string) string {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}
	return strings.Join(fields[1:], " ")
}

func warningsPath(stateDirectory string) string {
	if stateDirectory == "" {
		return ""
	}
	return filepath.Join(stateDirectory, "warns.json")
}
