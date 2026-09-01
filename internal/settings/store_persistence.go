package settings

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"reflect"

	statefile "github.com/Zakkaus/vestibule/internal/store"
)

func (s *Store) writeState(state *settingsFile) error {
	if s.path == "" {
		return nil
	}
	candidate := cloneSettingsFile(*state)
	if _, err := s.buildSnapshot(candidate); err != nil {
		return err
	}
	candidate.Version = SettingsSchemaVersion
	if err := statefile.Write(s.path, candidate); err != nil {
		return err
	}
	state.Version = candidate.Version
	return nil
}

func (s *Store) unavailableError() error {
	if current := s.lastError.Load(); current != nil {
		return fmt.Errorf("%w: %v", ErrSettingsUnavailable, current.err)
	}
	return ErrSettingsUnavailable
}

func (s *Store) setLastError(err error) {
	if err == nil {
		s.lastError.Store(nil)
		return
	}
	s.lastError.Store(&statusError{err: err})
}

func normalizeFile(state settingsFile) settingsFile {
	state.RegisteredGroups = nonNilRegisteredGroups(state.RegisteredGroups)
	state.EnrollmentNonces = nonNilEnrollmentNonces(state.EnrollmentNonces)
	state.PendingRegistrations = nonNilPendingRegistrations(state.PendingRegistrations)
	state.UnknownGroupLeaves = nonNilUnknownGroupLeaves(state.UnknownGroupLeaves)
	if state.Groups == nil {
		state.Groups = make(map[int64]groupRecord)
	}
	return state
}

func normalizeRegistrationState(state RegistrationState) RegistrationState {
	state.RegisteredGroups = nonNilRegisteredGroups(state.RegisteredGroups)
	state.EnrollmentNonces = nonNilEnrollmentNonces(state.EnrollmentNonces)
	state.PendingRegistrations = nonNilPendingRegistrations(state.PendingRegistrations)
	state.UnknownGroupLeaves = nonNilUnknownGroupLeaves(state.UnknownGroupLeaves)
	return state
}

func nonNilRegisteredGroups(values []RegisteredGroup) []RegisteredGroup {
	if values == nil {
		return []RegisteredGroup{}
	}
	return append([]RegisteredGroup(nil), values...)
}

func nonNilEnrollmentNonces(values []EnrollmentNonce) []EnrollmentNonce {
	if values == nil {
		return []EnrollmentNonce{}
	}
	return append([]EnrollmentNonce(nil), values...)
}

func nonNilPendingRegistrations(values []PendingRegistration) []PendingRegistration {
	if values == nil {
		return []PendingRegistration{}
	}
	return append([]PendingRegistration(nil), values...)
}

func nonNilUnknownGroupLeaves(values []UnknownGroupLeave) []UnknownGroupLeave {
	if values == nil {
		return []UnknownGroupLeave{}
	}
	return append([]UnknownGroupLeave(nil), values...)
}

func cloneSettingsBaseline(value SettingsBaseline) SettingsBaseline {
	out := value
	out.Groups = make([]GroupBaseline, len(value.Groups))
	for i := range value.Groups {
		out.Groups[i] = cloneGroupBaseline(value.Groups[i])
	}
	out.Factory = cloneGroupBaseline(value.Factory)
	return out
}

func cloneGroupBaseline(value GroupBaseline) GroupBaseline {
	out := value
	out.ChannelWhitelist.Value = cloneInt64s(value.ChannelWhitelist.Value)
	out.TrustedMemberGroupIDs.Value = cloneInt64s(value.TrustedMemberGroupIDs.Value)
	out.KnownChatIDs.Value = cloneInt64s(value.KnownChatIDs.Value)
	out.Questions.Value = cloneQuestions(value.Questions.Value)
	out.FallbackQuestions.Value = cloneShortQuestions(value.FallbackQuestions.Value)
	return out
}

