// Package settings loads process configuration and resolves per-chat settings.
package settings

import (
	"fmt"
	"time"
)

const (
	// ModeQuiz presents shuffled inline-button questions.
	ModeQuiz = "quiz"
	// ModeKernel requires applicants to type a kernel version.
	ModeKernel = "kernel"
	// ModeMixed chooses quiz or kernel verification per applicant.
	ModeMixed = "mixed"
)

const (
	// DeliveryGroup posts the challenge only in the guarded group.
	DeliveryGroup = "group"
	// DeliveryDM attempts a private challenge and posts in the group only after a definite rejection.
	DeliveryDM = "dm"
	// DeliveryBoth posts in the group before attempting the private challenge.
	DeliveryBoth = "both"
)

const (
	// ModuleGentoo contains Gentoo lookups and Bugzilla/news subscriptions.
	ModuleGentoo = "gentoo"
	// ModuleLinux contains cross-distribution and general Linux lookups.
	ModuleLinux = "linux"
)

var optionalModules = [...]string{ModuleGentoo, ModuleLinux}

// OptionalModuleNames returns every process-level module that may be disabled.
func OptionalModuleNames() []string {
	return append([]string(nil), optionalModules[:]...)
}

// ValidOptionalModule reports whether name identifies a supported optional module.
func ValidOptionalModule(name string) bool {
	for _, module := range optionalModules {
		if name == module {
			return true
		}
	}
	return false
}

const defaultDeliveryMode = DeliveryBoth

// ValidDeliveryMode reports whether mode names a supported challenge delivery mode.
func ValidDeliveryMode(mode string) bool {
	switch mode {
	case DeliveryGroup, DeliveryDM, DeliveryBoth:
		return true
	}
	return false
}

const defaultVerifyMode = ModeKernel

// ValidMode reports whether mode names a supported verification mode.
func ValidMode(mode string) bool {
	switch mode {
	case ModeQuiz, ModeKernel, ModeMixed:
		return true
	}
	return false
}

// ValidLanguage reports whether lang is empty or a supported canonical language tag.
func ValidLanguage(lang string) bool {
	switch lang {
	case "", "zh", "zh-Hant", "en":
		return true
	default:
		return false
	}
}

// Telegram treats until_date below 30 seconds or above 366 days as permanent.
const telegramBanMax = 366 * 86400

const (
	maxDurationSeconds           = int64((1<<63 - 1) / int64(time.Second))
	maxFeedIntervalSeconds       = 24 * 60 * 60
	maxOwnerClaimLifetimeSeconds = 24 * 60 * 60
	maxMessageTTLSeconds         = 24 * 60 * 60
	maxVerifyRetrySeconds        = telegramBanMax
)

// SecondsToDuration converts seconds without overflowing time.Duration.
func SecondsToDuration(seconds int) (time.Duration, bool) {
	value := int64(seconds)
	if value < -maxDurationSeconds || value > maxDurationSeconds {
		return 0, false
	}
	return time.Duration(value) * time.Second, true
}

func checkedConfigDuration(key string, seconds int, minimum, operationalMaximum int64) (time.Duration, error) {
	maximum := min(operationalMaximum, maxDurationSeconds)
	value := int64(seconds)
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s=%d is outside the accepted range %d..%d seconds", key, seconds, minimum, maximum)
	}
	duration, ok := SecondsToDuration(seconds)
	if !ok {
		return 0, fmt.Errorf("%s=%d is outside the accepted range %d..%d seconds", key, seconds, minimum, maximum)
	}
	return duration, nil
}

func validatePositiveConfigDuration(key string, seconds int, operationalMaximum int64) error {
	if seconds <= 0 {
		return nil
	}
	_, err := checkedConfigDuration(key, seconds, 1, operationalMaximum)
	return err
}

// ClampBanSeconds maps a ban duration into Telegram's enforced range.
func ClampBanSeconds(seconds int) int {
	switch {
	case seconds <= 0:
		return 0
	case seconds < 30:
		return 30
	case seconds > telegramBanMax:
		return 0
	default:
		return seconds
	}
}

