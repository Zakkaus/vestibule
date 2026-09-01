package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
)

type configValidationRule struct {
	field    string
	validate func(*Config) error
}

var configValidationRules = [...]configValidationRule{
	{field: "overlays", validate: validateConfigOverlays},
	{field: "questions", validate: validateConfigQuestions},
	{field: "fallback_questions", validate: validateConfigFallbackQuestions},
	{field: "lang", validate: validateConfigLanguage},
	{field: "verify_mode", validate: validateConfigVerifyMode},
	{field: "delivery_mode", validate: validateConfigDeliveryMode},
	{field: "groups", validate: validateConfigGroups},
	{field: "default runtime group", validate: validateDefaultRuntimeGroup},
	{field: "durations", validate: validateConfigDurations},
	{field: "owner_claim_lifetime_seconds", validate: validateOwnerClaimLifetime},
	{field: "owner_claim_user_id", validate: validateOwnerClaimUser},
}

type groupConfigValidationRule struct {
	field    string
	validate func(*Config, *GroupConfig) error
}

var groupConfigValidationRules = [...]groupConfigValidationRule{
	{field: "questions", validate: validateGroupQuestions},
	{field: "verify_mode", validate: validateGroupVerifyMode},
	{field: "delivery_mode", validate: validateGroupDeliveryMode},
	{field: "lang", validate: validateGroupLanguage},
	{field: "effective questions", validate: validateGroupEffectiveQuestions},
	{field: "required_channel_id", validate: validateGroupRequiredChannel},
}

type configDurationRule struct {
	key     string
	seconds func(*Config) int
	present func(*Config) bool
	maximum int64
}

var configDurationRules = [...]configDurationRule{
	{key: "timeout_seconds", seconds: func(c *Config) int { return c.TimeoutSeconds }, maximum: maxDurationSeconds},
	{key: "notify_ttl_seconds", seconds: func(c *Config) int { return c.NotifyTTLSeconds }, maximum: maxMessageTTLSeconds},
	{
		key:     "lookup_ttl_seconds",
		seconds: func(c *Config) int { return *c.LookupTTLSeconds },
		present: func(c *Config) bool { return c.LookupTTLSeconds != nil },
		maximum: maxMessageTTLSeconds,
	},
	{key: "ban_seconds", seconds: func(c *Config) int { return c.BanSeconds }, maximum: maxDurationSeconds},
	{key: "mute_seconds", seconds: func(c *Config) int { return c.MuteSeconds }, maximum: maxDurationSeconds},
	{key: "verify_retry_seconds", seconds: func(c *Config) int { return c.VerifyRetrySeconds }, maximum: maxVerifyRetrySeconds},
	{
		key:     "owner_claim_lifetime_seconds",
		seconds: func(c *Config) int { return c.OwnerClaimLifetimeSeconds },
		maximum: maxOwnerClaimLifetimeSeconds,
	},
}

