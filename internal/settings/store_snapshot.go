package settings

import (
	"fmt"
	"strings"
)

type effectiveGroupValidator func(*effectiveGroup) error

var effectiveGroupValidators = [...]effectiveGroupValidator{
	validateEffectiveDeliveryMode,
	validateEffectiveVerifyMode,
	validateEffectiveLanguage,
	validateEffectiveBanSeconds,
	validateEffectiveLookupTTL,
	validateEffectiveMuteSeconds,
	validateEffectiveWarnLimit,
	validateEffectiveTimeout,
	validateEffectivePrivateQueryRate,
	validateEffectiveIDLists,
	validateEffectiveRequiredChannel,
	validateEffectiveQuestions,
	validateEffectiveFallbackQuestions,
}

func (s *Store) buildSnapshot(state settingsFile) (*settingsSnapshot, error) {
	registration := RegistrationState{
		Revision:             state.RegistrationRevision,
		OwnerID:              state.OwnerID,
		OwnerClaimNonce:      state.OwnerClaimNonce,
		OwnerClaimExpiresAt:  state.OwnerClaimExpiresAt,
		RegisteredGroups:     append([]RegisteredGroup(nil), state.RegisteredGroups...),
		EnrollmentNonces:     append([]EnrollmentNonce(nil), state.EnrollmentNonces...),
		PendingRegistrations: append([]PendingRegistration(nil), state.PendingRegistrations...),
		UnknownGroupLeaves:   append([]UnknownGroupLeave(nil), state.UnknownGroupLeaves...),
	}
	if err := s.validateRegistrations(registration); err != nil {
		return nil, err
	}

	groups := make(map[int64]*effectiveGroup, len(s.baseline.Groups)+len(state.RegisteredGroups))
	order := make([]int64, 0, len(s.baseline.Groups)+len(state.RegisteredGroups))
	for _, baseline := range s.baseline.Groups {
		record := state.Groups[baseline.ID]
		group := buildEffectiveGroup(baseline, record, false)
		if err := validateEffectiveGroup(group); err != nil {
			return nil, fmt.Errorf("group %d: %w", baseline.ID, err)
		}
		groups[baseline.ID] = group
		order = append(order, baseline.ID)
	}
	for _, registered := range state.RegisteredGroups {
		baseline := cloneGroupBaseline(s.baseline.Factory)
		baseline.ID = registered.ID
		record := state.Groups[registered.ID]
		group := buildEffectiveGroup(baseline, record, true)
		if err := validateEffectiveGroup(group); err != nil {
			return nil, fmt.Errorf("group %d: %w", registered.ID, err)
		}
		groups[registered.ID] = group
		order = append(order, registered.ID)
	}
	return &settingsSnapshot{groups: groups, groupIDs: order, registration: registration}, nil
}

func buildEffectiveGroup(baseline GroupBaseline, record groupRecord, registered bool) *effectiveGroup {
	builtin := resolve(record.FallbackBuiltin, baseline.FallbackBuiltin)
	fallback := resolveSlice(record.FallbackQuestions, baseline.FallbackQuestions, cloneShortQuestions)
	if builtin.Value {
		fallback.Value = []ShortQuestion{}
		if record.FallbackBuiltin != nil {
			fallback.Source = SourceChatOverride
		} else if baseline.FallbackBuiltin.Value {
			fallback.Source = baseline.FallbackBuiltin.Source
		}
	}
	return &effectiveGroup{
		id:               baseline.ID,
		revision:         record.Revision,
		registered:       registered,
		baseline:         cloneGroupBaseline(baseline),
		overrides:        cloneGroupOverrides(record.GroupOverrides),
		enabled:          resolve(record.Enabled, baseline.Enabled),
		deliveryMode:     resolve(record.DeliveryMode, baseline.DeliveryMode),
		verifyMode:       resolve(record.VerifyMode, baseline.VerifyMode),
		nameSpoiler:      resolve(record.NameSpoiler, baseline.NameSpoiler),
		banSeconds:       resolve(record.BanSeconds, baseline.BanSeconds),
		lookupTTLSeconds: resolve(record.LookupTTLSeconds, baseline.LookupTTLSeconds),
		lookupAutoDeleteEnabled: resolve(
			record.LookupAutoDeleteEnabled,
			baseline.LookupAutoDeleteEnabled,
		),
		timeoutSeconds:          resolve(record.TimeoutSeconds, baseline.TimeoutSeconds),
		verifyMaxFails:          resolve(record.VerifyMaxFails, baseline.VerifyMaxFails),
		verifyRetrySeconds:      resolve(record.VerifyRetrySeconds, baseline.VerifyRetrySeconds),
		muteSeconds:             resolve(record.MuteSeconds, baseline.MuteSeconds),
		verifyInvited:           resolve(record.VerifyInvited, baseline.VerifyInvited),
		warnLimit:               resolve(record.WarnLimit, baseline.WarnLimit),
		antispamEnabled:         resolve(record.AntispamEnabled, baseline.AntispamEnabled),
		channelWhitelist:        resolveSlice(record.ChannelWhitelist, baseline.ChannelWhitelist, cloneInt64s),
		trustedMemberGroupIDs:   resolveSlice(record.TrustedMemberGroupIDs, baseline.TrustedMemberGroupIDs, cloneInt64s),
		knownChatIDs:            resolveSlice(record.KnownChatIDs, baseline.KnownChatIDs, cloneInt64s),
		requiredChannelID:       resolve(record.RequiredChannelID, baseline.RequiredChannelID),
		channelDisplay:          resolve(record.ChannelDisplay, baseline.ChannelDisplay),
		channelInviteURL:        resolve(record.ChannelInviteURL, baseline.ChannelInviteURL),
		questions:               resolveSlice(record.Questions, baseline.Questions, cloneQuestions),
		fallbackQuestions:       fallback,
		fallbackBuiltin:         builtin,
		lang:                    resolve(record.Lang, baseline.Lang),
		richMessages:            resolve(record.RichMessages, baseline.RichMessages),
		privateQueryPerMin:      resolve(record.PrivateQueryPerMin, baseline.PrivateQueryPerMin),
		adminLogChatID:          resolve(record.AdminLogChatID, baseline.AdminLogChatID),
		requiredChannelFailOpen: resolve(record.RequiredChannelFailOpen, baseline.RequiredChannelFailOpen),
	}
}

