package settings

type topLevelUserValueRule struct {
	key   string
	apply func(*GroupBaseline, *Config, bool)
}

var topLevelUserValueRules = [...]topLevelUserValueRule{
	{
		key: "enabled",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.Enabled != nil {
				group.Enabled = inputValue(*cfg.Enabled, present)
			}
		},
	},
	{
		key: "delivery_mode",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if ValidDeliveryMode(cfg.DeliveryMode) {
				group.DeliveryMode = inputValue(cfg.DeliveryMode, present)
			}
		},
	},
	{
		key: "verify_mode",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if ValidMode(cfg.VerifyMode) {
				group.VerifyMode = inputValue(cfg.VerifyMode, present)
			}
		},
	},
	{
		key: "name_spoiler",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.NameSpoiler != nil {
				group.NameSpoiler = inputValue(*cfg.NameSpoiler, present)
			}
		},
	},
	{
		key: "ban_seconds",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.BanSeconds != 0 {
				group.BanSeconds = inputValue(ClampBanSeconds(cfg.BanSeconds), present)
			}
		},
	},
	{
		key: "lookup_ttl_seconds",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.LookupTTLSeconds != nil {
				group.LookupTTLSeconds = inputValue(max(*cfg.LookupTTLSeconds, 0), present)
				if cfg.LookupAutoDeleteEnabled == nil {
					group.LookupAutoDeleteEnabled = inputValue(*cfg.LookupTTLSeconds > 0, present)
				}
			}
		},
	},
	{
		key: "lookup_auto_delete_enabled",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.LookupAutoDeleteEnabled != nil {
				group.LookupAutoDeleteEnabled = inputValue(*cfg.LookupAutoDeleteEnabled, present)
			}
		},
	},
	{
		key: "timeout_seconds",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.TimeoutSeconds != 0 {
				group.TimeoutSeconds = inputValue(min(max(cfg.TimeoutSeconds, 30), 1800), present)
			}
		},
	},
	{
		key: "verify_max_fails",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.VerifyMaxFails != 0 {
				group.VerifyMaxFails = inputValue(cfg.VerifyMaxFails, present)
			}
		},
	},
	{
		key: "verify_retry_seconds",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.VerifyRetrySeconds != 0 {
				group.VerifyRetrySeconds = inputValue(cfg.VerifyRetrySeconds, present)
			}
		},
	},
	{
		key: "mute_seconds",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.MuteSeconds != 0 {
				group.MuteSeconds = inputValue(cfg.MuteSeconds, present)
			}
		},
	},
	{
		key: "verify_invited",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.VerifyInvited != nil {
				group.VerifyInvited = inputValue(*cfg.VerifyInvited, present)
			}
		},
	},
	{
		key: "warn_limit",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.WarnLimit != 0 {
				group.WarnLimit = inputValue(cfg.WarnLimit, present)
			}
		},
	},
	{
		key: "antispam_enabled",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.AntispamEnabled != nil {
				group.AntispamEnabled = inputValue(*cfg.AntispamEnabled, present)
			}
		},
	},
	{
		key: "block_channel_senders",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.AntispamEnabled == nil && cfg.BlockChannelSenders != nil {
				group.AntispamEnabled = inputValue(*cfg.BlockChannelSenders, present)
			}
		},
	},
	{
		key: "channel_whitelist",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.ChannelWhitelist != nil {
				group.ChannelWhitelist = inputValue(append([]int64(nil), cfg.ChannelWhitelist...), present)
			}
		},
	},
	{
		key: "trusted_member_group_ids",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.TrustedMemberGroupIDs != nil {
				group.TrustedMemberGroupIDs = inputValue(append([]int64(nil), cfg.TrustedMemberGroupIDs...), present)
			}
		},
	},
	{
		key: "known_chat_ids",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.KnownChatIDs != nil {
				group.KnownChatIDs = inputValue(append([]int64(nil), cfg.KnownChatIDs...), present)
			}
		},
	},
	{
		key: "required_channel_id",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.RequiredChannelID != 0 {
				group.RequiredChannelID = inputValue(cfg.RequiredChannelID, present)
			}
		},
	},
	{
		key: "channel_display",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.ChannelDisplay != "" {
				group.ChannelDisplay = inputValue(cfg.ChannelDisplay, present)
			}
		},
	},
	{
		key: "channel_invite_url",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.ChannelInviteURL != "" {
				group.ChannelInviteURL = inputValue(cfg.ChannelInviteURL, present)
			}
		},
	},
	{
		key: "questions",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.Questions != nil {
				group.Questions = inputValue(cloneQuestions(cfg.Questions), present)
			}
		},
	},
	{
		key: "fallback_questions",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.FallbackQuestions != nil {
				group.FallbackQuestions = inputValue(cloneShortQuestions(cfg.FallbackQuestions), present)
				if cfg.FallbackBuiltin == nil {
					group.FallbackBuiltin = inputValue(len(cfg.FallbackQuestions) == 0, present)
				}
			}
		},
	},
	{
		key: "fallback_builtin",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.FallbackBuiltin != nil {
				group.FallbackBuiltin = inputValue(*cfg.FallbackBuiltin, present)
			}
		},
	},
	{
		key: "lang",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.Lang != "" {
				group.Lang = inputValue(cfg.Lang, present)
			}
		},
	},
	{
		key: "rich_messages",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.RichMessages {
				group.RichMessages = inputValue(cfg.RichMessages, present)
			}
		},
	},
	{
		key: "private_query_per_min",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.PrivateQueryPerMin != 0 {
				group.PrivateQueryPerMin = inputValue(cfg.PrivateQueryPerMin, present)
			}
		},
	},
	{
		key: "admin_log_chat_id",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if present || cfg.AdminLogChatID != 0 {
				group.AdminLogChatID = inputValue(cfg.AdminLogChatID, present)
			}
		},
	},
	{
		key: "required_channel_fail_open",
		apply: func(group *GroupBaseline, cfg *Config, present bool) {
			if cfg.RequiredChannelFailOpen != nil {
				group.RequiredChannelFailOpen = inputValue(*cfg.RequiredChannelFailOpen, present)
			}
		},
	},
}

