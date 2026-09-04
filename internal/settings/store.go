package settings

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

// SettingsSchemaVersion is the settings.json schema written by this package.
const SettingsSchemaVersion = 3

// Source identifies where an effective setting came from.
type Source uint8

const (
	// SourceFactory is a value embedded in defaults.yaml.
	SourceFactory Source = iota
	// SourceUserFile is a per-chat value managed by the user configuration file.
	SourceUserFile
	// SourceChatOverride is a sparse chat.settings override.
	SourceChatOverride
)

func (s Source) String() string {
	switch s {
	case SourceFactory:
		return "factory default"
	case SourceUserFile:
		return "user file"
	case SourceChatOverride:
		return "chat override"
	default:
		return "unknown"
	}
}

// Setting pairs an effective value with its provenance.
type Setting[T any] struct {
	Value  T
	Source Source
}

// BaselineValue is one immutable config-or-default input to Store.
type BaselineValue[T any] struct {
	Value  T
	Source Source
}

// GroupBaseline contains the configured/default values for one group.
type GroupBaseline struct {
	ID                      int64
	Enabled                 BaselineValue[bool]
	DeliveryMode            BaselineValue[string]
	VerifyMode              BaselineValue[string]
	NameSpoiler             BaselineValue[bool]
	BanSeconds              BaselineValue[int]
	LookupTTLSeconds        BaselineValue[int]
	LookupAutoDeleteEnabled BaselineValue[bool]
	TimeoutSeconds          BaselineValue[int]
	VerifyMaxFails          BaselineValue[int]
	VerifyRetrySeconds      BaselineValue[int]
	MuteSeconds             BaselineValue[int]
	VerifyInvited           BaselineValue[bool]
	WarnLimit               BaselineValue[int]
	AntispamEnabled         BaselineValue[bool]
	ChannelWhitelist        BaselineValue[[]int64]
	TrustedMemberGroupIDs   BaselineValue[[]int64]
	KnownChatIDs            BaselineValue[[]int64]
	RequiredChannelID       BaselineValue[int64]
	ChannelDisplay          BaselineValue[string]
	ChannelInviteURL        BaselineValue[string]
	Questions               BaselineValue[[]Question]
	FallbackQuestions       BaselineValue[[]ShortQuestion]
	FallbackBuiltin         BaselineValue[bool]
	Lang                    BaselineValue[string]
	RichMessages            BaselineValue[bool]
	PrivateQueryPerMin      BaselineValue[int]
	AdminLogChatID          BaselineValue[int64]
	RequiredChannelFailOpen BaselineValue[bool]
}

// SettingsBaseline combines immutable factory defaults with per-chat user-file values.
type SettingsBaseline struct {
	Groups  []GroupBaseline
	Factory GroupBaseline
}

// GroupOverrides is the sparse per-group settings.json record. A nil field follows the baseline.
type GroupOverrides struct {
	Enabled                 *bool            `json:"enabled,omitempty"`
	DeliveryMode            *string          `json:"delivery_mode,omitempty"`
	VerifyMode              *string          `json:"verify_mode,omitempty"`
	NameSpoiler             *bool            `json:"name_spoiler,omitempty"`
	BanSeconds              *int             `json:"ban_seconds,omitempty"`
	LookupTTLSeconds        *int             `json:"lookup_ttl_seconds,omitempty"`
	LookupAutoDeleteEnabled *bool            `json:"lookup_auto_delete_enabled,omitempty"`
	TimeoutSeconds          *int             `json:"timeout_seconds,omitempty"`
	VerifyMaxFails          *int             `json:"verify_max_fails,omitempty"`
	VerifyRetrySeconds      *int             `json:"verify_retry_seconds,omitempty"`
	MuteSeconds             *int             `json:"mute_seconds,omitempty"`
	VerifyInvited           *bool            `json:"verify_invited,omitempty"`
	WarnLimit               *int             `json:"warn_limit,omitempty"`
	AntispamEnabled         *bool            `json:"antispam_enabled,omitempty"`
	ChannelWhitelist        *[]int64         `json:"channel_whitelist,omitempty"`
	TrustedMemberGroupIDs   *[]int64         `json:"trusted_member_group_ids,omitempty"`
	KnownChatIDs            *[]int64         `json:"known_chat_ids,omitempty"`
	RequiredChannelID       *int64           `json:"required_channel_id,omitempty"`
	ChannelDisplay          *string          `json:"channel_display,omitempty"`
	ChannelInviteURL        *string          `json:"channel_invite_url,omitempty"`
	Questions               *[]Question      `json:"questions,omitempty"`
	FallbackQuestions       *[]ShortQuestion `json:"fallback_questions,omitempty"`
	FallbackBuiltin         *bool            `json:"fallback_builtin,omitempty"`
	Lang                    *string          `json:"lang,omitempty"`
	RichMessages            *bool            `json:"rich_messages,omitempty"`
	PrivateQueryPerMin      *int             `json:"private_query_per_min,omitempty"`
	AdminLogChatID          *int64           `json:"admin_log_chat_id,omitempty"`
	RequiredChannelFailOpen *bool            `json:"required_channel_fail_open,omitempty"`
}

