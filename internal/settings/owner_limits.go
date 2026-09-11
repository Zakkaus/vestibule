package settings

import (
	"errors"
	"fmt"
	"sort"
)

// OwnerLimits contains the instance-wide caps applied to effective group settings.
type OwnerLimits struct {
	TimeoutSeconds        int64 `json:"timeout_seconds"`
	BanSeconds            int64 `json:"ban_seconds"`
	MuteSeconds           int64 `json:"mute_seconds"`
	LookupTTLSeconds      int64 `json:"lookup_ttl_seconds"`
	VerifyRetrySeconds    int64 `json:"verify_retry_seconds"`
	VerifyMaxFails        int64 `json:"verify_max_fails"`
	WarnLimit             int64 `json:"warn_limit"`
	PrivateQueryPerMin    int64 `json:"private_query_per_min"`
	Questions             int64 `json:"questions"`
	FallbackQuestions     int64 `json:"fallback_questions"`
	ChannelWhitelist      int64 `json:"channel_whitelist"`
	TrustedMemberGroupIDs int64 `json:"trusted_member_group_ids"`
	KnownChatIDs          int64 `json:"known_chat_ids"`
}

// OwnerLimitsState is the immutable owner-limit snapshot exposed to adapters.
type OwnerLimitsState struct {
	Revision   uint64
	Limits     OwnerLimits
	Violations []LimitViolation
}

// LimitChanges is a sparse owner-limit update. A nil value restores the zero cap.
type LimitChanges map[string]*int64

// LimitViolation identifies one effective group setting above its instance cap.
type LimitViolation struct {
	ChatID int64  `json:"chat_id"`
	Field  string `json:"field"`
	Value  int64  `json:"value"`
	Limit  int64  `json:"limit"`
}

var (
	ErrOwnerLimitsInvalid  = errors.New("invalid owner limits")
	ErrOwnerLimitsExceeded = errors.New("settings limit exceeded")
)

type OwnerLimitsConflictError struct {
	Expected uint64
	Actual   uint64
}

func (e *OwnerLimitsConflictError) Error() string {
	return fmt.Sprintf("%s: owner limits expected revision %d, current revision %d", ErrSettingsConflict, e.Expected, e.Actual)
}

func (e *OwnerLimitsConflictError) Is(target error) bool { return target == ErrSettingsConflict }

type OwnerLimitsValidationError struct {
	Field  string
	Value  int64
	Reason string
}

func (e *OwnerLimitsValidationError) Error() string {
	return fmt.Sprintf("%s: %s=%d %s", ErrOwnerLimitsInvalid, e.Field, e.Value, e.Reason)
}

func (e *OwnerLimitsValidationError) Is(target error) bool { return target == ErrOwnerLimitsInvalid }

type OwnerLimitsExceededError struct {
	Violations []LimitViolation
}

func (e *OwnerLimitsExceededError) Error() string        { return ErrOwnerLimitsExceeded.Error() }
func (e *OwnerLimitsExceededError) Is(target error) bool { return target == ErrOwnerLimitsExceeded }

var ownerLimitFields = [...]string{
	"timeout_seconds",
	"ban_seconds",
	"mute_seconds",
	"lookup_ttl_seconds",
	"verify_retry_seconds",
	"verify_max_fails",
	"warn_limit",
	"private_query_per_min",
	"questions",
	"fallback_questions",
	"channel_whitelist",
	"trusted_member_group_ids",
	"known_chat_ids",
}

type ownerLimitRule struct {
	min int64
	max int64
}

var ownerLimitRules = map[string]ownerLimitRule{
	"timeout_seconds":          {min: 30, max: 1800},
	"ban_seconds":              {min: 30, max: 31622400},
	"mute_seconds":             {min: 30, max: 31622400},
	"lookup_ttl_seconds":       {min: 1, max: 86400},
	"verify_retry_seconds":     {min: 1, max: 31622400},
	"verify_max_fails":         {min: 1, max: 2147483647},
	"warn_limit":               {min: 1, max: 2147483647},
	"private_query_per_min":    {min: 1, max: 2147483647},
	"questions":                {min: 1, max: 2147483647},
	"fallback_questions":       {min: 1, max: 2147483647},
	"channel_whitelist":        {min: 1, max: 2147483647},
	"trusted_member_group_ids": {min: 1, max: 2147483647},
	"known_chat_ids":           {min: 1, max: 2147483647},
}

func (l OwnerLimits) Value(field string) (int64, bool) {
	switch field {
	case "timeout_seconds":
		return l.TimeoutSeconds, true
	case "ban_seconds":
		return l.BanSeconds, true
	case "mute_seconds":
		return l.MuteSeconds, true
	case "lookup_ttl_seconds":
		return l.LookupTTLSeconds, true
	case "verify_retry_seconds":
		return l.VerifyRetrySeconds, true
	case "verify_max_fails":
		return l.VerifyMaxFails, true
	case "warn_limit":
		return l.WarnLimit, true
	case "private_query_per_min":
		return l.PrivateQueryPerMin, true
	case "questions":
		return l.Questions, true
	case "fallback_questions":
		return l.FallbackQuestions, true
	case "channel_whitelist":
		return l.ChannelWhitelist, true
	case "trusted_member_group_ids":
		return l.TrustedMemberGroupIDs, true
	case "known_chat_ids":
		return l.KnownChatIDs, true
	default:
		return 0, false
	}
}

