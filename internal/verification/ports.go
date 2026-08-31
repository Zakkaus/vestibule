package verification

import (
	"context"
	"errors"
	"time"
)

const (
	ChatTypePrivate    = "private"
	ChatTypeSupergroup = "supergroup"

	MemberStatusCreator       = "creator"
	MemberStatusAdministrator = "administrator"
	MemberStatusMember        = "member"
	MemberStatusRestricted    = "restricted"
	MemberStatusLeft          = "left"
	MemberStatusBanned        = "kicked"
)

// User is the platform-neutral identity carried by an incoming verification event.
type User struct {
	ID           int64
	IsBot        bool
	FirstName    string
	Username     string
	LanguageCode string
}

func (u User) DisplayName() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return u.FirstName
}

// Chat is the platform-neutral chat identity carried by an incoming verification event.
type Chat struct {
	ID   int64
	Type string
}

// ChatJoinRequest is the information the core needs from a join-request event.
type ChatJoinRequest struct {
	Chat Chat
	From User
}

// ChatMember exposes membership state without leaking a platform SDK type.
type ChatMember interface {
	MemberStatus() string
	MemberIsMember() bool
	MemberUser() User
	RestrictionUntil() (int64, bool)
}

type ChatMemberMember struct {
	Status string
	User   User
}

func (m *ChatMemberMember) MemberStatus() string          { return m.Status }
func (*ChatMemberMember) MemberIsMember() bool            { return true }
func (m *ChatMemberMember) MemberUser() User              { return m.User }
func (*ChatMemberMember) RestrictionUntil() (int64, bool) { return 0, false }

type ChatMemberAdministrator struct {
	Status string
	User   User
}

func (m *ChatMemberAdministrator) MemberStatus() string          { return m.Status }
func (*ChatMemberAdministrator) MemberIsMember() bool            { return true }
func (m *ChatMemberAdministrator) MemberUser() User              { return m.User }
func (*ChatMemberAdministrator) RestrictionUntil() (int64, bool) { return 0, false }

type ChatMemberOwner struct {
	Status string
	User   User
}

func (m *ChatMemberOwner) MemberStatus() string          { return m.Status }
func (*ChatMemberOwner) MemberIsMember() bool            { return true }
func (m *ChatMemberOwner) MemberUser() User              { return m.User }
func (*ChatMemberOwner) RestrictionUntil() (int64, bool) { return 0, false }

type ChatMemberRestricted struct {
	Status    string
	User      User
	IsMember  bool
	UntilDate int64
}

func (m *ChatMemberRestricted) MemberStatus() string { return m.Status }
func (m *ChatMemberRestricted) MemberIsMember() bool { return m.IsMember }
func (m *ChatMemberRestricted) MemberUser() User     { return m.User }
func (m *ChatMemberRestricted) RestrictionUntil() (int64, bool) {
	return m.UntilDate, true
}

type ChatMemberLeft struct {
	Status string
	User   User
}

func (m *ChatMemberLeft) MemberStatus() string          { return m.Status }
func (*ChatMemberLeft) MemberIsMember() bool            { return false }
func (m *ChatMemberLeft) MemberUser() User              { return m.User }
func (*ChatMemberLeft) RestrictionUntil() (int64, bool) { return 0, false }

type ChatMemberBanned struct {
	Status string
	User   User
}

func (m *ChatMemberBanned) MemberStatus() string          { return m.Status }
func (*ChatMemberBanned) MemberIsMember() bool            { return false }
func (m *ChatMemberBanned) MemberUser() User              { return m.User }
func (*ChatMemberBanned) RestrictionUntil() (int64, bool) { return 0, false }

// ChatMemberUpdated is the information the core needs from a membership transition.
type ChatMemberUpdated struct {
	Chat          Chat
	From          User
	OldChatMember ChatMember
	NewChatMember ChatMember
}

// CallbackQuery is a platform-neutral interaction with one verification button.
type CallbackQuery struct {
	ID      string
	From    User
	Message *Message
	Data    string
}

// Message is the platform-neutral subset of an incoming direct message.
type Message struct {
	MessageID int
	Chat      Chat
	From      *User
	Text      string
}