// RegisteredGroup records an owner-authorized runtime group.
type RegisteredGroup struct {
	ID           int64  `json:"id"`
	RegisteredBy int64  `json:"registered_by"`
	Title        string `json:"title,omitempty"`
}

// EnrollmentNonce is one owner-issued, expiring registration capability.
type EnrollmentNonce struct {
	Nonce     string `json:"nonce"`
	IssuedBy  int64  `json:"issued_by"`
	ExpiresAt int64  `json:"expires_at"`
}

// PendingRegistration retains an authorized add-then-promote registration.
type PendingRegistration struct {
	GroupID      int64  `json:"group_id"`
	RegisteredBy int64  `json:"registered_by"`
	Title        string `json:"title,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
}

// UnknownGroupLeave retains cleanup for a group without registration authorization.
type UnknownGroupLeave struct {
	GroupID   int64  `json:"group_id"`
	Title     string `json:"title,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
}

// RegistrationState is the durable owner and runtime-group metadata transaction.
type RegistrationState struct {
	Revision             uint64
	OwnerID              int64
	OwnerClaimNonce      string
	OwnerClaimExpiresAt  int64
	RegisteredGroups     []RegisteredGroup
	EnrollmentNonces     []EnrollmentNonce
	PendingRegistrations []PendingRegistration
	UnknownGroupLeaves   []UnknownGroupLeave
}

// PersistenceStatus describes whether setting commits are durable and currently allowed.
type PersistenceStatus struct {
	Configured bool
	Durable    bool
	Writable   bool
	LastError  error
}

// CommitResult reports the new revision and whether the commit reached durable storage.
type CommitResult struct {
	Revision uint64
	Durable  bool
}

// Record is one database-backed sparse per-chat settings row.
type Record struct {
	ChatID    int64
	Revision  uint64
	Overrides GroupOverrides
}

// Repository is the persistence boundary required by the settings store.
type Repository interface {
	LoadSettings() ([]Record, error)
	SeedSettings([]Record) error
	CompareAndSwapSettings(chatID int64, expectedRevision uint64, next GroupOverrides) (actualRevision uint64, written bool, err error)
}

// ErrSettingsConflict identifies an optimistic-concurrency failure.
var ErrSettingsConflict = errors.New("settings revision conflict")

// ErrSettingsUnavailable identifies a state file that must not be overwritten.
var ErrSettingsUnavailable = errors.New("settings writes unavailable")

// ErrSettingsNotDurable identifies an operation, such as registration, that cannot be runtime-only.
var ErrSettingsNotDurable = errors.New("settings persistence is not durable")

// ErrUnknownGroup identifies a group outside the configured and registered effective list.
var ErrUnknownGroup = errors.New("unknown settings group")

// ErrOwnerClaimInvalid identifies a claim token that is absent, used, mismatched, or expired.
var ErrOwnerClaimInvalid = errors.New("owner claim is invalid")

// ErrRegistrationOwnerOnly identifies an enrollment request made by a non-owner.
var ErrRegistrationOwnerOnly = errors.New("registration owner authorization required")