func resolve[T any](override *T, baseline BaselineValue[T]) Setting[T] {
	if override != nil {
		return Setting[T]{Value: *override, Source: SourceChatOverride}
	}
	return Setting[T](baseline)
}

func resolveSlice[T any](override *[]T, baseline BaselineValue[[]T], clone func([]T) []T) Setting[[]T] {
	if override != nil {
		return Setting[[]T]{Value: clone(*override), Source: SourceChatOverride}
	}
	return Setting[[]T]{Value: clone(baseline.Value), Source: baseline.Source}
}

func normalizeBaselineLanguages(baseline *SettingsBaseline) {
	if baseline.Factory.Lang.Value == "" {
		baseline.Factory.Lang = BaselineValue[string]{Value: "zh", Source: SourceFactory}
	}
	for i := range baseline.Groups {
		if baseline.Groups[i].Lang.Value == "" {
			baseline.Groups[i].Lang = BaselineValue[string]{Value: "zh", Source: SourceFactory}
		}
	}
}

func validateBaseline(baseline SettingsBaseline) error {
	if err := validateBaselineSources(baseline.Factory); err != nil {
		return fmt.Errorf("default group: %w", err)
	}
	if err := validateEffectiveGroup(buildEffectiveGroup(baseline.Factory, groupRecord{}, false)); err != nil {
		return fmt.Errorf("default group: %w", err)
	}
	seen := make(map[int64]bool, len(baseline.Groups))
	for _, group := range baseline.Groups {
		if group.ID == 0 {
			return fmt.Errorf("settings baseline group ID 0 is invalid")
		}
		if seen[group.ID] {
			return fmt.Errorf("duplicate settings baseline group %d", group.ID)
		}
		seen[group.ID] = true
		if err := validateBaselineSources(group); err != nil {
			return fmt.Errorf("group %d: %w", group.ID, err)
		}
		if err := validateEffectiveGroup(buildEffectiveGroup(group, groupRecord{}, false)); err != nil {
			return fmt.Errorf("group %d: %w", group.ID, err)
		}
	}
	return nil
}

func validateBaselineSources(group GroupBaseline) error {
	sources := []Source{
		group.Enabled.Source, group.DeliveryMode.Source, group.VerifyMode.Source, group.NameSpoiler.Source,
		group.BanSeconds.Source, group.LookupTTLSeconds.Source, group.LookupAutoDeleteEnabled.Source,
		group.TimeoutSeconds.Source, group.VerifyMaxFails.Source, group.VerifyRetrySeconds.Source,
		group.MuteSeconds.Source, group.WarnLimit.Source, group.VerifyInvited.Source,
		group.AntispamEnabled.Source, group.ChannelWhitelist.Source,
		group.TrustedMemberGroupIDs.Source, group.KnownChatIDs.Source, group.RequiredChannelID.Source,
		group.ChannelDisplay.Source, group.ChannelInviteURL.Source, group.Questions.Source,
		group.FallbackQuestions.Source, group.FallbackBuiltin.Source, group.Lang.Source,
		group.RichMessages.Source, group.PrivateQueryPerMin.Source, group.AdminLogChatID.Source,
		group.RequiredChannelFailOpen.Source,
	}
	for _, source := range sources {
		if source != SourceFactory && source != SourceUserFile {
			return fmt.Errorf("baseline source %d is not config or default", source)
		}
	}
	return nil
}