// Update contains one verification-relevant event. The Telegram adapter constructs it.
type Update struct {
	ChatJoinRequest *ChatJoinRequest
	ChatMember      *ChatMemberUpdated
	CallbackQuery   *CallbackQuery
	Message         *Message
}

// HandlerContext carries cancellation and the gateway selected by the assembly layer.
type HandlerContext struct {
	ctx     context.Context
	gateway Gateway
}

func NewHandlerContext(ctx context.Context, gateway Gateway) *HandlerContext {
	return &HandlerContext{ctx: ctx, gateway: gateway}
}

func (c *HandlerContext) Context() context.Context { return c.ctx }
func (c *HandlerContext) Gateway() Gateway         { return c.gateway }

type Handler func(*HandlerContext, Update) error

// Button is one platform-neutral action attached to an outbound message.
type Button struct {
	Text         string
	URL          string
	CallbackData string
}

// OutgoingMessage contains content already selected and rendered by the core.
type OutgoingMessage struct {
	ChatID             int64
	Text               string
	HTML               bool
	DisableLinkPreview bool
	Buttons            [][]Button
}

func sendText(ctx context.Context, gateway Gateway, chatID int64, text string) (int, error) {
	return gateway.Send(ctx, OutgoingMessage{ChatID: chatID, Text: text})
}

func sendHTML(ctx context.Context, gateway Gateway, chatID int64, text string, buttons [][]Button) (int, error) {
	return gateway.Send(ctx, OutgoingMessage{
		ChatID: chatID, Text: text, HTML: true, DisableLinkPreview: true, Buttons: buttons,
	})
}

func ackFast(ctx context.Context, gateway Gateway, interactionID string) {
	_ = gateway.AckFast(ctx, interactionID)
}

func ackResult(ctx context.Context, gateway Gateway, interactionID, text string, alert bool) {
	_ = gateway.AckResult(ctx, interactionID, AckResult{Text: text, Alert: alert})
}

// AckResult is the result shown for an interaction. Alert selects a modal alert instead of a toast.
type AckResult struct {
	Text  string
	Alert bool
}

// Gateway is the complete external-action surface used by verification.
//
// Every method may perform network I/O and must not be called while a database transaction is
// open. Implementations must not decide verification outcomes or retry an ambiguous identical
// send. SendHTMLFallback may retry only rejected markup with the supplied degraded content.
// An interaction may call exactly one of AckFast and AckResult.
type Gateway interface {
	Send(context.Context, OutgoingMessage) (int, error)
	SendHTMLFallback(context.Context, int64, string, string) (int, error)
	Delete(context.Context, int64, int)
	Notify(context.Context, int64, string, int)
	Alert(context.Context, int64, string)
	AuditLog(context.Context, int64, string)
	FailAlert(context.Context, int64, int64, string)

	ApproveJoin(context.Context, int64, int64) error
	DeclineJoin(context.Context, int64, int64) error
	Ban(context.Context, int64, int64, int, bool) error
	Unban(context.Context, int64, int64, bool) error
	Mute(context.Context, int64, int64, int) error
	Unmute(context.Context, int64, int64) error
	Member(context.Context, int64, int64) (ChatMember, error)

	CachedAdmin(context.Context, int64, int64) (bool, error)
	FreshAdmin(context.Context, int64, int64) (bool, error)
	AckFast(context.Context, string) error
	AckResult(context.Context, string, AckResult) error
}

// LiveProbe reports whether the external service is reachable without exposing its identity type.
type LiveProbe interface {
	Probe(context.Context) error
}

type FailureKind uint16

const (
	FailureJoinRequestGone FailureKind = 1 << iota
	FailureApplicantGone
	FailureCannotInitiateConversation
	FailureBlockedByUser
	FailureGroupUnreachable
	FailureRateLimited
)

// GatewayError is a platform failure translated at the adapter boundary.
type GatewayError struct {
	Cause      error
	Kinds      FailureKind
	Code       int
	RetryAfter time.Duration
}

func (e *GatewayError) Error() string {
	if e == nil || e.Cause == nil {
		return "gateway error"
	}
	return e.Cause.Error()
}