// clampMuteSecs maps a finite mute duration into Telegram's enforced range.
func clampMuteSecs(secs int) int {
	switch {
	case secs < 30:
		return 30
	case secs > telegramBanMax:
		return telegramBanMax
	default:
		return secs
	}
}

// Question is one verification quiz item with a zero-based answer index.
type Question struct {
	// Q is the question prompt.
	Q string `json:"q"`
	// Options lists the possible answers.
	Options []string `json:"options"`
	// Answer is the zero-based index of the correct option.
	Answer int `json:"answer"`
}

// ShortQuestion is an answer-hidden verification question.
type ShortQuestion struct {
	// Q is the question prompt.
	Q string `json:"q"`
	// Answers lists normalized whole replies accepted as correct.
	Answers []string `json:"answers"`
}

// OverlayCfg identifies a GitHub overlay searched by /pkg.
type OverlayCfg struct {
	// Name is the overlay's display and cache name.
	Name string `json:"name"`
	// Repo is the GitHub repository in owner/name form.
	Repo string `json:"repo"`
	// Branch is the repository branch and defaults to master when empty.
	Branch string `json:"branch"`
}

// GroupConfig contains file-managed values for one guarded chat.
type GroupConfig struct {
	ID                      int64            `json:"id"`
	Enabled                 *bool            `json:"enabled"`
	DeliveryMode            string           `json:"delivery_mode"`
	VerifyMode              string           `json:"verify_mode"`
	NameSpoiler             *bool            `json:"name_spoiler"`
	BanSeconds              *int             `json:"ban_seconds"`
	LookupTTLSeconds        *int             `json:"lookup_ttl_seconds"`
	LookupAutoDeleteEnabled *bool            `json:"lookup_auto_delete_enabled"`
	TimeoutSeconds          *int             `json:"timeout_seconds"`
	VerifyMaxFails          *int             `json:"verify_max_fails"`
	VerifyRetrySeconds      *int             `json:"verify_retry_seconds"`
	MuteSeconds             *int             `json:"mute_seconds"`
	VerifyInvited           *bool            `json:"verify_invited"`
	WarnLimit               *int             `json:"warn_limit"`
	AntispamEnabled         *bool            `json:"antispam_enabled"`
	ChannelWhitelist        *[]int64         `json:"channel_whitelist"`
	TrustedMemberGroupIDs   []int64          `json:"trusted_member_group_ids"`
	KnownChatIDs            *[]int64         `json:"known_chat_ids"`
	RequiredChannelID       *int64           `json:"required_channel_id"`
	ChannelDisplay          string           `json:"channel_display"`
	ChannelInviteURL        string           `json:"channel_invite_url"`
	Questions               []Question       `json:"questions"`
	FallbackQuestions       *[]ShortQuestion `json:"fallback_questions"`
	FallbackBuiltin         *bool            `json:"fallback_builtin"`
	Lang                    string           `json:"lang"`
	RichMessages            *bool            `json:"rich_messages"`
	PrivateQueryPerMin      *int             `json:"private_query_per_min"`
	AdminLogChatID          *int64           `json:"admin_log_chat_id"`
	RequiredChannelFailOpen *bool            `json:"required_channel_fail_open"`
}

// FeedConfig configures one optional Bugzilla and news destination.
type FeedConfig struct {
	// ChatID is the channel or group receiving feed posts.
	ChatID int64 `json:"chat_id"`
	// Lang selects zh, zh-Hant, or en and defaults to zh.
	Lang string `json:"lang"`
	// IntervalSeconds is the polling interval with a 300-second default and one-day maximum.
	IntervalSeconds int `json:"interval_seconds"`
	// Bugs enables Bugzilla posts and defaults to true.
	Bugs *bool `json:"bugs"`
	// News enables news posts and defaults to true.
	News *bool `json:"news"`
	// BugProduct filters bugs by product when non-empty.
	BugProduct string `json:"bug_product"`
	// BugComponent filters bugs by component when non-empty.
	BugComponent string `json:"bug_component"`
	// SilentBugs makes every bug post silent when true.
	SilentBugs *bool `json:"silent_bugs"`
}