func validateEffectiveGroup(group *effectiveGroup) error {
	for _, validate := range effectiveGroupValidators {
		if err := validate(group); err != nil {
			return err
		}
	}
	return nil
}

func validateEffectiveDeliveryMode(group *effectiveGroup) error {
	if !ValidDeliveryMode(group.deliveryMode.Value) {
		return fmt.Errorf("invalid delivery mode %q", group.deliveryMode.Value)
	}
	return nil
}

func validateEffectiveVerifyMode(group *effectiveGroup) error {
	if !ValidMode(group.verifyMode.Value) {
		return fmt.Errorf("invalid verify mode %q", group.verifyMode.Value)
	}
	return nil
}

func validateEffectiveLanguage(group *effectiveGroup) error {
	if group.lang.Value == "" || !ValidLanguage(group.lang.Value) {
		return fmt.Errorf("invalid language %q", group.lang.Value)
	}
	return nil
}

func validateEffectiveBanSeconds(group *effectiveGroup) error {
	if group.banSeconds.Source == SourceChatOverride && group.banSeconds.Value != ClampBanSeconds(group.banSeconds.Value) {
		return fmt.Errorf("ban_seconds %d is outside Telegram's supported range", group.banSeconds.Value)
	}
	return nil
}

func validateEffectiveLookupTTL(group *effectiveGroup) error {
	if group.lookupTTLSeconds.Source == SourceChatOverride && group.lookupTTLSeconds.Value <= 0 {
		return fmt.Errorf("lookup_ttl_seconds must be positive")
	}
	return nil
}

func validateEffectiveMuteSeconds(group *effectiveGroup) error {
	if group.muteSeconds.Source == SourceChatOverride &&
		(group.muteSeconds.Value <= 0 || group.muteSeconds.Value != ClampBanSeconds(group.muteSeconds.Value)) {
		return fmt.Errorf("mute_seconds %d is outside Telegram's supported range", group.muteSeconds.Value)
	}
	return nil
}

func validateEffectiveWarnLimit(group *effectiveGroup) error {
	if group.warnLimit.Source == SourceChatOverride && group.warnLimit.Value <= 0 {
		return fmt.Errorf("warn_limit must be positive")
	}
	return nil
}

func validateEffectiveTimeout(group *effectiveGroup) error {
	if group.timeoutSeconds.Source == SourceChatOverride && (group.timeoutSeconds.Value < 30 || group.timeoutSeconds.Value > 1800) {
		return fmt.Errorf("timeout_seconds must be between 30 and 1800")
	}
	return nil
}

func validateEffectivePrivateQueryRate(group *effectiveGroup) error {
	if group.privateQueryPerMin.Value <= 0 {
		return fmt.Errorf("private_query_per_min must be positive")
	}
	return nil
}

func validateEffectiveIDLists(group *effectiveGroup) error {
	if group.channelWhitelist.Source == SourceChatOverride {
		if err := validateIDs("channel_whitelist", group.channelWhitelist.Value); err != nil {
			return err
		}
	}
	if group.trustedMemberGroupIDs.Source == SourceChatOverride {
		if err := validateIDs("trusted_member_group_ids", group.trustedMemberGroupIDs.Value); err != nil {
			return err
		}
	}
	if group.knownChatIDs.Source == SourceChatOverride {
		if err := validateIDs("known_chat_ids", group.knownChatIDs.Value); err != nil {
			return err
		}
	}
	return nil
}

func validateEffectiveRequiredChannel(group *effectiveGroup) error {
	if group.requiredChannelID.Source == SourceChatOverride || group.channelDisplay.Source == SourceChatOverride || group.channelInviteURL.Source == SourceChatOverride {
		if group.requiredChannelID.Value != 0 && group.channelInviteURL.Value == "" && !strings.HasPrefix(group.channelDisplay.Value, "@") {
			return fmt.Errorf("required channel has no reachable display or invite URL")
		}
	}
	return nil
}

func validateEffectiveQuestions(group *effectiveGroup) error {
	if group.questions.Source == SourceChatOverride {
		return validateQuestions(group.questions.Value)
	}
	return nil
}

func validateEffectiveFallbackQuestions(group *effectiveGroup) error {
	if group.fallbackBuiltin.Value {
		if group.overrides.FallbackQuestions != nil {
			return fmt.Errorf("fallback_questions cannot be overridden while fallback_builtin is true")
		}
		return nil
	}
	return validateFallbackQuestions(group.fallbackQuestions.Value)
}

