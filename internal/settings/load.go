package settings

import (
	"encoding/json"
	"fmt"
	"strconv"

	"go.mau.fi/util/configupgrade"
	"gopkg.in/yaml.v3"
)

const currentSettingsBase = `
version: 3
registration_revision: 0
owner_id: 0
owner_claim_nonce: ""
owner_claim_expires_at: 0
registered_groups: []
enrollment_nonces: []
pending_registrations: []
unknown_group_leaves: []
groups: {}
`

const currentGroupRecordBase = `
revision: 0
enabled:
delivery_mode:
verify_mode:
name_spoiler:
ban_seconds:
lookup_ttl_seconds:
lookup_auto_delete_enabled:
timeout_seconds:
verify_max_fails:
verify_retry_seconds:
mute_seconds:
verify_invited:
warn_limit:
antispam_enabled:
channel_whitelist:
trusted_member_group_ids:
known_chat_ids:
required_channel_id:
channel_display:
channel_invite_url:
questions:
fallback_questions:
fallback_builtin:
lang:
rich_messages:
private_query_per_min:
admin_log_chat_id:
required_channel_fail_open:
`

type copyRule struct {
	kind configupgrade.YAMLType
	path []string
}

var settingsCopyRules = []copyRule{
	{configupgrade.Int, []string{"registration_revision"}},
	{configupgrade.Int, []string{"owner_id"}},
	{configupgrade.Str, []string{"owner_claim_nonce"}},
	{configupgrade.Int, []string{"owner_claim_expires_at"}},
	{configupgrade.List, []string{"registered_groups"}},
	{configupgrade.List, []string{"enrollment_nonces"}},
	{configupgrade.List, []string{"pending_registrations"}},
	{configupgrade.List, []string{"unknown_group_leaves"}},
}

var groupCopyRules = []copyRule{
	{configupgrade.Int, []string{"revision"}},
	{configupgrade.Bool, []string{"enabled"}},
	{configupgrade.Str, []string{"delivery_mode"}},
	{configupgrade.Str, []string{"verify_mode"}},
	{configupgrade.Bool, []string{"name_spoiler"}},
	{configupgrade.Int, []string{"ban_seconds"}},
	{configupgrade.Int, []string{"lookup_ttl_seconds"}},
	{configupgrade.Bool, []string{"lookup_auto_delete_enabled"}},
	{configupgrade.Int, []string{"timeout_seconds"}},
	{configupgrade.Int, []string{"verify_max_fails"}},
	{configupgrade.Int, []string{"verify_retry_seconds"}},
	{configupgrade.Int, []string{"mute_seconds"}},
	{configupgrade.Bool, []string{"verify_invited"}},
	{configupgrade.Int, []string{"warn_limit"}},
	{configupgrade.Bool, []string{"antispam_enabled"}},
	{configupgrade.List, []string{"channel_whitelist"}},
	{configupgrade.List, []string{"trusted_member_group_ids"}},
	{configupgrade.List, []string{"known_chat_ids"}},
	{configupgrade.Int, []string{"required_channel_id"}},
	{configupgrade.Str, []string{"channel_display"}},
	{configupgrade.Str, []string{"channel_invite_url"}},
	{configupgrade.List, []string{"questions"}},
	{configupgrade.List, []string{"fallback_questions"}},
	{configupgrade.Bool, []string{"fallback_builtin"}},
	{configupgrade.Str, []string{"lang"}},
	{configupgrade.Bool, []string{"rich_messages"}},
	{configupgrade.Int, []string{"private_query_per_min"}},
	{configupgrade.Int, []string{"admin_log_chat_id"}},
	{configupgrade.Bool, []string{"required_channel_fail_open"}},
}

func applyCopyRule(helper configupgrade.Helper, rule copyRule) {
	source := helper.GetNode(rule.path...)
	target := helper.GetBaseNode(rule.path...)
	if source == nil || target == nil || rule.kind&yamlNodeType(source.Node) == 0 {
		return
	}
	*target.Node = *cloneYAMLNode(source.Node)
}