func cloneSettingsFile(value settingsFile) settingsFile {
	out := value
	out.RegisteredGroups = nonNilRegisteredGroups(value.RegisteredGroups)
	out.EnrollmentNonces = nonNilEnrollmentNonces(value.EnrollmentNonces)
	out.PendingRegistrations = nonNilPendingRegistrations(value.PendingRegistrations)
	out.UnknownGroupLeaves = nonNilUnknownGroupLeaves(value.UnknownGroupLeaves)
	out.Groups = make(map[int64]groupRecord, len(value.Groups))
	for id, record := range value.Groups {
		record.GroupOverrides = cloneGroupOverrides(record.GroupOverrides)
		out.Groups[id] = record
	}
	return out
}

func cloneRegistrationState(value RegistrationState) RegistrationState {
	out := value
	out.RegisteredGroups = nonNilRegisteredGroups(value.RegisteredGroups)
	out.EnrollmentNonces = nonNilEnrollmentNonces(value.EnrollmentNonces)
	out.PendingRegistrations = nonNilPendingRegistrations(value.PendingRegistrations)
	out.UnknownGroupLeaves = nonNilUnknownGroupLeaves(value.UnknownGroupLeaves)
	return out
}

func cloneGroupOverrides(value GroupOverrides) GroupOverrides {
	out := value
	out.Enabled = clonePtr(value.Enabled)
	out.DeliveryMode = clonePtr(value.DeliveryMode)
	out.VerifyMode = clonePtr(value.VerifyMode)
	out.NameSpoiler = clonePtr(value.NameSpoiler)
	out.BanSeconds = clonePtr(value.BanSeconds)
	out.MuteSeconds = clonePtr(value.MuteSeconds)
	out.VerifyInvited = clonePtr(value.VerifyInvited)
	out.WarnLimit = clonePtr(value.WarnLimit)
	out.LookupTTLSeconds = clonePtr(value.LookupTTLSeconds)
	out.LookupAutoDeleteEnabled = clonePtr(value.LookupAutoDeleteEnabled)
	out.TimeoutSeconds = clonePtr(value.TimeoutSeconds)
	out.VerifyMaxFails = clonePtr(value.VerifyMaxFails)
	out.VerifyRetrySeconds = clonePtr(value.VerifyRetrySeconds)
	out.AntispamEnabled = clonePtr(value.AntispamEnabled)
	out.ChannelWhitelist = cloneSlicePtr(value.ChannelWhitelist, cloneInt64s)
	out.TrustedMemberGroupIDs = cloneSlicePtr(value.TrustedMemberGroupIDs, cloneInt64s)
	out.KnownChatIDs = cloneSlicePtr(value.KnownChatIDs, cloneInt64s)
	out.RequiredChannelID = clonePtr(value.RequiredChannelID)
	out.ChannelDisplay = clonePtr(value.ChannelDisplay)
	out.ChannelInviteURL = clonePtr(value.ChannelInviteURL)
	out.Questions = cloneSlicePtr(value.Questions, cloneQuestions)
	out.FallbackQuestions = cloneSlicePtr(value.FallbackQuestions, cloneShortQuestions)
	out.FallbackBuiltin = clonePtr(value.FallbackBuiltin)
	out.Lang = clonePtr(value.Lang)
	out.RichMessages = clonePtr(value.RichMessages)
	out.PrivateQueryPerMin = clonePtr(value.PrivateQueryPerMin)
	out.AdminLogChatID = clonePtr(value.AdminLogChatID)
	out.RequiredChannelFailOpen = clonePtr(value.RequiredChannelFailOpen)
	return out
}