// BugsOn reports whether this feed posts Bugzilla bugs.
func (f *FeedConfig) BugsOn() bool { return f.Bugs == nil || *f.Bugs }

// NewsOn reports whether this feed posts news items.
func (f *FeedConfig) NewsOn() bool { return f.News == nil || *f.News }

// Interval returns this feed's clamped polling interval.
func (f *FeedConfig) Interval() time.Duration {
	seconds := f.IntervalSeconds
	switch {
	case seconds <= 0:
		seconds = 5 * 60
	case seconds < 60:
		seconds = 60
	case seconds > maxFeedIntervalSeconds:
		seconds = maxFeedIntervalSeconds
	}
	duration, _ := SecondsToDuration(seconds)
	return duration
}

// Config contains the validated JSON configuration.
type Config struct {
	// DisabledModules turns off optional query and subscription modules for this bot instance.
	DisabledModules []string `json:"disabled_modules"`
	// Groups is the canonical guarded-group list after legacy IDs are merged.
	Groups []GroupConfig `json:"groups"`
	// GroupIDs mirrors Groups and accepts the legacy group_ids key.
	GroupIDs []int64 `json:"group_ids"`
	// GroupID accepts the legacy singular group_id key.
	GroupID int64 `json:"group_id"`
	// Enabled is a legacy top-level value expanded into every configured chat.
	Enabled *bool `json:"enabled"`
	// NameSpoiler is a legacy top-level value expanded into every configured chat.
	NameSpoiler *bool `json:"name_spoiler"`
	// Lang is the default language for group-facing output and defaults to zh.
	Lang string `json:"lang"`
	// RequiredChannelID gates approval on channel membership and zero disables it.
	RequiredChannelID int64 `json:"required_channel_id"`
	// ChannelDisplay names the required channel for messages and public links.
	ChannelDisplay string `json:"channel_display"`
	// TrustedMemberGroupIDs is the global verification-bypass source list.
	TrustedMemberGroupIDs []int64 `json:"trusted_member_group_ids"`
	// KnownChatIDs prevents auto-leave without granting verification or bypass semantics.
	KnownChatIDs []int64 `json:"known_chat_ids"`
	// OwnerClaimLifetimeSeconds limits the one-use journal claim to at most one day.
	OwnerClaimLifetimeSeconds int `json:"owner_claim_lifetime_seconds"`
	// OwnerClaimUserID optionally restricts the first owner claim to one Telegram user.
	OwnerClaimUserID int64 `json:"owner_claim_user_id"`
	// ChannelInviteURL links a private required channel without a public handle.
	ChannelInviteURL string `json:"channel_invite_url"`
	// TimeoutSeconds is the verification deadline.
	TimeoutSeconds int `json:"timeout_seconds"`
	// AdminLogChatID receives moderation and failed-action notices.
	AdminLogChatID int64 `json:"admin_log_chat_id"`
	// NotifyTTLSeconds controls notice deletion, defaults to 60, and has a one-day maximum.
	NotifyTTLSeconds int `json:"notify_ttl_seconds"`
	// LookupTTLSeconds controls lookup deletion, defaults to 180, and has a one-day maximum.
	LookupTTLSeconds *int `json:"lookup_ttl_seconds"`
	// LookupAutoDeleteEnabled overrides TTL-derived lookup cleanup for configured chats.
	LookupAutoDeleteEnabled *bool `json:"lookup_auto_delete_enabled"`
	// WarnLimit is the strike count before an automatic kick and defaults to three.
	WarnLimit int `json:"warn_limit"`
	// PrivateQueryPerMin is the per-user DM query limit and defaults to three.
	PrivateQueryPerMin int `json:"private_query_per_min"`
	// RequiredChannelFailOpen controls admission when required-channel membership is unreadable.
	RequiredChannelFailOpen *bool `json:"required_channel_fail_open"`
	// VerifyInvited controls whether a member somebody else added still has to verify.
	VerifyInvited *bool `json:"verify_invited"`
	// BanSeconds is the default ban duration and zero means permanent.
	BanSeconds int `json:"ban_seconds"`
	// MuteSeconds is the finite default mute duration.
	MuteSeconds int `json:"mute_seconds"`
	// VerifyRetrySeconds is the cooldown, capped at 366 days; a negative value disables it.
	VerifyRetrySeconds int `json:"verify_retry_seconds"`
	// VerifyMaxFails is the automatic-ban threshold and a negative value disables it.
	VerifyMaxFails int `json:"verify_max_fails"`
	// VerifyMode selects kernel, quiz, or mixed verification.
	VerifyMode string `json:"verify_mode"`
	// DeliveryMode selects group-only, private-with-fallback, or group-and-private challenge delivery.
	DeliveryMode string `json:"delivery_mode"`
	// FallbackQuestions is the answer-hidden path for applicants without Linux.
	FallbackQuestions []ShortQuestion `json:"fallback_questions"`
	// FallbackBuiltin selects the embedded factory rules when no chat bank is configured.
	FallbackBuiltin *bool `json:"fallback_builtin"`
	// Overlays lists GitHub overlays searched by /pkg.
	Overlays []OverlayCfg `json:"overlays"`
	// NewsURL is the Gentoo news-items index used by /news.
	NewsURL string `json:"news_url"`
	// StatsTimezone is the IANA time zone for the daily /stats boundary.
	StatsTimezone string `json:"stats_timezone"`
	// RichMessages enables rich Bot API messages with an HTML fallback.
	RichMessages bool `json:"rich_messages"`
	// UserAgent overrides the outbound HTTP User-Agent when non-empty.
	UserAgent string `json:"user_agent"`
	// PrivateReply handles non-command DMs outside verification.
	PrivateReply string `json:"private_reply"`
	// BlockChannelSenders rejects sender-chat posts. Unset means enabled: posting as a channel is
	// how this spam arrives, and the ban does nothing at all while Telegram's privacy mode keeps
	// those messages from the bot, so leaving it on costs a group that never sees them nothing.
	BlockChannelSenders *bool `json:"block_channel_senders"`
	// AntispamEnabled is the current spelling of block_channel_senders.
	AntispamEnabled *bool `json:"antispam_enabled"`
	// ChannelWhitelist lists sender chats allowed to post in guarded groups.
	ChannelWhitelist []int64 `json:"channel_whitelist"`
	// Feeds lists Bugzilla and news destinations.
	Feeds []FeedConfig `json:"feeds"`
	// Feed accepts the legacy singular feed form and is merged into Feeds.
	Feed *FeedConfig `json:"feed"`
	// Questions is the global verification quiz pool.
	Questions              []Question `json:"questions"`
	processSettingsSources processSettingsSources
}