func yamlNodeType(node *yaml.Node) configupgrade.YAMLType {
	switch node.Tag {
	case configupgrade.NullTag:
		return configupgrade.Null
	case configupgrade.BoolTag:
		return configupgrade.Bool
	case configupgrade.StrTag:
		return configupgrade.Str
	case configupgrade.IntTag:
		return configupgrade.Int
	case configupgrade.FloatTag:
		return configupgrade.Float
	case configupgrade.TimestampTag:
		return configupgrade.Timestamp
	case configupgrade.SeqTag:
		return configupgrade.List
	case configupgrade.MapTag:
		return configupgrade.Map
	case configupgrade.BinaryTag:
		return configupgrade.Binary
	default:
		return 0
	}
}

func cloneYAMLNode(source *yaml.Node) *yaml.Node {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.Content = make([]*yaml.Node, len(source.Content))
	for i, child := range source.Content {
		cloned.Content[i] = cloneYAMLNode(child)
	}
	if source.Alias != nil {
		cloned.Alias = cloneYAMLNode(source.Alias)
	}
	return &cloned
}

type legacySettingsUpgrader struct {
	chatIDs []int64
	err     error
}

func (u *legacySettingsUpgrader) GetBase() string { return currentSettingsBase }

func (u *legacySettingsUpgrader) DoUpgrade(helper configupgrade.Helper) {
	for _, rule := range settingsCopyRules {
		applyCopyRule(helper, rule)
	}
	groups := u.upgradeExistingGroupRecords(helper)
	if u.err != nil {
		return
	}
	u.addConfiguredGroupRecords(helper, groups)
	if u.err != nil {
		return
	}
	helper.SetMap(groups, "groups")
}

func (u *legacySettingsUpgrader) upgradeExistingGroupRecords(helper configupgrade.Helper) configupgrade.YAMLMap {
	groups := make(configupgrade.YAMLMap)
	source := helper.GetNode("groups")
	if source == nil || source.Map == nil {
		return groups
	}
	for chatID, sourceRecord := range source.Map {
		upgradedRecord, err := upgradeGroupRecord(sourceRecord)
		if err != nil {
			u.err = err
			return groups
		}
		var record groupRecord
		if u.err = decodeYAMLNode(upgradedRecord.Node, &record); u.err != nil {
			return groups
		}
		groups[chatID], u.err = encodeYAMLNode(record)
		if u.err != nil {
			return groups
		}
	}
	return groups
}

func (u *legacySettingsUpgrader) addConfiguredGroupRecords(helper configupgrade.Helper, groups configupgrade.YAMLMap) {
	for _, chatID := range u.chatIDs {
		key := strconv.FormatInt(chatID, 10)
		recordNode, ok := groups[key]
		if !ok {
			recordNode, u.err = emptyGroupRecordNode()
			if u.err != nil {
				return
			}
		}
		var record groupRecord
		if u.err = decodeYAMLNode(recordNode.Node, &record); u.err != nil {
			return
		}
		if u.applyLegacyTopLevelValues(helper, &record) && record.Revision == 0 {
			record.Revision = 1
		}
		if u.err != nil {
			return
		}
		groups[key], u.err = encodeYAMLNode(record)
		if u.err != nil {
			return
		}
	}
}

func (u *legacySettingsUpgrader) applyLegacyTopLevelValues(helper configupgrade.Helper, record *groupRecord) bool {
	changed := u.applyLegacyEnabled(helper, record)
	if u.err != nil {
		return false
	}
	changed = u.applyLegacyNameSpoiler(helper, record) || changed
	if u.err != nil {
		return false
	}
	return u.applyLegacyVerifyMode(helper, record) || changed
}

func (u *legacySettingsUpgrader) applyLegacyEnabled(helper configupgrade.Helper, record *groupRecord) bool {
	if record.Enabled != nil {
		return false
	}
	value, ok := helper.Get(configupgrade.Bool, "enabled")
	if !ok {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		u.err = err
		return false
	}
	record.Enabled = &parsed
	return true
}

