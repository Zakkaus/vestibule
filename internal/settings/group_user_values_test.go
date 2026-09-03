package settings

import (
	"reflect"
	"testing"
)

// Twenty-eight rules carry a group's own configuration entry into that group's baseline.
// Probing them one at a time found twenty-six that no test held: remove one and the entry an
// operator wrote under that group has no effect, with the group silently keeping whatever the
// top level or the factory said.
//
// The rules were anonymous closures until this change, so a test could only address them by
// position. Each now names the key it carries, which is what lets a row below say which
// setting it stands for.
func TestGroupUserValuesReachThatGroupsBaseline(t *testing.T) {
	yes, no := true, false
	i := func(v int) *int { return &v }
	i64 := func(v int64) *int64 { return &v }
	for _, tc := range []struct {
		key  string
		set  func(*GroupConfig)
		read func(GroupBaseline) any
		want any
	}{
		{"enabled", func(c *GroupConfig) { c.Enabled = &no },
			func(b GroupBaseline) any { return b.Enabled.Value }, false},
		{"delivery_mode", func(c *GroupConfig) { c.DeliveryMode = DeliveryDM },
			func(b GroupBaseline) any { return b.DeliveryMode.Value }, DeliveryDM},
		{"verify_mode", func(c *GroupConfig) { c.VerifyMode = ModeQuiz },
			func(b GroupBaseline) any { return b.VerifyMode.Value }, ModeQuiz},
		{"name_spoiler", func(c *GroupConfig) { c.NameSpoiler = &no },
			func(b GroupBaseline) any { return b.NameSpoiler.Value }, false},
		{"ban_seconds", func(c *GroupConfig) { c.BanSeconds = i(86400) },
			func(b GroupBaseline) any { return b.BanSeconds.Value }, 86400},
		{"lookup_ttl_seconds", func(c *GroupConfig) { c.LookupTTLSeconds = i(321) },
			func(b GroupBaseline) any { return b.LookupTTLSeconds.Value }, 321},
		{"lookup_auto_delete_enabled", func(c *GroupConfig) { c.LookupAutoDeleteEnabled = &yes },
			func(b GroupBaseline) any { return b.LookupAutoDeleteEnabled.Value }, true},
		{"timeout_seconds", func(c *GroupConfig) { c.TimeoutSeconds = i(600) },
			func(b GroupBaseline) any { return b.TimeoutSeconds.Value }, 600},
		{"verify_max_fails", func(c *GroupConfig) { c.VerifyMaxFails = i(5) },
			func(b GroupBaseline) any { return b.VerifyMaxFails.Value }, 5},
		{"verify_retry_seconds", func(c *GroupConfig) { c.VerifyRetrySeconds = i(900) },
			func(b GroupBaseline) any { return b.VerifyRetrySeconds.Value }, 900},
		{"mute_seconds", func(c *GroupConfig) { c.MuteSeconds = i(7200) },
			func(b GroupBaseline) any { return b.MuteSeconds.Value }, 7200},
		{"verify_invited", func(c *GroupConfig) { c.VerifyInvited = &no },
			func(b GroupBaseline) any { return b.VerifyInvited.Value }, false},
		{"warn_limit", func(c *GroupConfig) { c.WarnLimit = i(9) },
			func(b GroupBaseline) any { return b.WarnLimit.Value }, 9},
		{"antispam_enabled", func(c *GroupConfig) { c.AntispamEnabled = &no },
			func(b GroupBaseline) any { return b.AntispamEnabled.Value }, false},
		{"channel_whitelist", func(c *GroupConfig) { c.ChannelWhitelist = &[]int64{-1009000001401} },
			func(b GroupBaseline) any { return b.ChannelWhitelist.Value }, []int64{-1009000001401}},
		{"trusted_member_group_ids", func(c *GroupConfig) { c.TrustedMemberGroupIDs = []int64{-1009000001402} },
			func(b GroupBaseline) any { return b.TrustedMemberGroupIDs.Value }, []int64{-1009000001402}},
		{"known_chat_ids", func(c *GroupConfig) { c.KnownChatIDs = &[]int64{-1009000001403} },
			func(b GroupBaseline) any { return b.KnownChatIDs.Value }, []int64{-1009000001403}},
		{"required_channel_id", func(c *GroupConfig) { c.RequiredChannelID = i64(-1009000001404) },
			func(b GroupBaseline) any { return b.RequiredChannelID.Value }, int64(-1009000001404)},
		{"channel_display", func(c *GroupConfig) { c.ChannelDisplay = "@thisgroup" },
			func(b GroupBaseline) any { return b.ChannelDisplay.Value }, "@thisgroup"},
		{"channel_invite_url", func(c *GroupConfig) { c.ChannelInviteURL = "https://t.me/+thisgroup" },
			func(b GroupBaseline) any { return b.ChannelInviteURL.Value }, "https://t.me/+thisgroup"},
		{"fallback_builtin", func(c *GroupConfig) { c.FallbackBuiltin = &yes },
			func(b GroupBaseline) any { return b.FallbackBuiltin.Value }, true},
		{"lang", func(c *GroupConfig) { c.Lang = "en" },
			func(b GroupBaseline) any { return b.Lang.Value }, "en"},
		{"rich_messages", func(c *GroupConfig) { c.RichMessages = &no },
			func(b GroupBaseline) any { return b.RichMessages.Value }, false},
		{"private_query_per_min", func(c *GroupConfig) { c.PrivateQueryPerMin = i(11) },
			func(b GroupBaseline) any { return b.PrivateQueryPerMin.Value }, 11},
		{"admin_log_chat_id", func(c *GroupConfig) { c.AdminLogChatID = i64(-1009000001405) },
			func(b GroupBaseline) any { return b.AdminLogChatID.Value }, int64(-1009000001405)},
		{"required_channel_fail_open", func(c *GroupConfig) { c.RequiredChannelFailOpen = &yes },
			func(b GroupBaseline) any { return b.RequiredChannelFailOpen.Value }, true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			cfg := &GroupConfig{}
			tc.set(cfg)
			got := applyGroupUserValues(GroupBaseline{}, cfg)
			if value := tc.read(got); !reflect.DeepEqual(value, tc.want) {
				t.Fatalf("%s written under this group did not reach it: got %v, want %v; the "+
					"group keeps whatever the top level or the factory said, silently",
					tc.key, value, tc.want)
			}
		})
	}
}

// Every rule names a key, and no two name the same one: the key is how a reader and a test
// address a rule, so a blank or duplicated one takes a rule back out of reach.
func TestGroupUserValueRuleKeysAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i, rule := range groupUserValueRules {
		if rule.key == "" {
			t.Errorf("rule %d carries no key; it can only be addressed by position", i)
			continue
		}
		if seen[rule.key] {
			t.Errorf("rule %d repeats the key %q", i, rule.key)
		}
		seen[rule.key] = true
	}
	if len(seen) != len(groupUserValueRules) {
		t.Errorf("%d distinct keys across %d rules", len(seen), len(groupUserValueRules))
	}
}