// ConflictError carries the expected and current revision for a refused write.
type ConflictError struct {
	GroupID  int64
	Expected uint64
	Actual   uint64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s: chat %d: expected revision %d, current revision %d", ErrSettingsConflict, e.GroupID, e.Expected, e.Actual)
}

func (e *ConflictError) Is(target error) bool { return target == ErrSettingsConflict }

type groupRecord struct {
	Revision uint64 `json:"revision"`
	GroupOverrides
}

type legacyAntispamState struct {
	Enabled   bool    `json:"enabled"`
	Whitelist []int64 `json:"whitelist"`
}

type settingsFile struct {
	Version              int                   `json:"version"`
	RegistrationRevision uint64                `json:"registration_revision"`
	OwnerID              int64                 `json:"owner_id"`
	OwnerClaimNonce      string                `json:"owner_claim_nonce"`
	OwnerClaimExpiresAt  int64                 `json:"owner_claim_expires_at"`
	RegisteredGroups     []RegisteredGroup     `json:"registered_groups"`
	EnrollmentNonces     []EnrollmentNonce     `json:"enrollment_nonces"`
	PendingRegistrations []PendingRegistration `json:"pending_registrations"`
	UnknownGroupLeaves   []UnknownGroupLeave   `json:"unknown_group_leaves,omitempty"`
	Groups               map[int64]groupRecord `json:"groups"`
}

type effectiveGroup struct {
	id                      int64
	revision                uint64
	registered              bool
	baseline                GroupBaseline
	overrides               GroupOverrides
	enabled                 Setting[bool]
	deliveryMode            Setting[string]
	verifyMode              Setting[string]
	nameSpoiler             Setting[bool]
	banSeconds              Setting[int]
	lookupTTLSeconds        Setting[int]
	lookupAutoDeleteEnabled Setting[bool]
	timeoutSeconds          Setting[int]
	verifyMaxFails          Setting[int]
	verifyRetrySeconds      Setting[int]
	muteSeconds             Setting[int]
	verifyInvited           Setting[bool]
	warnLimit               Setting[int]
	antispamEnabled         Setting[bool]
	channelWhitelist        Setting[[]int64]
	trustedMemberGroupIDs   Setting[[]int64]
	knownChatIDs            Setting[[]int64]
	requiredChannelID       Setting[int64]
	channelDisplay          Setting[string]
	channelInviteURL        Setting[string]
	questions               Setting[[]Question]
	fallbackQuestions       Setting[[]ShortQuestion]
	fallbackBuiltin         Setting[bool]
	lang                    Setting[string]
	richMessages            Setting[bool]
	privateQueryPerMin      Setting[int]
	adminLogChatID          Setting[int64]
	requiredChannelFailOpen Setting[bool]
}

type settingsSnapshot struct {
	groups       map[int64]*effectiveGroup
	groupIDs     []int64
	registration RegistrationState
}

type statusError struct{ err error }

// Store owns the one immutable runtime-settings snapshot and its serialized commit path.
type Store struct {
	path         string
	repository   Repository
	baseline     SettingsBaseline
	baselineByID map[int64]GroupBaseline
	writer       sync.Mutex
	state        settingsFile
	writable     bool
	snapshot     atomic.Pointer[settingsSnapshot]
	lastError    atomic.Pointer[statusError]
}

// GroupView is a read-only, allocation-free handle into one immutable snapshot.
type GroupView struct{ group *effectiveGroup }

// reconcileWithBaseline resolves the two ways config.json and settings.json routinely drift
// apart, and reports what it changed so the operator can be told. Per-group overrides are never
// touched: a group promoted into config.json keeps the settings its administrators chose.
func (s *Store) reconcileWithBaseline(state settingsFile) (settingsFile, []string) {
	out := cloneSettingsFile(state)
	var adjustments []string

	kept := out.RegisteredGroups[:0]
	for _, group := range out.RegisteredGroups {
		if _, configured := s.baselineByID[group.ID]; configured {
			adjustments = append(adjustments, fmt.Sprintf("group %d is now configured, dropping its runtime registration", group.ID))
			continue
		}
		kept = append(kept, group)
	}
	out.RegisteredGroups = kept

	return out, adjustments
}

