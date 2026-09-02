package verification

import (
	"context"
	"log"
	"sync/atomic"
)

// ObservedOperation identifies one external write suppressed by observe-only mode.
type ObservedOperation string

const (
	ObservedSend             ObservedOperation = "send"
	ObservedSendHTMLFallback ObservedOperation = "send_html_fallback"
	ObservedDelete           ObservedOperation = "delete"
	ObservedNotify           ObservedOperation = "notify"
	ObservedAlert            ObservedOperation = "alert"
	ObservedAuditLog         ObservedOperation = "audit_log"
	ObservedFailAlert        ObservedOperation = "fail_alert"
	ObservedApproveJoin      ObservedOperation = "approve_join"
	ObservedDeclineJoin      ObservedOperation = "decline_join"
	ObservedBan              ObservedOperation = "ban"
	ObservedUnban            ObservedOperation = "unban"
	ObservedMute             ObservedOperation = "mute"
	ObservedUnmute           ObservedOperation = "unmute"
	ObservedAckFast          ObservedOperation = "ack_fast"
	ObservedAckResult        ObservedOperation = "ack_result"
)

// Valid reports whether the operation names one of Gateway's external writes.
func (operation ObservedOperation) Valid() bool {
	switch operation {
	case ObservedSend, ObservedSendHTMLFallback, ObservedDelete, ObservedNotify, ObservedAlert,
		ObservedAuditLog, ObservedFailAlert, ObservedApproveJoin, ObservedDeclineJoin, ObservedBan,
		ObservedUnban, ObservedMute, ObservedUnmute, ObservedAckFast, ObservedAckResult:
		return true
	default:
		return false
	}
}

// ObservedAction is a privacy-limited record of a suppressed write. ChatID and UserID are set only
// for membership mutations; message bodies, callback IDs, and challenge answers are never retained.
type ObservedAction struct {
	Operation ObservedOperation
	ChatID    int64
	UserID    int64
	Seconds   int
	Flag      bool
}

// ObservationRecorder durably records writes that observe-only mode suppresses.
type ObservationRecorder interface {
	RecordObservedAction(context.Context, ObservedAction) error
}

// ObserveOnlyGateway keeps reads on the live gateway while replacing every external write with a
// durable observation. Synthetic message IDs are negative and cannot name a Telegram message.
type ObserveOnlyGateway struct {
	live          Gateway
	recorder      ObservationRecorder
	nextMessageID atomic.Int64
}

var _ Gateway = (*ObserveOnlyGateway)(nil)

// ApplyObservationMode returns the live gateway unchanged unless observe-only mode is enabled.
func ApplyObservationMode(live Gateway, recorder ObservationRecorder, enabled bool) Gateway {
	if live == nil {
		panic("verification: observe-only mode requires a live gateway")
	}
	if !enabled {
		return live
	}
	if recorder == nil {
		panic("verification: observe-only mode requires an observation recorder")
	}
	return &ObserveOnlyGateway{live: live, recorder: recorder}
}

func (g *ObserveOnlyGateway) record(ctx context.Context, action ObservedAction) error {
	return g.recorder.RecordObservedAction(ctx, action)
}

func (g *ObserveOnlyGateway) recordWithoutResult(ctx context.Context, action ObservedAction) {
	if err := g.record(ctx, action); err != nil {
		log.Printf("observe-only: record %s: %v", action.Operation, err)
	}
}

func (g *ObserveOnlyGateway) syntheticMessageID() int {
	return int(g.nextMessageID.Add(-1))
}

func (g *ObserveOnlyGateway) Send(ctx context.Context, message OutgoingMessage) (int, error) {
	if err := g.record(ctx, ObservedAction{Operation: ObservedSend, Flag: message.HTML}); err != nil {
		return 0, err
	}
	return g.syntheticMessageID(), nil
}

func (g *ObserveOnlyGateway) SendHTMLFallback(ctx context.Context, _ int64, _, _ string) (int, error) {
	if err := g.record(ctx, ObservedAction{Operation: ObservedSendHTMLFallback}); err != nil {
		return 0, err
	}
	return g.syntheticMessageID(), nil
}

func (g *ObserveOnlyGateway) Delete(ctx context.Context, _ int64, _ int) error {
	return g.record(ctx, ObservedAction{Operation: ObservedDelete})
}

func (g *ObserveOnlyGateway) Notify(ctx context.Context, _ int64, _ string, ttlSeconds int) {
	g.recordWithoutResult(ctx, ObservedAction{Operation: ObservedNotify, Seconds: ttlSeconds})
}

func (g *ObserveOnlyGateway) Alert(ctx context.Context, _ int64, _ string) {
	g.recordWithoutResult(ctx, ObservedAction{Operation: ObservedAlert})
}

func (g *ObserveOnlyGateway) AuditLog(ctx context.Context, _ int64, _ string) {
	g.recordWithoutResult(ctx, ObservedAction{Operation: ObservedAuditLog})
}

func (g *ObserveOnlyGateway) FailAlert(ctx context.Context, _, _ int64, _ string) {
	g.recordWithoutResult(ctx, ObservedAction{Operation: ObservedFailAlert})
}

func (g *ObserveOnlyGateway) ApproveJoin(ctx context.Context, chatID, userID int64) error {
	return g.record(ctx, memberObservedAction(ObservedApproveJoin, chatID, userID))
}

func (g *ObserveOnlyGateway) DeclineJoin(ctx context.Context, chatID, userID int64) error {
	return g.record(ctx, memberObservedAction(ObservedDeclineJoin, chatID, userID))
}

func (g *ObserveOnlyGateway) Ban(ctx context.Context, chatID, userID int64, seconds int, revoke bool) error {
	action := memberObservedAction(ObservedBan, chatID, userID)
	action.Seconds, action.Flag = seconds, revoke
	return g.record(ctx, action)
}

func (g *ObserveOnlyGateway) Unban(ctx context.Context, chatID, userID int64, onlyIfBanned bool) error {
	action := memberObservedAction(ObservedUnban, chatID, userID)
	action.Flag = onlyIfBanned
	return g.record(ctx, action)
}

func (g *ObserveOnlyGateway) Mute(ctx context.Context, chatID, userID int64, seconds int) error {
	action := memberObservedAction(ObservedMute, chatID, userID)
	action.Seconds = seconds
	return g.record(ctx, action)
}

func (g *ObserveOnlyGateway) Unmute(ctx context.Context, chatID, userID int64) error {
	return g.record(ctx, memberObservedAction(ObservedUnmute, chatID, userID))
}

func memberObservedAction(operation ObservedOperation, chatID, userID int64) ObservedAction {
	return ObservedAction{Operation: operation, ChatID: chatID, UserID: userID}
}

func (g *ObserveOnlyGateway) Member(ctx context.Context, chatID, userID int64) (ChatMember, error) {
	return g.live.Member(ctx, chatID, userID)
}

func (g *ObserveOnlyGateway) CachedAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return g.live.CachedAdmin(ctx, chatID, userID)
}

func (g *ObserveOnlyGateway) FreshAdmin(ctx context.Context, chatID, userID int64) (bool, error) {
	return g.live.FreshAdmin(ctx, chatID, userID)
}

func (g *ObserveOnlyGateway) AckFast(ctx context.Context, _ string) error {
	return g.record(ctx, ObservedAction{Operation: ObservedAckFast})
}

func (g *ObserveOnlyGateway) AckResult(ctx context.Context, _ string, result AckResult) error {
	return g.record(ctx, ObservedAction{Operation: ObservedAckResult, Flag: result.Alert})
}