func compactGroupOverrides(value GroupOverrides, baseline GroupBaseline) GroupOverrides {
	value.Enabled = omitBaseline(value.Enabled, baseline.Enabled.Value)
	value.DeliveryMode = omitBaseline(value.DeliveryMode, baseline.DeliveryMode.Value)
	value.VerifyMode = omitBaseline(value.VerifyMode, baseline.VerifyMode.Value)
	value.NameSpoiler = omitBaseline(value.NameSpoiler, baseline.NameSpoiler.Value)
	value.BanSeconds = omitBaseline(value.BanSeconds, baseline.BanSeconds.Value)
	value.MuteSeconds = omitBaseline(value.MuteSeconds, baseline.MuteSeconds.Value)
	value.VerifyInvited = omitBaseline(value.VerifyInvited, baseline.VerifyInvited.Value)
	value.WarnLimit = omitBaseline(value.WarnLimit, baseline.WarnLimit.Value)
	value.LookupTTLSeconds = omitBaseline(value.LookupTTLSeconds, baseline.LookupTTLSeconds.Value)
	value.LookupAutoDeleteEnabled = omitBaseline(value.LookupAutoDeleteEnabled, baseline.LookupAutoDeleteEnabled.Value)
	value.TimeoutSeconds = omitBaseline(value.TimeoutSeconds, baseline.TimeoutSeconds.Value)
	value.VerifyMaxFails = omitBaseline(value.VerifyMaxFails, baseline.VerifyMaxFails.Value)
	value.VerifyRetrySeconds = omitBaseline(value.VerifyRetrySeconds, baseline.VerifyRetrySeconds.Value)
	value.AntispamEnabled = omitBaseline(value.AntispamEnabled, baseline.AntispamEnabled.Value)
	value.ChannelWhitelist = omitBaseline(value.ChannelWhitelist, baseline.ChannelWhitelist.Value)
	value.TrustedMemberGroupIDs = omitBaseline(value.TrustedMemberGroupIDs, baseline.TrustedMemberGroupIDs.Value)
	value.KnownChatIDs = omitBaseline(value.KnownChatIDs, baseline.KnownChatIDs.Value)
	value.RequiredChannelID = omitBaseline(value.RequiredChannelID, baseline.RequiredChannelID.Value)
	value.ChannelDisplay = omitBaseline(value.ChannelDisplay, baseline.ChannelDisplay.Value)
	value.ChannelInviteURL = omitBaseline(value.ChannelInviteURL, baseline.ChannelInviteURL.Value)
	value.Questions = omitBaseline(value.Questions, baseline.Questions.Value)
	value.FallbackQuestions = omitBaseline(value.FallbackQuestions, baseline.FallbackQuestions.Value)
	value.FallbackBuiltin = omitBaseline(value.FallbackBuiltin, baseline.FallbackBuiltin.Value)
	value.Lang = omitBaseline(value.Lang, baseline.Lang.Value)
	value.RichMessages = omitBaseline(value.RichMessages, baseline.RichMessages.Value)
	value.PrivateQueryPerMin = omitBaseline(value.PrivateQueryPerMin, baseline.PrivateQueryPerMin.Value)
	value.AdminLogChatID = omitBaseline(value.AdminLogChatID, baseline.AdminLogChatID.Value)
	value.RequiredChannelFailOpen = omitBaseline(value.RequiredChannelFailOpen, baseline.RequiredChannelFailOpen.Value)
	return value
}

func omitBaseline[T any](value *T, baseline T) *T {
	if value != nil && reflect.DeepEqual(*value, baseline) {
		return nil
	}
	return value
}

func clonePtr[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneSlicePtr[T any](value *[]T, clone func([]T) []T) *[]T {
	if value == nil {
		return nil
	}
	cloned := clone(*value)
	return &cloned
}

func cloneInt64s(values []int64) []int64 {
	if values == nil {
		return []int64{}
	}
	out := make([]int64, len(values))
	copy(out, values)
	return out
}

func cloneQuestions(values []Question) []Question {
	if values == nil {
		return []Question{}
	}
	out := make([]Question, len(values))
	for i := range values {
		out[i] = values[i]
		out[i].Options = append([]string(nil), values[i].Options...)
	}
	return out
}

func cloneShortQuestions(values []ShortQuestion) []ShortQuestion {
	if values == nil {
		return []ShortQuestion{}
	}
	out := make([]ShortQuestion, len(values))
	for i := range values {
		out[i] = values[i]
		out[i].Answers = append([]string(nil), values[i].Answers...)
	}
	return out
}

func randomRegistrationNonce() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate registration nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
