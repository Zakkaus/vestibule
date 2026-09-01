package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

// SettingsService is the settings use case required by the HTTP adapter.
type SettingsService interface {
	Settings(int64) (settings.GroupView, bool)
	Update(int64, uint64, settings.GroupOverrides) (settings.CommitResult, error)
}

type settingResponse[T any] struct {
	Value  T      `json:"value"`
	Source string `json:"source"`
}

type settingsResponse struct {
	Revision                uint64                                    `json:"revision"`
	Enabled                 settingResponse[bool]                     `json:"enabled"`
	DeliveryMode            settingResponse[string]                   `json:"delivery_mode"`
	VerifyMode              settingResponse[string]                   `json:"verify_mode"`
	NameSpoiler             settingResponse[bool]                     `json:"name_spoiler"`
	BanSeconds              settingResponse[int]                      `json:"ban_seconds"`
	LookupTTLSeconds        settingResponse[int]                      `json:"lookup_ttl_seconds"`
	LookupAutoDeleteEnabled settingResponse[bool]                     `json:"lookup_auto_delete_enabled"`
	TimeoutSeconds          settingResponse[int]                      `json:"timeout_seconds"`
	VerifyMaxFails          settingResponse[int]                      `json:"verify_max_fails"`
	VerifyRetrySeconds      settingResponse[int]                      `json:"verify_retry_seconds"`
	MuteSeconds             settingResponse[int]                      `json:"mute_seconds"`
	VerifyInvited           settingResponse[bool]                     `json:"verify_invited"`
	WarnLimit               settingResponse[int]                      `json:"warn_limit"`
	AntispamEnabled         settingResponse[bool]                     `json:"antispam_enabled"`
	ChannelWhitelist        settingResponse[[]int64]                  `json:"channel_whitelist"`
	TrustedMemberGroupIDs   settingResponse[[]int64]                  `json:"trusted_member_group_ids"`
	KnownChatIDs            settingResponse[[]int64]                  `json:"known_chat_ids"`
	RequiredChannelID       settingResponse[int64]                    `json:"required_channel_id"`
	ChannelDisplay          settingResponse[string]                   `json:"channel_display"`
	ChannelInviteURL        settingResponse[string]                   `json:"channel_invite_url"`
	Questions               settingResponse[[]settings.Question]      `json:"questions"`
	FallbackQuestions       settingResponse[[]settings.ShortQuestion] `json:"fallback_questions"`
	FallbackBuiltin         settingResponse[bool]                     `json:"fallback_builtin"`
	Lang                    settingResponse[string]                   `json:"lang"`
	RichMessages            settingResponse[bool]                     `json:"rich_messages"`
	PrivateQueryPerMin      settingResponse[int]                      `json:"private_query_per_min"`
	AdminLogChatID          settingResponse[int64]                    `json:"admin_log_chat_id"`
	RequiredChannelFailOpen settingResponse[bool]                     `json:"required_channel_fail_open"`
}

func settingsView(group settings.GroupView) settingsResponse {
	return settingsResponse{
		Revision: group.Revision(),
		Enabled:  settingView(group.Enabled()), DeliveryMode: settingView(group.DeliveryMode()),
		VerifyMode: settingView(group.VerifyMode()), NameSpoiler: settingView(group.NameSpoiler()),
		BanSeconds: settingView(group.BanSeconds()), LookupTTLSeconds: settingView(group.LookupTTLSeconds()),
		LookupAutoDeleteEnabled: settingView(group.LookupAutoDeleteEnabled()),
		TimeoutSeconds:          settingView(group.TimeoutSeconds()), VerifyMaxFails: settingView(group.VerifyMaxFails()),
		VerifyRetrySeconds: settingView(group.VerifyRetrySeconds()), MuteSeconds: settingView(group.MuteSeconds()),
		VerifyInvited: settingView(group.VerifyInvited()), WarnLimit: settingView(group.WarnLimit()),
		AntispamEnabled: settingView(group.AntispamEnabled()), ChannelWhitelist: settingView(group.ChannelWhitelist()),
		TrustedMemberGroupIDs: settingView(group.TrustedMemberGroupIDs()), KnownChatIDs: settingView(group.KnownChatIDs()),
		RequiredChannelID: settingView(group.RequiredChannelID()), ChannelDisplay: settingView(group.ChannelDisplay()),
		ChannelInviteURL: settingView(group.ChannelInviteURL()), Questions: settingView(group.Questions()),
		FallbackQuestions: settingView(group.FallbackQuestions()), FallbackBuiltin: settingView(group.FallbackBuiltin()),
		Lang: settingView(group.Lang()), RichMessages: settingView(group.RichMessages()),
		PrivateQueryPerMin: settingView(group.PrivateQueryPerMin()), AdminLogChatID: settingView(group.AdminLogChatID()),
		RequiredChannelFailOpen: settingView(group.RequiredChannelFailOpen()),
	}
}