func (e *GatewayError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func gatewayError(err error) *GatewayError {
	var translated *GatewayError
	if errors.As(err, &translated) {
		return translated
	}
	return nil
}

func gatewayFailureHas(err error, kind FailureKind) bool {
	translated := gatewayError(err)
	return translated != nil && translated.Kinds&kind != 0
}

func gatewayFailureCode(err error) int {
	if translated := gatewayError(err); translated != nil {
		return translated.Code
	}
	return 0
}

func gatewayFailureRetryAfter(err error) time.Duration {
	if translated := gatewayError(err); translated != nil {
		return translated.RetryAfter
	}
	return 0
}

func pace(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// PendingRecord is the legacy JSON representation of one in-progress verification.
type PendingRecord struct {
	UserID             int64    `json:"user_id"`
	GroupID            int64    `json:"group_id"`
	GroupMsgID         int      `json:"group_msg_id"`
	PrivateMsgID       int      `json:"private_msg_id,omitempty"`
	ChallengeDelivered bool     `json:"challenge_delivered,omitempty"`
	Mode               string   `json:"mode,omitempty"`
	Lang               string   `json:"lang,omitempty"`
	FbAnswers          []string `json:"fb_answers,omitempty"`
	FallbackPending    bool     `json:"fallback_pending,omitempty"`
	Prompted           bool     `json:"prompted,omitempty"`
	Hinted             bool     `json:"hinted,omitempty"`
	SampleBounced      bool     `json:"sample_bounced,omitempty"`
	NoLinuxReminded    bool     `json:"no_linux_reminded,omitempty"`
	OSClarified        bool     `json:"os_clarified,omitempty"`
	Tries              int      `json:"tries,omitempty"`
	QText              string   `json:"q_text"`
	QOpts              []string `json:"q_opts"`
	CorrectIdx         int      `json:"correct_idx"`
	Nonce              string   `json:"nonce"`
	Name               string   `json:"name,omitempty"`
	Deadline           int64    `json:"deadline"`
	DeferredSince      int64    `json:"deferred_since,omitempty"`
	DeferralCapReached bool     `json:"deferral_cap_reached,omitempty"`
	Gate               string   `json:"gate,omitempty"`
	Invited            bool     `json:"invited,omitempty"`
	Held               bool     `json:"held,omitempty"`
	HoldUntil          int64    `json:"hold_until,omitempty"`
	ChannelUnreadable  bool     `json:"channel_unreadable,omitempty"`
	Passing            bool     `json:"passing,omitempty"`
	SettleFailures     int      `json:"settle_failures,omitempty"`
	SettlePendingSaid  bool     `json:"settle_pending_said,omitempty"`
}

// FailureRecord is the legacy JSON representation of one applicant's strike window.
type FailureRecord struct {
	GroupID int64 `json:"group_id"`
	UserID  int64 `json:"user_id"`
	Count   int   `json:"count"`
	Last    int64 `json:"last"`
}

// AgentTally is the legacy JSON representation of automated-agent tripwire counts.
type AgentTally struct {
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

// HeartbeatRecord is the legacy JSON representation of the last successful probe.
type HeartbeatRecord struct {
	LastOnline int64 `json:"last_online"`
}

// ErrStoreReadOnly marks a state file that could not be read without risking later overwrite.
var ErrStoreReadOnly = errors.New("verification state is read-only")

// Store persists the four verification snapshots. Snapshot callbacks execute under the
// implementation's process-wide write lock, preserving the existing lock order. Implementations
// must not perform network I/O; database implementations ignore the legacy path arguments.
type Store interface {
	LoadPending(string) ([]PendingRecord, error)
	SavePending(string, func() []PendingRecord) error
	LoadFailures(string) ([]FailureRecord, error)
	SaveFailures(string, func() []FailureRecord) error
	LoadAgents(string) (AgentTally, error)
	SaveAgents(string, func() AgentTally) error
	LoadHeartbeat(string) (HeartbeatRecord, error)
	SaveHeartbeat(string, HeartbeatRecord) error
}