func (l *OwnerLimits) setValue(field string, value int64) bool {
	switch field {
	case "timeout_seconds":
		l.TimeoutSeconds = value
	case "ban_seconds":
		l.BanSeconds = value
	case "mute_seconds":
		l.MuteSeconds = value
	case "lookup_ttl_seconds":
		l.LookupTTLSeconds = value
	case "verify_retry_seconds":
		l.VerifyRetrySeconds = value
	case "verify_max_fails":
		l.VerifyMaxFails = value
	case "warn_limit":
		l.WarnLimit = value
	case "private_query_per_min":
		l.PrivateQueryPerMin = value
	case "questions":
		l.Questions = value
	case "fallback_questions":
		l.FallbackQuestions = value
	case "channel_whitelist":
		l.ChannelWhitelist = value
	case "trusted_member_group_ids":
		l.TrustedMemberGroupIDs = value
	case "known_chat_ids":
		l.KnownChatIDs = value
	default:
		return false
	}
	return true
}

func validateOwnerLimit(field string, value int64) error {
	rule, ok := ownerLimitRules[field]
	if !ok {
		return &OwnerLimitsValidationError{Field: field, Value: value, Reason: "unknown field"}
	}
	if value < 0 {
		return &OwnerLimitsValidationError{Field: field, Value: value, Reason: "must not be negative"}
	}
	if value != 0 && (value < rule.min || value > rule.max) {
		return &OwnerLimitsValidationError{Field: field, Value: value, Reason: fmt.Sprintf("must be 0 or between %d and %d", rule.min, rule.max)}
	}
	return nil
}

func validateOwnerLimits(limits OwnerLimits) error {
	for _, field := range ownerLimitFields {
		value, _ := limits.Value(field)
		if err := validateOwnerLimit(field, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) OwnerLimits() OwnerLimitsState {
	snapshot := s.snapshot.Load()
	return ownerLimitsState(snapshot)
}

func ownerLimitsState(snapshot *settingsSnapshot) OwnerLimitsState {
	if snapshot == nil {
		return OwnerLimitsState{}
	}
	return OwnerLimitsState{
		Revision:   snapshot.limitsRevision,
		Limits:     snapshot.limits,
		Violations: ownerLimitViolations(snapshot),
	}
}

func (s *Store) UpdateOwnerLimits(expectedRevision uint64, changes LimitChanges) (OwnerLimitsState, error) {
	s.writer.Lock()
	defer s.writer.Unlock()
	if s.path == "" || !s.writable {
		if !s.writable {
			return OwnerLimitsState{}, s.unavailableError()
		}
		return OwnerLimitsState{}, ErrSettingsNotDurable
	}
	current := s.snapshot.Load()
	if current.limitsRevision != expectedRevision {
		return OwnerLimitsState{}, &OwnerLimitsConflictError{Expected: expectedRevision, Actual: current.limitsRevision}
	}
	next := current.limits
	for field, pointer := range changes {
		value := int64(0)
		if pointer != nil {
			value = *pointer
		}
		if !next.setValue(field, value) {
			return OwnerLimitsState{}, &OwnerLimitsValidationError{Field: field, Value: value, Reason: "unknown field"}
		}
		if err := validateOwnerLimit(field, value); err != nil {
			return OwnerLimitsState{}, err
		}
	}
	if next == current.limits {
		return ownerLimitsState(current), nil
	}
	candidate := cloneSettingsFile(s.state)
	candidate.Limits = next
	candidate.LimitsRevision = current.limitsRevision + 1
	snap, err := s.buildSnapshot(candidate)
	if err != nil {
		return OwnerLimitsState{}, err
	}
	if err := s.writeState(&candidate); err != nil {
		s.setLastError(err)
		return OwnerLimitsState{}, err
	}
	s.state = candidate
	s.snapshot.Store(snap)
	s.setLastError(nil)
	return ownerLimitsState(snap), nil
}

func ownerLimitViolations(snapshot *settingsSnapshot) []LimitViolation {
	if snapshot == nil {
		return nil
	}
	groupIDs := append([]int64(nil), snapshot.groupIDs...)
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	violations := make([]LimitViolation, 0)
	for _, groupID := range groupIDs {
		violations = append(violations, ownerLimitViolationsForGroup(snapshot.groups[groupID], snapshot.limits)...)
	}
	return violations
}

func effectiveLimitValues(group *effectiveGroup) [13]int64 {
	return [13]int64{
		int64(group.timeoutSeconds.Value),
		int64(group.banSeconds.Value),
		int64(group.muteSeconds.Value),
		int64(group.lookupTTLSeconds.Value),
		int64(group.verifyRetrySeconds.Value),
		int64(group.verifyMaxFails.Value),
		int64(group.warnLimit.Value),
		int64(group.privateQueryPerMin.Value),
		int64(len(group.questions.Value)),
		int64(len(group.fallbackQuestions.Value)),
		int64(len(group.channelWhitelist.Value)),
		int64(len(group.trustedMemberGroupIDs.Value)),
		int64(len(group.knownChatIDs.Value)),
	}
}

func exceedsOwnerLimit(field string, value, limit int64) bool {
	if field == "ban_seconds" && value == 0 {
		return true
	}
	if (field == "verify_retry_seconds" || field == "verify_max_fails") && value < 0 {
		return false
	}
	return value > limit
}

func ownerLimitViolationsForGroup(group *effectiveGroup, limits OwnerLimits) []LimitViolation {
	values := effectiveLimitValues(group)
	violations := make([]LimitViolation, 0)
	for index, field := range ownerLimitFields {
		limit, _ := limits.Value(field)
		if limit != 0 && exceedsOwnerLimit(field, values[index], limit) {
			violations = append(violations, LimitViolation{ChatID: group.id, Field: field, Value: values[index], Limit: limit})
		}
	}
	return violations
}