func settingView[T any](value settings.Setting[T]) settingResponse[T] {
	return settingResponse[T]{Value: value.Value, Source: value.Source.String()}
}

type settingsPatchRequest struct {
	ExpectedRevision *uint64       `json:"expected_revision"`
	Changes          settingsPatch `json:"changes"`
}

type settingsPatch struct {
	Enabled                 *bool                     `json:"enabled"`
	DeliveryMode            *string                   `json:"delivery_mode"`
	VerifyMode              *string                   `json:"verify_mode"`
	NameSpoiler             *bool                     `json:"name_spoiler"`
	BanSeconds              *int                      `json:"ban_seconds"`
	LookupTTLSeconds        *int                      `json:"lookup_ttl_seconds"`
	LookupAutoDeleteEnabled *bool                     `json:"lookup_auto_delete_enabled"`
	TimeoutSeconds          *int                      `json:"timeout_seconds"`
	VerifyMaxFails          *int                      `json:"verify_max_fails"`
	VerifyRetrySeconds      *int                      `json:"verify_retry_seconds"`
	MuteSeconds             *int                      `json:"mute_seconds"`
	VerifyInvited           *bool                     `json:"verify_invited"`
	WarnLimit               *int                      `json:"warn_limit"`
	AntispamEnabled         *bool                     `json:"antispam_enabled"`
	ChannelWhitelist        *[]int64                  `json:"channel_whitelist"`
	TrustedMemberGroupIDs   *[]int64                  `json:"trusted_member_group_ids"`
	KnownChatIDs            *[]int64                  `json:"known_chat_ids"`
	RequiredChannelID       *int64                    `json:"required_channel_id"`
	ChannelDisplay          *string                   `json:"channel_display"`
	ChannelInviteURL        *string                   `json:"channel_invite_url"`
	Questions               *[]settings.Question      `json:"questions"`
	FallbackQuestions       *[]settings.ShortQuestion `json:"fallback_questions"`
	FallbackBuiltin         *bool                     `json:"fallback_builtin"`
	Lang                    *string                   `json:"lang"`
	RichMessages            *bool                     `json:"rich_messages"`
	PrivateQueryPerMin      *int                      `json:"private_query_per_min"`
	AdminLogChatID          *int64                    `json:"admin_log_chat_id"`
	RequiredChannelFailOpen *bool                     `json:"required_channel_fail_open"`
	present                 map[string]struct{}
}

func (p *settingsPatch) UnmarshalJSON(data []byte) error {
	type wire settingsPatch
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*p = settingsPatch(decoded)
	p.present = make(map[string]struct{}, len(fields))
	for name := range fields {
		p.present[name] = struct{}{}
	}
	return nil
}

func (p settingsPatch) apply(next *settings.GroupOverrides) {
	p.applyModesAndTiming(next)
	p.applyModeration(next)
	p.applyRelations(next)
	p.applyContent(next)
}

func (p settingsPatch) applyModesAndTiming(next *settings.GroupOverrides) {
	if p.has("enabled") {
		next.Enabled = p.Enabled
	}
	if p.has("delivery_mode") {
		next.DeliveryMode = p.DeliveryMode
	}
	if p.has("verify_mode") {
		next.VerifyMode = p.VerifyMode
	}
	if p.has("name_spoiler") {
		next.NameSpoiler = p.NameSpoiler
	}
	if p.has("ban_seconds") {
		next.BanSeconds = p.BanSeconds
	}
	if p.has("lookup_ttl_seconds") {
		next.LookupTTLSeconds = p.LookupTTLSeconds
	}
	if p.has("lookup_auto_delete_enabled") {
		next.LookupAutoDeleteEnabled = p.LookupAutoDeleteEnabled
	}
}