// Persistence returns a lock-free persistence status snapshot.
func (s *Store) Persistence() PersistenceStatus {
	status := PersistenceStatus{
		Configured: s.repository != nil || s.path != "",
		Durable:    (s.repository != nil || s.path != "") && s.writable,
		Writable:   s.writable,
	}
	if current := s.lastError.Load(); current != nil {
		status.LastError = current.err
	}
	return status
}

// Settings returns one immutable effective chat view.
func (s *Store) Settings(chatID int64) (GroupView, bool) {
	group, ok := s.snapshot.Load().groups[chatID]
	return GroupView{group: group}, ok
}

// IsGroup reports whether an ID is configured or durably registered.
func (s *Store) IsGroup(groupID int64) bool {
	_, ok := s.snapshot.Load().groups[groupID]
	return ok
}

// IsKnownChat reports whether an ID is an effective group, required channel, trusted group, or allowlisted support chat.
func (s *Store) IsKnownChat(chatID int64) bool {
	if chatID == 0 {
		return false
	}
	snapshot := s.snapshot.Load()
	if _, ok := snapshot.groups[chatID]; ok {
		return true
	}
	for _, groupID := range snapshot.groupIDs {
		group := snapshot.groups[groupID]
		if group.requiredChannelID.Value == chatID || group.adminLogChatID.Value == chatID {
			return true
		}
		for _, knownID := range group.knownChatIDs.Value {
			if knownID == chatID {
				return true
			}
		}
		for _, trustedID := range group.trustedMemberGroupIDs.Value {
			if trustedID == chatID {
				return true
			}
		}
	}
	return false
}

// ChatIDs returns the effective configured-then-registered chat order.
func (s *Store) ChatIDs() []int64 {
	return append([]int64(nil), s.snapshot.Load().groupIDs...)
}

// RegisteredGroupTitle returns the durable title captured for a runtime registration.
func (s *Store) RegisteredGroupTitle(chatID int64) (string, bool) {
	for _, group := range s.snapshot.Load().registration.RegisteredGroups {
		if group.ID == chatID && group.Title != "" {
			return group.Title, true
		}
	}
	return "", false
}

// Registrations returns a detached copy of owner and runtime-group metadata.
func (s *Store) Registrations() RegistrationState {
	return cloneRegistrationState(s.snapshot.Load().registration)
}

// Update validates and atomically commits a complete sparse record at the expected revision.
func (s *Store) Update(groupID int64, expectedRevision uint64, next GroupOverrides) (CommitResult, error) {
	s.writer.Lock()
	defer s.writer.Unlock()
	if !s.writable {
		return CommitResult{}, s.unavailableError()
	}
	current := s.snapshot.Load()
	group, ok := current.groups[groupID]
	if !ok {
		return CommitResult{}, fmt.Errorf("%w: %d", ErrUnknownGroup, groupID)
	}
	if group.revision != expectedRevision {
		return CommitResult{}, &ConflictError{GroupID: groupID, Expected: expectedRevision, Actual: group.revision}
	}
	next = compactGroupOverrides(cloneGroupOverrides(next), group.baseline)
	if reflect.DeepEqual(group.overrides, next) {
		return CommitResult{Revision: group.revision, Durable: s.repository != nil || s.path != ""}, nil
	}
	candidate := cloneSettingsFile(s.state)
	record := candidate.Groups[groupID]
	record.Revision = group.revision + 1
	record.GroupOverrides = next
	candidate.Groups[groupID] = record
	snap, err := s.buildSnapshot(candidate)
	if err != nil {
		return CommitResult{}, err
	}
	if s.repository != nil {
		actual, written, writeErr := s.repository.CompareAndSwapSettings(groupID, expectedRevision, next)
		if writeErr != nil {
			s.setLastError(writeErr)
			return CommitResult{}, writeErr
		}
		if !written {
			return CommitResult{}, &ConflictError{GroupID: groupID, Expected: expectedRevision, Actual: actual}
		}
		record.Revision = actual
		candidate.Groups[groupID] = record
	} else if err := s.writeState(&candidate); err != nil {
		s.setLastError(err)
		return CommitResult{}, err
	}
	s.state = candidate
	s.snapshot.Store(snap)
	s.setLastError(nil)
	return CommitResult{Revision: record.Revision, Durable: s.repository != nil || s.path != ""}, nil
}

