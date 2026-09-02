package verification

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

const (
	observeTestChatID = int64(-1009000000611)
	observeTestUserID = int64(9000000612)
)

type observationRecorderSpy struct {
	actions []ObservedAction
	err     error
}

func (r *observationRecorderSpy) RecordObservedAction(_ context.Context, action ObservedAction) error {
	r.actions = append(r.actions, action)
	return r.err
}

type observedWriteCase struct {
	operation ObservedOperation
	want      ObservedAction
	invoke    func(*testing.T, Gateway)
	leaked    func(*fakeVerifyBot) bool
}

func TestObserveOnlyGatewaySuppressesEveryExternalWrite(t *testing.T) {
	for _, test := range observedWriteCases() {
		t.Run(string(test.operation), func(t *testing.T) {
			live := newFakeVerifyBot()
			recorder := &observationRecorderSpy{}
			gateway := ApplyObservationMode(live, recorder, true)
			test.invoke(t, gateway)
			if test.leaked(live) {
				t.Errorf("%s reached the live gateway", test.operation)
			}
			if !reflect.DeepEqual(recorder.actions, []ObservedAction{test.want}) {
				t.Errorf("recorded actions = %+v, want %+v", recorder.actions, test.want)
			}
		})
	}
}

func observedWriteCases() []observedWriteCase {
	messageCases := observedMessageWriteCases()
	messageCases = append(messageCases, observedMembershipWriteCases()...)
	return append(messageCases, observedCallbackWriteCases()...)
}

func observedMessageWriteCases() []observedWriteCase {
	return []observedWriteCase{
		{
			operation: ObservedSend,
			want:      ObservedAction{Operation: ObservedSend, Flag: true},
			invoke: func(t *testing.T, gateway Gateway) {
				messageID, err := gateway.Send(context.Background(), OutgoingMessage{
					ChatID: observeTestUserID, Text: "private challenge", HTML: true,
				})
				requireSyntheticMessage(t, messageID, err)
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
		{
			operation: ObservedSendHTMLFallback,
			want:      ObservedAction{Operation: ObservedSendHTMLFallback},
			invoke: func(t *testing.T, gateway Gateway) {
				messageID, err := gateway.SendHTMLFallback(context.Background(), observeTestUserID, "rich", "plain")
				requireSyntheticMessage(t, messageID, err)
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
		{
			operation: ObservedDelete,
			want:      ObservedAction{Operation: ObservedDelete},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.Delete(context.Background(), observeTestChatID, 17))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.deletes != 0 },
		},
		{
			operation: ObservedNotify,
			want:      ObservedAction{Operation: ObservedNotify, Seconds: 60},
			invoke: func(_ *testing.T, gateway Gateway) {
				gateway.Notify(context.Background(), observeTestChatID, "notice", 60)
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
		{
			operation: ObservedAlert,
			want:      ObservedAction{Operation: ObservedAlert},
			invoke: func(_ *testing.T, gateway Gateway) {
				gateway.Alert(context.Background(), observeTestChatID, "alert")
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
		{
			operation: ObservedAuditLog,
			want:      ObservedAction{Operation: ObservedAuditLog},
			invoke: func(_ *testing.T, gateway Gateway) {
				gateway.AuditLog(context.Background(), observeTestChatID, "audit")
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
		{
			operation: ObservedFailAlert,
			want:      ObservedAction{Operation: ObservedFailAlert},
			invoke: func(_ *testing.T, gateway Gateway) {
				gateway.FailAlert(context.Background(), observeTestChatID, observeTestChatID, "failure")
			},
			leaked: func(live *fakeVerifyBot) bool { return live.sends != 0 },
		},
	}
}

func observedMembershipWriteCases() []observedWriteCase {
	return []observedWriteCase{
		memberWriteCase(ObservedApproveJoin, func(gateway Gateway) error {
			return gateway.ApproveJoin(context.Background(), observeTestChatID, observeTestUserID)
		}, func(live *fakeVerifyBot) bool { return live.approves != 0 }),
		memberWriteCase(ObservedDeclineJoin, func(gateway Gateway) error {
			return gateway.DeclineJoin(context.Background(), observeTestChatID, observeTestUserID)
		}, func(live *fakeVerifyBot) bool { return live.declines != 0 }),
		{
			operation: ObservedBan,
			want: ObservedAction{
				Operation: ObservedBan, ChatID: observeTestChatID, UserID: observeTestUserID,
				Seconds: 3600, Flag: true,
			},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.Ban(context.Background(), observeTestChatID, observeTestUserID, 3600, true))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.bans != 0 },
		},
		{
			operation: ObservedUnban,
			want: ObservedAction{
				Operation: ObservedUnban, ChatID: observeTestChatID, UserID: observeTestUserID, Flag: true,
			},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.Unban(context.Background(), observeTestChatID, observeTestUserID, true))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.unbans != 0 },
		},
		{
			operation: ObservedMute,
			want: ObservedAction{
				Operation: ObservedMute, ChatID: observeTestChatID, UserID: observeTestUserID, Seconds: 180,
			},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.Mute(context.Background(), observeTestChatID, observeTestUserID, 180))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.mutes != 0 },
		},
		memberWriteCase(ObservedUnmute, func(gateway Gateway) error {
			return gateway.Unmute(context.Background(), observeTestChatID, observeTestUserID)
		}, func(live *fakeVerifyBot) bool { return live.unmutes != 0 }),
	}
}

func observedCallbackWriteCases() []observedWriteCase {
	return []observedWriteCase{
		{
			operation: ObservedAckFast,
			want:      ObservedAction{Operation: ObservedAckFast},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.AckFast(context.Background(), "callback"))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.answers != 0 },
		},
		{
			operation: ObservedAckResult,
			want:      ObservedAction{Operation: ObservedAckResult, Flag: true},
			invoke: func(t *testing.T, gateway Gateway) {
				requireNoError(t, gateway.AckResult(context.Background(), "callback", AckResult{Text: "result", Alert: true}))
			},
			leaked: func(live *fakeVerifyBot) bool { return live.answers != 0 },
		},
	}
}