func (p settingsPatch) applyModeration(next *settings.GroupOverrides) {
	if p.has("timeout_seconds") {
		next.TimeoutSeconds = p.TimeoutSeconds
	}
	if p.has("verify_max_fails") {
		next.VerifyMaxFails = p.VerifyMaxFails
	}
	if p.has("verify_retry_seconds") {
		next.VerifyRetrySeconds = p.VerifyRetrySeconds
	}
	if p.has("mute_seconds") {
		next.MuteSeconds = p.MuteSeconds
	}
	if p.has("verify_invited") {
		next.VerifyInvited = p.VerifyInvited
	}
	if p.has("warn_limit") {
		next.WarnLimit = p.WarnLimit
	}
	if p.has("antispam_enabled") {
		next.AntispamEnabled = p.AntispamEnabled
	}
}

func (p settingsPatch) applyRelations(next *settings.GroupOverrides) {
	if p.has("channel_whitelist") {
		next.ChannelWhitelist = p.ChannelWhitelist
	}
	if p.has("trusted_member_group_ids") {
		next.TrustedMemberGroupIDs = p.TrustedMemberGroupIDs
	}
	if p.has("known_chat_ids") {
		next.KnownChatIDs = p.KnownChatIDs
	}
	if p.has("required_channel_id") {
		next.RequiredChannelID = p.RequiredChannelID
	}
	if p.has("channel_display") {
		next.ChannelDisplay = p.ChannelDisplay
	}
	if p.has("channel_invite_url") {
		next.ChannelInviteURL = p.ChannelInviteURL
	}
	if p.has("required_channel_fail_open") {
		next.RequiredChannelFailOpen = p.RequiredChannelFailOpen
	}
}

func (p settingsPatch) applyContent(next *settings.GroupOverrides) {
	if p.has("questions") {
		next.Questions = p.Questions
	}
	if p.has("fallback_questions") {
		next.FallbackQuestions = p.FallbackQuestions
	}
	if p.has("fallback_builtin") {
		next.FallbackBuiltin = p.FallbackBuiltin
	}
	if p.has("lang") {
		next.Lang = p.Lang
	}
	if p.has("rich_messages") {
		next.RichMessages = p.RichMessages
	}
	if p.has("private_query_per_min") {
		next.PrivateQueryPerMin = p.PrivateQueryPerMin
	}
	if p.has("admin_log_chat_id") {
		next.AdminLogChatID = p.AdminLogChatID
	}
}

func (p settingsPatch) has(name string) bool {
	_, ok := p.present[name]
	return ok
}

func (s *Server) settingsRoute(writer http.ResponseWriter, request *http.Request, chatID int64, rest []string) {
	if len(rest) != 0 {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	switch request.Method {
	case http.MethodGet:
		s.readSettings(writer, request, chatID)
	case http.MethodPatch:
		s.patchSettings(writer, request, chatID)
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) readSettings(writer http.ResponseWriter, request *http.Request, chatID int64) {
	if _, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess); !ok {
		return
	}
	group, ok := s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, settingsView(group))
}

func (s *Server) patchSettings(writer http.ResponseWriter, request *http.Request, chatID int64) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	group, ok := s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	var input settingsPatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.ExpectedRevision == nil {
		writeError(writer, http.StatusBadRequest, "invalid_settings")
		return
	}
	next := group.Overrides()
	input.Changes.apply(&next)
	if _, err := s.settings.Update(chatID, *input.ExpectedRevision, next); err != nil {
		writeSettingsError(writer, err)
		return
	}
	group, ok = s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, settingsView(group))
}

func (s *Server) settingsGroup(writer http.ResponseWriter, chatID int64) (settings.GroupView, bool) {
	if s.settings == nil {
		writeError(writer, http.StatusServiceUnavailable, "settings_unavailable")
		return settings.GroupView{}, false
	}
	group, ok := s.settings.Settings(chatID)
	if !ok {
		writeError(writer, http.StatusNotFound, "chat_not_found")
		return settings.GroupView{}, false
	}
	return group, true
}

func writeSettingsError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, settings.ErrSettingsConflict):
		writeError(writer, http.StatusConflict, "settings_conflict")
	case errors.Is(err, settings.ErrUnknownGroup):
		writeError(writer, http.StatusNotFound, "chat_not_found")
	default:
		writeError(writer, http.StatusServiceUnavailable, "settings_unavailable")
	}
}