// LoadConfig reads, validates, defaults, and normalizes a JSON configuration file.
func LoadConfig(path string) (*Config, error) {
	data, err := readConfig(path)
	if err != nil {
		return nil, err
	}
	c := defaultConfig()
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	warnUnknownConfigKeys(data)
	if err := mergeConfigGroups(&c); err != nil {
		return nil, err
	}
	if err := validateConfig(&c); err != nil {
		return nil, err
	}
	applyConfigDefaults(&c)
	if err := normalizeConfigFeeds(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func readConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return []byte("{}"), nil
}

func mergeConfigGroups(c *Config) error {
	legacy := c.GroupIDs
	if c.GroupID != 0 {
		legacy = append(legacy, c.GroupID)
	}
	for _, id := range legacy {
		if c.group(id) == nil {
			c.Groups = append(c.Groups, GroupConfig{ID: id})
		}
	}
	c.GroupIDs = make([]int64, 0, len(c.Groups))
	for i := range c.Groups {
		c.GroupIDs = append(c.GroupIDs, c.Groups[i].ID)
	}
	seenGroup := map[int64]bool{}
	for i := range c.Groups {
		id := c.Groups[i].ID
		if id == 0 {
			return fmt.Errorf("group id 0 is invalid (a Telegram group/supergroup id is negative)")
		}
		if seenGroup[id] {
			return fmt.Errorf("duplicate group id %d", id)
		}
		seenGroup[id] = true
	}
	return nil
}

func validateConfig(c *Config) error {
	for _, rule := range configValidationRules {
		if err := rule.validate(c); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigOverlays(c *Config) error {
	seenOverlay := map[string]bool{}
	for i, overlay := range c.Overlays {
		parts := strings.Split(overlay.Repo, "/")
		if overlay.Repo == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("overlay %d: repo must be \"owner/name\" (got %q)", i, overlay.Repo)
		}
		name := overlay.Name
		if name == "" {
			name = overlay.Repo
		}
		if seenOverlay[name] {
			return fmt.Errorf("duplicate overlay name %q", name)
		}
		seenOverlay[name] = true
	}
	return nil
}

func validateConfigQuestions(c *Config) error {
	return validateConfigQuestionList(c.Questions, "global")
}

func validateConfigQuestionList(questions []Question, where string) error {
	for i, question := range questions {
		if len(question.Options) < 2 {
			return fmt.Errorf("%s question %d: need at least 2 options", where, i)
		}
		if question.Answer < 0 || question.Answer >= len(question.Options) {
			return fmt.Errorf("%s question %d: answer index %d out of range", where, i, question.Answer)
		}
	}
	return nil
}

func validateConfigFallbackQuestions(c *Config) error {
	for i, question := range c.FallbackQuestions {
		if strings.TrimSpace(question.Q) == "" || len(question.Answers) == 0 {
			return fmt.Errorf("fallback_questions %d requires q and at least one answers entry", i)
		}
		for _, answer := range question.Answers {
			if strings.TrimSpace(answer) == "" {
				return fmt.Errorf("fallback_questions %d: answers must not contain an empty string", i)
			}
		}
	}
	return nil
}

func validateConfigLanguage(c *Config) error {
	if !ValidLanguage(c.Lang) {
		return fmt.Errorf("lang %q is not one of %q, %q, %q", c.Lang, "zh", "zh-Hant", "en")
	}
	return nil
}

func validateConfigVerifyMode(c *Config) error {
	if c.VerifyMode != "" && !ValidMode(c.VerifyMode) {
		return fmt.Errorf("verify_mode %q is not one of %q, %q, %q", c.VerifyMode, ModeKernel, ModeQuiz, ModeMixed)
	}
	return nil
}

func validateConfigDeliveryMode(c *Config) error {
	if c.DeliveryMode != "" && !ValidDeliveryMode(c.DeliveryMode) {
		return fmt.Errorf("delivery_mode %q is not one of %q, %q, %q", c.DeliveryMode, DeliveryGroup, DeliveryDM, DeliveryBoth)
	}
	return nil
}

func validateConfigGroups(c *Config) error {
	for i := range c.Groups {
		if err := validateConfigGroup(c, &c.Groups[i]); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigGroup(c *Config, group *GroupConfig) error {
	for _, rule := range groupConfigValidationRules {
		if err := rule.validate(c, group); err != nil {
			return err
		}
	}
	return nil
}

func validateGroupQuestions(_ *Config, group *GroupConfig) error {
	return validateConfigQuestionList(group.Questions, fmt.Sprintf("group %d", group.ID))
}

func validateGroupVerifyMode(_ *Config, group *GroupConfig) error {
	if group.VerifyMode != "" && !ValidMode(group.VerifyMode) {
		return fmt.Errorf("group %d: verify_mode %q is not one of %q, %q, %q", group.ID, group.VerifyMode, ModeKernel, ModeQuiz, ModeMixed)
	}
	return nil
}

func validateGroupDeliveryMode(_ *Config, group *GroupConfig) error {
	if group.DeliveryMode != "" && !ValidDeliveryMode(group.DeliveryMode) {
		return fmt.Errorf("group %d: delivery_mode %q is not one of %q, %q, %q", group.ID, group.DeliveryMode, DeliveryGroup, DeliveryDM, DeliveryBoth)
	}
	return nil
}

func validateGroupLanguage(_ *Config, group *GroupConfig) error {
	if !ValidLanguage(group.Lang) {
		return fmt.Errorf("group %d: lang %q is not one of %q, %q, %q", group.ID, group.Lang, "zh", "zh-Hant", "en")
	}
	return nil
}

func validateGroupEffectiveQuestions(c *Config, group *GroupConfig) error {
	if c.VerifyModeFor(group.ID) != ModeKernel && len(c.QuestionsFor(group.ID)) == 0 {
		return fmt.Errorf("group %d: no questions (add global questions or this group's own questions, or set verify_mode to %q)", group.ID, ModeKernel)
	}
	return nil
}

func validateGroupRequiredChannel(c *Config, group *GroupConfig) error {
	if c.RequiredChannel(group.ID) != 0 && c.ChannelInvite(group.ID) == "" && !strings.HasPrefix(c.ChannelDisplayFor(group.ID), "@") {
		return fmt.Errorf("group %d: required_channel_id is set but the channel has no reachable link (set channel_display to an @handle, or channel_invite_url for a private channel)", group.ID)
	}
	return nil
}

func validateDefaultRuntimeGroup(c *Config) error {
	if len(c.Groups) != 0 {
		return nil
	}
	if c.VerifyMode != "" && c.VerifyMode != ModeKernel && len(c.Questions) == 0 {
		return fmt.Errorf("default runtime group: no questions (add global questions or set verify_mode to %q)", ModeKernel)
	}
	if c.RequiredChannelID != 0 && c.ChannelInviteURL == "" && !strings.HasPrefix(c.ChannelDisplay, "@") {
		return fmt.Errorf("default runtime group: required_channel_id is set but the channel has no reachable link (set channel_display to an @handle, or channel_invite_url for a private channel)")
	}
	return nil
}

func validateConfigDurations(c *Config) error {
	for _, rule := range configDurationRules {
		if rule.present != nil && !rule.present(c) {
			continue
		}
		if err := validatePositiveConfigDuration(rule.key, rule.seconds(c), rule.maximum); err != nil {
			return err
		}
	}
	return nil
}

func validateOwnerClaimLifetime(c *Config) error {
	if c.OwnerClaimLifetimeSeconds < 0 {
		return fmt.Errorf("owner_claim_lifetime_seconds must not be negative")
	}
	return nil
}

func validateOwnerClaimUser(c *Config) error {
	if c.OwnerClaimUserID < 0 {
		return fmt.Errorf("owner_claim_user_id must not be negative")
	}
	return nil
}

func applyConfigDefaults(c *Config) {
	if c.OwnerClaimLifetimeSeconds == 0 {
		c.OwnerClaimLifetimeSeconds = 10 * 60
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 240
	}
	if c.TimeoutSeconds < 30 {
		c.TimeoutSeconds = 30
	}
	if c.TimeoutSeconds > 1800 {
		c.TimeoutSeconds = 1800
	}
	if c.NotifyTTLSeconds == 0 {
		c.NotifyTTLSeconds = 60
	}
	if c.WarnLimit <= 0 {
		c.WarnLimit = 3
	}
	if c.PrivateQueryPerMin <= 0 {
		c.PrivateQueryPerMin = 3
	}
	if c.VerifyRetrySeconds == 0 {
		c.VerifyRetrySeconds = 180
	}
	if c.VerifyMaxFails == 0 {
		c.VerifyMaxFails = 3
	}
	if c.MuteSeconds <= 0 {
		c.MuteSeconds = 3600
	}
	c.BanSeconds = ClampBanSeconds(c.BanSeconds)
	c.MuteSeconds = clampMuteSecs(c.MuteSeconds)
}

func normalizeConfigFeeds(c *Config) error {
	if c.Feed != nil {
		c.Feeds = append(c.Feeds, *c.Feed)
	}
	for i := range c.Feeds {
		if err := validatePositiveConfigDuration(
			fmt.Sprintf("feeds[%d].interval_seconds", i), c.Feeds[i].IntervalSeconds, maxFeedIntervalSeconds,
		); err != nil {
			return err
		}
		if !ValidLanguage(c.Feeds[i].Lang) {
			return fmt.Errorf("feed %d: lang %q is not one of %q, %q, %q", i, c.Feeds[i].Lang, "zh", "zh-Hant", "en")
		}
	}
	seenFeed := map[int64]bool{}
	deduped := c.Feeds[:0]
	for _, feed := range c.Feeds {
		if feed.ChatID != 0 && seenFeed[feed.ChatID] {
			log.Printf("config: duplicate feed for chat_id %d ignored (feed state is per chat)", feed.ChatID)
			continue
		}
		seenFeed[feed.ChatID] = true
		deduped = append(deduped, feed)
	}
	c.Feeds = deduped
	return nil
}

func warnUnknownJSONKeys(raw json.RawMessage, typ reflect.Type, where string) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return
	}
	known := make(map[string]struct{}, typ.NumField())
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			known[name] = struct{}{}
		}
	}
	var unknown []string
	for name := range object {
		if _, ok := known[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		log.Printf("WARNING: %s: unknown key %q", where, name)
	}
}

func warnUnknownJSONEntries(raw json.RawMessage, typ reflect.Type, where string) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return
	}
	for i, entry := range entries {
		warnUnknownJSONKeys(entry, typ, fmt.Sprintf("%s[%d]", where, i))
	}
}

func warnUnknownConfigKeys(data []byte) {
	warnUnknownJSONKeys(data, reflect.TypeOf(Config{}), "config")
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return
	}
	warnUnknownJSONEntries(top["groups"], reflect.TypeOf(GroupConfig{}), "config groups")
	warnUnknownJSONEntries(top["feeds"], reflect.TypeOf(FeedConfig{}), "config feeds")
	if raw, ok := top["feed"]; ok {
		warnUnknownJSONKeys(raw, reflect.TypeOf(FeedConfig{}), "config feed")
	}
}