// OwnerClaimLifetime returns the configured first-owner claim lifetime.
func (c *Config) OwnerClaimLifetime() time.Duration {
	seconds := c.OwnerClaimLifetimeSeconds
	if seconds <= 0 || seconds > maxOwnerClaimLifetimeSeconds {
		seconds = 10 * 60
	}
	duration, _ := SecondsToDuration(seconds)
	return duration
}

// ModuleEnabled reports whether the named optional module is enabled for this bot instance.
func (c *Config) ModuleEnabled(name string) bool {
	for _, disabled := range c.DisabledModules {
		if disabled == name {
			return false
		}
	}
	return true
}

// IsGroup reports whether id is one of the guarded groups.
func (c *Config) IsGroup(id int64) bool {
	for _, g := range c.GroupIDs {
		if g == id {
			return true
		}
	}
	return false
}

func (c *Config) group(id int64) *GroupConfig {
	for i := range c.Groups {
		if c.Groups[i].ID == id {
			return &c.Groups[i]
		}
	}
	return nil
}

// LangForGroup returns the group override, global language, or zh by default.
func (c *Config) LangForGroup(id int64) string {
	if g := c.group(id); g != nil && g.Lang != "" {
		return g.Lang
	}
	if c.Lang != "" {
		return c.Lang
	}
	return "zh"
}

// RequiredChannel returns the effective required-channel ID for a group.
func (c *Config) RequiredChannel(id int64) int64 {
	if g := c.group(id); g != nil && g.RequiredChannelID != nil {
		return *g.RequiredChannelID
	}
	return c.RequiredChannelID
}