func validateQuestions(questions []Question) error {
	for i, question := range questions {
		if strings.TrimSpace(question.Q) == "" {
			return fmt.Errorf("question %d has an empty prompt", i)
		}
		if len(question.Options) < 2 {
			return fmt.Errorf("question %d needs at least two options", i)
		}
		if question.Answer < 0 || question.Answer >= len(question.Options) {
			return fmt.Errorf("question %d answer index %d is out of range", i, question.Answer)
		}
	}
	return nil
}

func validateFallbackQuestions(questions []ShortQuestion) error {
	if len(questions) == 0 {
		return fmt.Errorf("custom fallback questions cannot be empty")
	}
	for i, question := range questions {
		if strings.TrimSpace(question.Q) == "" || len(question.Answers) == 0 {
			return fmt.Errorf("fallback question %d needs a prompt and answers", i)
		}
		for _, answer := range question.Answers {
			if strings.TrimSpace(answer) == "" {
				return fmt.Errorf("fallback question %d has an empty answer", i)
			}
		}
	}
	return nil
}

func validateIDs(name string, ids []int64) error {
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id == 0 {
			return fmt.Errorf("%s contains ID 0", name)
		}
		if seen[id] {
			return fmt.Errorf("%s contains duplicate ID %d", name, id)
		}
		seen[id] = true
	}
	return nil
}

func (s *Store) validateRegistrations(state RegistrationState) error {
	if err := validateRegistrationOwner(state); err != nil {
		return err
	}
	if err := s.validateRegisteredGroups(state); err != nil {
		return err
	}
	if err := validateEnrollmentNonces(state); err != nil {
		return err
	}
	return validateRegistrationIntents(state)
}

func validateRegistrationOwner(state RegistrationState) error {
	if state.OwnerID < 0 {
		return fmt.Errorf("owner ID must not be negative")
	}
	if (state.OwnerClaimNonce == "") != (state.OwnerClaimExpiresAt == 0) {
		return fmt.Errorf("owner claim nonce and expiry must be set together")
	}
	if state.OwnerClaimNonce != "" && state.OwnerClaimExpiresAt <= 0 {
		return fmt.Errorf("owner claim expiry must be positive")
	}
	if state.OwnerID != 0 && state.OwnerClaimNonce != "" {
		return fmt.Errorf("owner claim must be cleared after ownership is claimed")
	}
	return nil
}

func (s *Store) validateRegisteredGroups(state RegistrationState) error {
	seenGroups := make(map[int64]bool, len(s.baseline.Groups)+len(state.RegisteredGroups))
	for _, group := range s.baseline.Groups {
		seenGroups[group.ID] = true
	}
	for _, group := range state.RegisteredGroups {
		if group.ID == 0 {
			return fmt.Errorf("registered group ID 0 is invalid")
		}
		if group.RegisteredBy <= 0 {
			return fmt.Errorf("registered group %d has no valid actor", group.ID)
		}
		if seenGroups[group.ID] {
			return fmt.Errorf("duplicate registered group %d", group.ID)
		}
		seenGroups[group.ID] = true
	}
	return nil
}

func validateEnrollmentNonces(state RegistrationState) error {
	seenNonces := make(map[string]bool, len(state.EnrollmentNonces))
	for _, nonce := range state.EnrollmentNonces {
		if nonce.Nonce == "" || seenNonces[nonce.Nonce] {
			return fmt.Errorf("enrollment nonce is empty or duplicated")
		}
		if nonce.IssuedBy == 0 || nonce.ExpiresAt <= 0 {
			return fmt.Errorf("enrollment nonce issuer and expiry are required")
		}
		seenNonces[nonce.Nonce] = true
	}
	return nil
}

func validateRegistrationIntents(state RegistrationState) error {
	seenPending := make(map[int64]bool, len(state.PendingRegistrations))
	for _, pending := range state.PendingRegistrations {
		if pending.GroupID == 0 || seenPending[pending.GroupID] {
			return fmt.Errorf("pending registration group is zero or duplicated")
		}
		if pending.RegisteredBy == 0 || pending.ExpiresAt <= 0 {
			return fmt.Errorf("pending registration actor and expiry are required")
		}
		seenPending[pending.GroupID] = true
	}
	seenLeaves := make(map[int64]bool, len(state.UnknownGroupLeaves))
	for _, leave := range state.UnknownGroupLeaves {
		if leave.GroupID == 0 || seenLeaves[leave.GroupID] {
			return fmt.Errorf("unknown-group leave group is zero or duplicated")
		}
		if leave.ExpiresAt <= 0 {
			return fmt.Errorf("unknown-group leave %d has no valid expiry", leave.GroupID)
		}
		if seenPending[leave.GroupID] {
			return fmt.Errorf("group %d cannot be pending registration and unknown-group cleanup", leave.GroupID)
		}
		seenLeaves[leave.GroupID] = true
	}
	return nil
}
