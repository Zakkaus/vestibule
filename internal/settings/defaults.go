package settings

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var defaultsYAML string

type defaultsDocument struct {
	Process struct {
		OwnerClaimLifetimeSeconds int    `yaml:"owner_claim_lifetime_seconds"`
		OwnerClaimUserID          int64  `yaml:"owner_claim_user_id"`
		NotifyTTLSeconds          int    `yaml:"notify_ttl_seconds"`
		PrivateQueryPerMin        int    `yaml:"private_query_per_min"`
		StatsTimezone             string `yaml:"stats_timezone"`
		UserAgent                 string `yaml:"user_agent"`
		PrivateReply              string `yaml:"private_reply"`
	} `yaml:"process"`
	Resources struct {
		NewsURL  string       `yaml:"news_url"`
		Overlays []OverlayCfg `yaml:"overlays"`
		Feeds    []FeedConfig `yaml:"feeds"`
	} `yaml:"resources"`
	Factory groupDefaults `yaml:"factory"`
}

type groupDefaults struct {
	Enabled                 bool            `yaml:"enabled"`
	DeliveryMode            string          `yaml:"delivery_mode"`
	VerifyMode              string          `yaml:"verify_mode"`
	NameSpoiler             bool            `yaml:"name_spoiler"`
	BanSeconds              int             `yaml:"ban_seconds"`
	LookupTTLSeconds        int             `yaml:"lookup_ttl_seconds"`
	LookupAutoDeleteEnabled bool            `yaml:"lookup_auto_delete_enabled"`
	TimeoutSeconds          int             `yaml:"timeout_seconds"`
	VerifyMaxFails          int             `yaml:"verify_max_fails"`
	VerifyRetrySeconds      int             `yaml:"verify_retry_seconds"`
	MuteSeconds             int             `yaml:"mute_seconds"`
	VerifyInvited           bool            `yaml:"verify_invited"`
	WarnLimit               int             `yaml:"warn_limit"`
	AntispamEnabled         bool            `yaml:"antispam_enabled"`
	ChannelWhitelist        []int64         `yaml:"channel_whitelist"`
	TrustedMemberGroupIDs   []int64         `yaml:"trusted_member_group_ids"`
	KnownChatIDs            []int64         `yaml:"known_chat_ids"`
	RequiredChannelID       int64           `yaml:"required_channel_id"`
	ChannelDisplay          string          `yaml:"channel_display"`
	ChannelInviteURL        string          `yaml:"channel_invite_url"`
	Questions               []Question      `yaml:"questions"`
	FallbackQuestions       []ShortQuestion `yaml:"fallback_questions"`
	FallbackBuiltin         bool            `yaml:"fallback_builtin"`
	Lang                    string          `yaml:"lang"`
	RichMessages            bool            `yaml:"rich_messages"`
	PrivateQueryPerMin      int             `yaml:"private_query_per_min"`
	AdminLogChatID          int64           `yaml:"admin_log_chat_id"`
	RequiredChannelFailOpen bool            `yaml:"required_channel_fail_open"`
}

var embeddedDefaults = mustParseDefaults()

func mustParseDefaults() defaultsDocument {
	var defaults defaultsDocument
	if err := yaml.Unmarshal([]byte(defaultsYAML), &defaults); err != nil {
		panic(fmt.Sprintf("parse embedded settings defaults: %v", err))
	}
	return defaults
}

func defaultConfig() Config {
	return Config{
		OwnerClaimLifetimeSeconds: embeddedDefaults.Process.OwnerClaimLifetimeSeconds,
		OwnerClaimUserID:          embeddedDefaults.Process.OwnerClaimUserID,
		NotifyTTLSeconds:          embeddedDefaults.Process.NotifyTTLSeconds,
		PrivateQueryPerMin:        embeddedDefaults.Process.PrivateQueryPerMin,
		StatsTimezone:             embeddedDefaults.Process.StatsTimezone,
		UserAgent:                 embeddedDefaults.Process.UserAgent,
		PrivateReply:              embeddedDefaults.Process.PrivateReply,
		Overlays:                  append([]OverlayCfg(nil), embeddedDefaults.Resources.Overlays...),
		NewsURL:                   embeddedDefaults.Resources.NewsURL,
		Feeds:                     append([]FeedConfig(nil), embeddedDefaults.Resources.Feeds...),
	}
}