func (v GroupView) ID() int64                     { return v.group.id }
func (v GroupView) Revision() uint64              { return v.group.revision }
func (v GroupView) RuntimeRegistered() bool       { return v.group.registered }
func (v GroupView) Enabled() Setting[bool]        { return v.group.enabled }
func (v GroupView) DeliveryMode() Setting[string] { return v.group.deliveryMode }
func (v GroupView) VerifyMode() Setting[string]   { return v.group.verifyMode }
func (v GroupView) NameSpoiler() Setting[bool]    { return v.group.nameSpoiler }
func (v GroupView) BanSeconds() Setting[int]      { return v.group.banSeconds }
func (v GroupView) LookupTTLSeconds() Setting[int] {
	return v.group.lookupTTLSeconds
}
func (v GroupView) LookupAutoDeleteEnabled() Setting[bool] {
	return v.group.lookupAutoDeleteEnabled
}
func (v GroupView) TimeoutSeconds() Setting[int]      { return v.group.timeoutSeconds }
func (v GroupView) VerifyMaxFails() Setting[int]      { return v.group.verifyMaxFails }
func (v GroupView) VerifyRetrySeconds() Setting[int]  { return v.group.verifyRetrySeconds }
func (v GroupView) MuteSeconds() Setting[int]         { return v.group.muteSeconds }
func (v GroupView) VerifyInvited() Setting[bool]      { return v.group.verifyInvited }
func (v GroupView) WarnLimit() Setting[int]           { return v.group.warnLimit }
func (v GroupView) AntispamEnabled() Setting[bool]    { return v.group.antispamEnabled }
func (v GroupView) ChannelDisplay() Setting[string]   { return v.group.channelDisplay }
func (v GroupView) ChannelInviteURL() Setting[string] { return v.group.channelInviteURL }
func (v GroupView) FallbackBuiltin() Setting[bool]    { return v.group.fallbackBuiltin }
func (v GroupView) Lang() Setting[string]             { return v.group.lang }
func (v GroupView) RequiredChannelID() Setting[int64] { return v.group.requiredChannelID }
func (v GroupView) RichMessages() Setting[bool]       { return v.group.richMessages }
func (v GroupView) PrivateQueryPerMin() Setting[int]  { return v.group.privateQueryPerMin }
func (v GroupView) AdminLogChatID() Setting[int64]    { return v.group.adminLogChatID }
func (v GroupView) RequiredChannelFailOpen() Setting[bool] {
	return v.group.requiredChannelFailOpen
}

// Overrides returns a detached copy suitable for editing and Update.
func (v GroupView) Overrides() GroupOverrides { return cloneGroupOverrides(v.group.overrides) }

func (v GroupView) ChannelWhitelist() Setting[[]int64] {
	return Setting[[]int64]{Value: cloneInt64s(v.group.channelWhitelist.Value), Source: v.group.channelWhitelist.Source}
}

func (v GroupView) TrustedMemberGroupIDs() Setting[[]int64] {
	return Setting[[]int64]{Value: cloneInt64s(v.group.trustedMemberGroupIDs.Value), Source: v.group.trustedMemberGroupIDs.Source}
}

func (v GroupView) KnownChatIDs() Setting[[]int64] {
	return Setting[[]int64]{Value: cloneInt64s(v.group.knownChatIDs.Value), Source: v.group.knownChatIDs.Source}
}

func (v GroupView) Questions() Setting[[]Question] {
	return Setting[[]Question]{Value: cloneQuestions(v.group.questions.Value), Source: v.group.questions.Source}
}

func (v GroupView) FallbackQuestions() Setting[[]ShortQuestion] {
	return Setting[[]ShortQuestion]{Value: cloneShortQuestions(v.group.fallbackQuestions.Value), Source: v.group.fallbackQuestions.Source}
}