func (u *legacySettingsUpgrader) applyLegacyNameSpoiler(helper configupgrade.Helper, record *groupRecord) bool {
	if record.NameSpoiler != nil {
		return false
	}
	value, ok := helper.Get(configupgrade.Bool, "name_spoiler")
	if !ok {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		u.err = err
		return false
	}
	record.NameSpoiler = &parsed
	return true
}

func (u *legacySettingsUpgrader) applyLegacyVerifyMode(helper configupgrade.Helper, record *groupRecord) bool {
	if record.VerifyMode != nil {
		return false
	}
	value, ok := helper.Get(configupgrade.Str, "verify_mode")
	if !ok || !ValidMode(value) {
		return false
	}
	record.VerifyMode = &value
	return true
}

func upgradeGroupRecord(source configupgrade.YAMLNode) (configupgrade.YAMLNode, error) {
	var base, input yaml.Node
	if err := yaml.Unmarshal([]byte(currentGroupRecordBase), &base); err != nil {
		return configupgrade.YAMLNode{}, fmt.Errorf("parse current group settings base: %w", err)
	}
	input = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{source.Node}}
	helper := configupgrade.NewHelper(&base, &input)
	for _, rule := range groupCopyRules {
		applyCopyRule(helper, rule)
	}
	if helper.GetNode("lookup_auto_delete_enabled") == nil {
		if value, ok := helper.Get(configupgrade.Int, "lookup_ttl_seconds"); ok {
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return configupgrade.YAMLNode{}, fmt.Errorf("parse lookup_ttl_seconds: %w", err)
			}
			helper.Set(configupgrade.Bool, strconv.FormatBool(seconds > 0), "lookup_auto_delete_enabled")
			if seconds <= 0 {
				helper.Set(configupgrade.Null, "", "lookup_ttl_seconds")
			}
		}
	}
	if helper.GetNode("delivery_mode") == nil {
		if value, ok := helper.Get(configupgrade.Bool, "dm_first"); ok {
			dmFirst, err := strconv.ParseBool(value)
			if err != nil {
				return configupgrade.YAMLNode{}, fmt.Errorf("parse dm_first: %w", err)
			}
			mode := DeliveryGroup
			if dmFirst {
				mode = DeliveryDM
			}
			helper.Set(configupgrade.Str, mode, "delivery_mode")
		}
	}
	return helper.Base, nil
}

func emptyGroupRecordNode() (configupgrade.YAMLNode, error) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(currentGroupRecordBase), &node); err != nil {
		return configupgrade.YAMLNode{}, err
	}
	return configupgrade.NewHelper(&node, &node).Base, nil
}

func encodeYAMLNode(value any) (configupgrade.YAMLNode, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return configupgrade.YAMLNode{}, err
	}
	var node yaml.Node
	if err = yaml.Unmarshal(data, &node); err != nil {
		return configupgrade.YAMLNode{}, err
	}
	return configupgrade.NewHelper(&node, &node).Base, nil
}

func decodeYAMLNode(node *yaml.Node, target any) error {
	data, err := yaml.Marshal(node)
	if err != nil {
		return err
	}
	return decodeYAML(data, target)
}

func decodeYAML(data []byte, target any) error {
	var value any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return err
	}
	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, target)
}

func upgradeSettingsFile(path string, chatIDs []int64, save bool) (settingsFile, error) {
	upgrader := &legacySettingsUpgrader{chatIDs: append([]int64(nil), chatIDs...)}
	data, _, err := configupgrade.Do(path, save, upgrader)
	if err != nil {
		return settingsFile{}, err
	}
	if upgrader.err != nil {
		return settingsFile{}, upgrader.err
	}
	var upgraded settingsFile
	if err = decodeYAML(data, &upgraded); err != nil {
		return settingsFile{}, fmt.Errorf("decode upgraded settings: %w", err)
	}
	return normalizeFile(upgraded), nil
}