// TrustedGroups returns the effective verification-bypass source list for a group.
func (c *Config) TrustedGroups(id int64) []int64 {
	if g := c.group(id); g != nil && g.TrustedMemberGroupIDs != nil {
		return g.TrustedMemberGroupIDs
	}
	return c.TrustedMemberGroupIDs
}

// VerifyInvitedMembers reports whether a member added by somebody else still has to verify.
// Being vouched for is not verification, so this defaults on.
func (c *Config) VerifyInvitedMembers() bool {
	return c.VerifyInvited == nil || *c.VerifyInvited
}

// BlockChannelSendersEnabled reports whether sender-chat posts are rejected, defaulting to on.
func (c *Config) BlockChannelSendersEnabled() bool {
	return c.BlockChannelSenders == nil || *c.BlockChannelSenders
}

// FailOpenChannel reports whether unreadable required-channel membership admits users.
func (c *Config) FailOpenChannel() bool {
	return c.RequiredChannelFailOpen == nil || *c.RequiredChannelFailOpen
}

// ChannelDisplayFor returns the effective required-channel display name for a group.
func (c *Config) ChannelDisplayFor(id int64) string {
	if g := c.group(id); g != nil && g.ChannelDisplay != "" {
		return g.ChannelDisplay
	}
	return c.ChannelDisplay
}

// ChannelInvite returns the effective private-channel invite URL for a group.
func (c *Config) ChannelInvite(id int64) string {
	if g := c.group(id); g != nil && g.ChannelInviteURL != "" {
		return g.ChannelInviteURL
	}
	return c.ChannelInviteURL
}

// VerifyModeFor returns the effective verification mode for a group.
func (c *Config) VerifyModeFor(id int64) string {
	if g := c.group(id); g != nil && ValidMode(g.VerifyMode) {
		return g.VerifyMode
	}
	if ValidMode(c.VerifyMode) {
		return c.VerifyMode
	}
	return defaultVerifyMode
}

// DeliveryModeFor returns the effective challenge delivery mode for a group.
func (c *Config) DeliveryModeFor(id int64) string {
	if g := c.group(id); g != nil && ValidDeliveryMode(g.DeliveryMode) {
		return g.DeliveryMode
	}
	if ValidDeliveryMode(c.DeliveryMode) {
		return c.DeliveryMode
	}
	return defaultDeliveryMode
}

// QuestionsFor returns the effective verification quiz pool for a group.
func (c *Config) QuestionsFor(id int64) []Question {
	if g := c.group(id); g != nil && len(g.Questions) > 0 {
		return g.Questions
	}
	return c.Questions
}

// IsKnownChat is the auto-leave allowlist, including support-only chats.
func (c *Config) IsKnownChat(id int64) bool {
	return c.isDirectKnownChat(id) ||
		containsChatID(c.KnownChatIDs, id) ||
		containsChatID(c.TrustedMemberGroupIDs, id) ||
		c.groupReferencesKnownChat(id) ||
		c.feedReferencesKnownChat(id)
}

func (c *Config) isDirectKnownChat(id int64) bool {
	return c.IsGroup(id) ||
		(c.RequiredChannelID != 0 && id == c.RequiredChannelID) ||
		(c.AdminLogChatID != 0 && id == c.AdminLogChatID)
}

func containsChatID(chatIDs []int64, id int64) bool {
	for _, chatID := range chatIDs {
		if chatID == id {
			return true
		}
	}
	return false
}

func (c *Config) groupReferencesKnownChat(id int64) bool {
	for i := range c.Groups {
		group := &c.Groups[i]
		if group.RequiredChannelID != nil && *group.RequiredChannelID == id {
			return true
		}
		if containsChatID(group.TrustedMemberGroupIDs, id) {
			return true
		}
	}
	return false
}

func (c *Config) feedReferencesKnownChat(id int64) bool {
	for i := range c.Feeds {
		if c.Feeds[i].ChatID == id {
			return true
		}
	}
	return false
}
