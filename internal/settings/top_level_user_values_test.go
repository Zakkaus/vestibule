package settings

import (
	"reflect"
	"testing"
)

// Twenty-nine rules carry a value written at the top of the configuration file into every
// group's baseline. Removing one leaves the operator's setting with no effect at all: the file
// says the timeout is 600 seconds, the group keeps the factory 240, and nothing anywhere
// reports a difference. Probing the rules one at a time found twenty-four that no test held.
//
// One case each, in one table, because the rules are one mechanism: each row fails when its
// own rule is removed, and the row names the key an operator would have written.
func TestTopLevelUserValuesReachTheGroupBaseline(t *testing.T) {
	trueValue, falseValue := true, false
	for _, tc := range []struct {
		key  string
		set  func(*Config)
		read func(GroupBaseline) any
		want any
	}{
		{"enabled", func(c *Config) { c.Enabled = &falseValue },
			func(b GroupBaseline) any { return b.Enabled.Value }, false},
		{"delivery_mode", func(c *Config) { c.DeliveryMode = DeliveryDM },
			func(b GroupBaseline) any { return b.DeliveryMode.Value }, DeliveryDM},
		{"verify_mode", func(c *Config) { c.VerifyMode = ModeQuiz },
			func(b GroupBaseline) any { return b.VerifyMode.Value }, ModeQuiz},
		{"name_spoiler", func(c *Config) { c.NameSpoiler = &falseValue },
			func(b GroupBaseline) any { return b.NameSpoiler.Value }, false},
		{"ban_seconds", func(c *Config) { c.BanSeconds = 86400 },
			func(b GroupBaseline) any { return b.BanSeconds.Value }, 86400},
		{"lookup_ttl_seconds", func(c *Config) { v := 321; c.LookupTTLSeconds = &v },
			func(b GroupBaseline) any { return b.LookupTTLSeconds.Value }, 321},
		{"lookup_auto_delete_enabled", func(c *Config) { c.LookupAutoDeleteEnabled = &trueValue },
			func(b GroupBaseline) any { return b.LookupAutoDeleteEnabled.Value }, true},
		{"timeout_seconds", func(c *Config) { c.TimeoutSeconds = 600 },
			func(b GroupBaseline) any { return b.TimeoutSeconds.Value }, 600},
		{"verify_max_fails", func(c *Config) { c.VerifyMaxFails = 5 },
			func(b GroupBaseline) any { return b.VerifyMaxFails.Value }, 5},
		{"verify_retry_seconds", func(c *Config) { c.VerifyRetrySeconds = 900 },
			func(b GroupBaseline) any { return b.VerifyRetrySeconds.Value }, 900},
		{"mute_seconds", func(c *Config) { c.MuteSeconds = 7200 },
			func(b GroupBaseline) any { return b.MuteSeconds.Value }, 7200},
		{"verify_invited", func(c *Config) { c.VerifyInvited = &falseValue },
			func(b GroupBaseline) any { return b.VerifyInvited.Value }, false},
		{"warn_limit", func(c *Config) { c.WarnLimit = 9 },
			func(b GroupBaseline) any { return b.WarnLimit.Value }, 9},
		{"antispam_enabled", func(c *Config) { c.AntispamEnabled = &falseValue },
			func(b GroupBaseline) any { return b.AntispamEnabled.Value }, false},
		{"channel_whitelist", func(c *Config) { c.ChannelWhitelist = []int64{-1009000001301} },
			func(b GroupBaseline) any { return b.ChannelWhitelist.Value }, []int64{-1009000001301}},
		{"trusted_member_group_ids", func(c *Config) { c.TrustedMemberGroupIDs = []int64{-1009000001302} },
			func(b GroupBaseline) any { return b.TrustedMemberGroupIDs.Value }, []int64{-1009000001302}},
		{"known_chat_ids", func(c *Config) { c.KnownChatIDs = []int64{-1009000001303} },
			func(b GroupBaseline) any { return b.KnownChatIDs.Value }, []int64{-1009000001303}},
		{"required_channel_id", func(c *Config) { c.RequiredChannelID = -1009000001304 },
			func(b GroupBaseline) any { return b.RequiredChannelID.Value }, int64(-1009000001304)},
		{"channel_display", func(c *Config) { c.ChannelDisplay = "@somewhere" },
			func(b GroupBaseline) any { return b.ChannelDisplay.Value }, "@somewhere"},
		{"channel_invite_url", func(c *Config) { c.ChannelInviteURL = "https://t.me/+chosen" },
			func(b GroupBaseline) any { return b.ChannelInviteURL.Value }, "https://t.me/+chosen"},
		{"fallback_builtin", func(c *Config) { c.FallbackBuiltin = &trueValue },
			func(b GroupBaseline) any { return b.FallbackBuiltin.Value }, true},
		{"lang", func(c *Config) { c.Lang = "en" },
			func(b GroupBaseline) any { return b.Lang.Value }, "en"},
		{"rich_messages", func(c *Config) { c.RichMessages = true },
			func(b GroupBaseline) any { return b.RichMessages.Value }, true},
		{"private_query_per_min", func(c *Config) { c.PrivateQueryPerMin = 11 },
			func(b GroupBaseline) any { return b.PrivateQueryPerMin.Value }, 11},
		{"admin_log_chat_id", func(c *Config) { c.AdminLogChatID = -1009000001305 },
			func(b GroupBaseline) any { return b.AdminLogChatID.Value }, int64(-1009000001305)},
		{"required_channel_fail_open", func(c *Config) { c.RequiredChannelFailOpen = &trueValue },
			func(b GroupBaseline) any { return b.RequiredChannelFailOpen.Value }, true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			cfg := &Config{}
			tc.set(cfg)
			got := applyTopLevelUserValues(GroupBaseline{}, cfg, configPresence{tc.key: true})
			if value := tc.read(got); !reflect.DeepEqual(value, tc.want) {
				t.Fatalf("%s in the configuration file did not reach the group: got %v, want %v; "+
					"the operator's setting has no effect and nothing reports it",
					tc.key, value, tc.want)
			}
		})
	}
}

// Presence decides provenance: a key the file actually carries is recorded as coming from the
// file, so the console can show an operator which of their settings the file is holding.
func TestTopLevelUserValueSourceFollowsPresence(t *testing.T) {
	cfg := &Config{TimeoutSeconds: 600}
	present := applyTopLevelUserValues(GroupBaseline{}, cfg, configPresence{"timeout_seconds": true})
	if present.TimeoutSeconds.Source != SourceUserFile {
		t.Errorf("a key the file carries has source %v, want %v",
			present.TimeoutSeconds.Source, SourceUserFile)
	}
	absent := applyTopLevelUserValues(GroupBaseline{}, cfg, configPresence{})
	if absent.TimeoutSeconds.Source != SourceFactory {
		t.Errorf("a key the file does not carry has source %v, want %v",
			absent.TimeoutSeconds.Source, SourceFactory)
	}
}