type groupUserValueRule func(*GroupBaseline, *GroupConfig)

var groupUserValueRules = [...]groupUserValueRule{
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.Enabled != nil {
			group.Enabled = userFileValue(*cfg.Enabled)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if ValidDeliveryMode(cfg.DeliveryMode) {
			group.DeliveryMode = userFileValue(cfg.DeliveryMode)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if ValidMode(cfg.VerifyMode) {
			group.VerifyMode = userFileValue(cfg.VerifyMode)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.NameSpoiler != nil {
			group.NameSpoiler = userFileValue(*cfg.NameSpoiler)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.BanSeconds != nil {
			group.BanSeconds = userFileValue(ClampBanSeconds(*cfg.BanSeconds))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.LookupTTLSeconds != nil {
			group.LookupTTLSeconds = userFileValue(*cfg.LookupTTLSeconds)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.LookupAutoDeleteEnabled != nil {
			group.LookupAutoDeleteEnabled = userFileValue(*cfg.LookupAutoDeleteEnabled)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.TimeoutSeconds != nil {
			group.TimeoutSeconds = userFileValue(*cfg.TimeoutSeconds)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.VerifyMaxFails != nil {
			group.VerifyMaxFails = userFileValue(*cfg.VerifyMaxFails)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.VerifyRetrySeconds != nil {
			group.VerifyRetrySeconds = userFileValue(*cfg.VerifyRetrySeconds)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.MuteSeconds != nil {
			group.MuteSeconds = userFileValue(*cfg.MuteSeconds)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.VerifyInvited != nil {
			group.VerifyInvited = userFileValue(*cfg.VerifyInvited)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.WarnLimit != nil {
			group.WarnLimit = userFileValue(*cfg.WarnLimit)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.AntispamEnabled != nil {
			group.AntispamEnabled = userFileValue(*cfg.AntispamEnabled)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.ChannelWhitelist != nil {
			group.ChannelWhitelist = userFileValue(append([]int64(nil), (*cfg.ChannelWhitelist)...))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.TrustedMemberGroupIDs != nil {
			group.TrustedMemberGroupIDs = userFileValue(append([]int64(nil), cfg.TrustedMemberGroupIDs...))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.KnownChatIDs != nil {
			group.KnownChatIDs = userFileValue(append([]int64(nil), (*cfg.KnownChatIDs)...))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.RequiredChannelID != nil {
			group.RequiredChannelID = userFileValue(*cfg.RequiredChannelID)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.ChannelDisplay != "" {
			group.ChannelDisplay = userFileValue(cfg.ChannelDisplay)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.ChannelInviteURL != "" {
			group.ChannelInviteURL = userFileValue(cfg.ChannelInviteURL)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.Questions != nil {
			group.Questions = userFileValue(cloneQuestions(cfg.Questions))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.FallbackQuestions != nil {
			group.FallbackQuestions = userFileValue(cloneShortQuestions(*cfg.FallbackQuestions))
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.FallbackBuiltin != nil {
			group.FallbackBuiltin = userFileValue(*cfg.FallbackBuiltin)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.Lang != "" {
			group.Lang = userFileValue(cfg.Lang)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.RichMessages != nil {
			group.RichMessages = userFileValue(*cfg.RichMessages)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.PrivateQueryPerMin != nil {
			group.PrivateQueryPerMin = userFileValue(*cfg.PrivateQueryPerMin)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.AdminLogChatID != nil {
			group.AdminLogChatID = userFileValue(*cfg.AdminLogChatID)
		}
	},
	func(group *GroupBaseline, cfg *GroupConfig) {
		if cfg.RequiredChannelFailOpen != nil {
			group.RequiredChannelFailOpen = userFileValue(*cfg.RequiredChannelFailOpen)
		}
	},
}

func applyTopLevelUserValues(group GroupBaseline, cfg *Config, presence configPresence) GroupBaseline {
	for _, rule := range topLevelUserValueRules {
		rule.apply(&group, cfg, presence.has(rule.key))
	}
	return group
}

func applyGroupUserValues(group GroupBaseline, cfg *GroupConfig) GroupBaseline {
	for _, rule := range groupUserValueRules {
		rule(&group, cfg)
	}
	return group
}