func factoryBaseline() GroupBaseline {
	defaults := embeddedDefaults.Factory
	return GroupBaseline{
		Enabled:                 factoryValue(defaults.Enabled),
		DeliveryMode:            factoryValue(defaults.DeliveryMode),
		VerifyMode:              factoryValue(defaults.VerifyMode),
		NameSpoiler:             factoryValue(defaults.NameSpoiler),
		BanSeconds:              factoryValue(defaults.BanSeconds),
		LookupTTLSeconds:        factoryValue(defaults.LookupTTLSeconds),
		LookupAutoDeleteEnabled: factoryValue(defaults.LookupAutoDeleteEnabled),
		TimeoutSeconds:          factoryValue(defaults.TimeoutSeconds),
		VerifyMaxFails:          factoryValue(defaults.VerifyMaxFails),
		VerifyRetrySeconds:      factoryValue(defaults.VerifyRetrySeconds),
		MuteSeconds:             factoryValue(defaults.MuteSeconds),
		VerifyInvited:           factoryValue(defaults.VerifyInvited),
		WarnLimit:               factoryValue(defaults.WarnLimit),
		AntispamEnabled:         factoryValue(defaults.AntispamEnabled),
		ChannelWhitelist:        factoryValue(append([]int64(nil), defaults.ChannelWhitelist...)),
		TrustedMemberGroupIDs:   factoryValue(append([]int64(nil), defaults.TrustedMemberGroupIDs...)),
		KnownChatIDs:            factoryValue(append([]int64(nil), defaults.KnownChatIDs...)),
		RequiredChannelID:       factoryValue(defaults.RequiredChannelID),
		ChannelDisplay:          factoryValue(defaults.ChannelDisplay),
		ChannelInviteURL:        factoryValue(defaults.ChannelInviteURL),
		Questions:               factoryValue(cloneQuestions(defaults.Questions)),
		FallbackQuestions:       factoryValue(cloneShortQuestions(defaults.FallbackQuestions)),
		FallbackBuiltin:         factoryValue(defaults.FallbackBuiltin),
		Lang:                    factoryValue(defaults.Lang),
		RichMessages:            factoryValue(defaults.RichMessages),
		PrivateQueryPerMin:      factoryValue(defaults.PrivateQueryPerMin),
		AdminLogChatID:          factoryValue(defaults.AdminLogChatID),
		RequiredChannelFailOpen: factoryValue(defaults.RequiredChannelFailOpen),
	}
}

func factoryValue[T any](value T) BaselineValue[T] {
	return BaselineValue[T]{Value: value, Source: SourceFactory}
}

func userFileValue[T any](value T) BaselineValue[T] {
	return BaselineValue[T]{Value: value, Source: SourceUserFile}
}

func inputValue[T any](value T, managedByFile bool) BaselineValue[T] {
	if managedByFile {
		return userFileValue(value)
	}
	return factoryValue(value)
}

type configPresence map[string]any

func readConfigPresence(path string) (configPresence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return configPresence{}, nil
		}
		return nil, fmt.Errorf("read settings user-file sources: %w", err)
	}
	var top configPresence
	if err = yaml.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parse settings user-file sources: %w", err)
	}
	return top, nil
}

func (p configPresence) has(key string) bool {
	value, ok := p[key]
	return ok && value != nil
}

// LoadBaseline expands legacy top-level file values into configured chats. The factory remains
// immutable, so a subsequently registered chat cannot inherit another chat's file-managed values.
func LoadBaseline(configPath string, cfg *Config) (SettingsBaseline, error) {
	presence, err := readConfigPresence(configPath)
	if err != nil {
		return SettingsBaseline{}, err
	}
	return settingsBaselineFromConfig(cfg, presence), nil
}

func settingsBaselineFromConfig(cfg *Config, presence configPresence) SettingsBaseline {
	factory := factoryBaseline()
	configuredTemplate := applyTopLevelUserValues(factory, cfg, presence)
	baseline := SettingsBaseline{Factory: factory}
	baseline.Groups = make([]GroupBaseline, 0, max(len(cfg.Groups), len(cfg.GroupIDs)))
	seen := make(map[int64]bool, max(len(cfg.Groups), len(cfg.GroupIDs)))
	for i := range cfg.Groups {
		configured := &cfg.Groups[i]
		group := applyGroupUserValues(configuredTemplate, configured)
		group.ID = configured.ID
		baseline.Groups = append(baseline.Groups, group)
		seen[group.ID] = true
	}
	for _, chatID := range cfg.GroupIDs {
		if chatID == 0 || seen[chatID] {
			continue
		}
		group := cloneGroupBaseline(configuredTemplate)
		group.ID = chatID
		baseline.Groups = append(baseline.Groups, group)
		seen[chatID] = true
	}
	return baseline
}