func memberWriteCase(
	operation ObservedOperation,
	call func(Gateway) error,
	leaked func(*fakeVerifyBot) bool,
) observedWriteCase {
	return observedWriteCase{
		operation: operation,
		want: ObservedAction{
			Operation: operation, ChatID: observeTestChatID, UserID: observeTestUserID,
		},
		invoke: func(t *testing.T, gateway Gateway) { requireNoError(t, call(gateway)) },
		leaked: leaked,
	}
}

func requireSyntheticMessage(t *testing.T, messageID int, err error) {
	t.Helper()
	requireNoError(t, err)
	if messageID >= 0 {
		t.Errorf("synthetic message ID = %d, want a negative non-platform ID", messageID)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

type readGatewaySpy struct {
	Gateway
	member      ChatMember
	memberCalls int
	cachedCalls int
	freshCalls  int
}

func (g *readGatewaySpy) Member(context.Context, int64, int64) (ChatMember, error) {
	g.memberCalls++
	return g.member, nil
}

func (g *readGatewaySpy) CachedAdmin(context.Context, int64, int64) (bool, error) {
	g.cachedCalls++
	return true, nil
}

func (g *readGatewaySpy) FreshAdmin(context.Context, int64, int64) (bool, error) {
	g.freshCalls++
	return false, nil
}

func TestObserveOnlyGatewayDelegatesEveryRead(t *testing.T) {
	member := &ChatMemberMember{Status: MemberStatusMember, User: User{ID: observeTestUserID}}
	live := &readGatewaySpy{member: member}
	gateway := ApplyObservationMode(live, &observationRecorderSpy{}, true)
	gotMember, err := gateway.Member(context.Background(), observeTestChatID, observeTestUserID)
	if err != nil || gotMember != member {
		t.Fatalf("Member() = (%v, %v), want delegated member", gotMember, err)
	}
	cached, err := gateway.CachedAdmin(context.Background(), observeTestChatID, observeTestUserID)
	if err != nil || !cached {
		t.Fatalf("CachedAdmin() = (%v, %v), want true", cached, err)
	}
	fresh, err := gateway.FreshAdmin(context.Background(), observeTestChatID, observeTestUserID)
	if err != nil || fresh {
		t.Fatalf("FreshAdmin() = (%v, %v), want false", fresh, err)
	}
	if live.memberCalls != 1 || live.cachedCalls != 1 || live.freshCalls != 1 {
		t.Fatalf("read calls = member:%d cached:%d fresh:%d, want one each",
			live.memberCalls, live.cachedCalls, live.freshCalls)
	}
}

func TestObservationModeDisabledReturnsLiveGatewayUnchanged(t *testing.T) {
	live := newFakeVerifyBot()
	gateway := ApplyObservationMode(live, nil, false)
	if gateway != live {
		t.Fatal("disabled observation mode wrapped the live gateway")
	}
	requireNoError(t, gateway.ApproveJoin(context.Background(), observeTestChatID, observeTestUserID))
	if live.approves != 1 {
		t.Fatalf("live approvals = %d, want 1", live.approves)
	}
}

func TestObserveOnlyGatewayDoesNotDelegateWhenRecordingFails(t *testing.T) {
	live := newFakeVerifyBot()
	failure := errors.New("observation store unavailable")
	gateway := ApplyObservationMode(live, &observationRecorderSpy{err: failure}, true)
	err := gateway.ApproveJoin(context.Background(), observeTestChatID, observeTestUserID)
	if !errors.Is(err, failure) || live.approves != 0 {
		t.Fatalf("ApproveJoin error = %v, live approvals = %d; want recorder error and no live call", err, live.approves)
	}
}
