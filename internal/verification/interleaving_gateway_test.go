package verification_test

import (
	"context"
	"errors"
	"strings"
	"time"

	v "github.com/Zakkaus/vestibule/internal/verification"
)

type chaosMember struct {
	present   bool
	heldUntil int64
	banned    bool
}

type chaosGateway struct {
	v.Gateway
	fixture        *chaosFixture
	members        map[chaosPair]chaosMember
	faults         map[string]string
	questions      map[[2]int64]bool
	admitted       map[chaosPair]time.Time
	messageID      int
	noFaultCalls   map[chaosPair]string
	calls          map[string]int
	afterGroupSend func(int64, int)
}

// Faults are one-shot: a failure step arms the next call, not a wall-clock outage.
func (b *chaosGateway) fault(op string, pair chaosPair) error {
	if b.calls == nil {
		b.calls = make(map[string]int)
	}
	kind := b.faults[op]
	delete(b.faults, op)
	if kind == "" {
		return nil
	}
	b.calls[op+"/"+kind]++
	b.fixture.lastFault = op + "/" + kind
	err := &v.GatewayError{Cause: errors.New("injected " + op + " " + kind)}
	switch kind {
	case "transient":
		err.Code = 503
	case "permanent":
		err.Code = 403
		err.Kinds = v.FailureGroupUnreachable
	case "request-gone":
		err.Code = 400
		err.Kinds = v.FailureJoinRequestGone
	case "member-gone":
		err.Code = 400
		err.Kinds = v.FailureApplicantGone
		b.members[pair] = chaosMember{}
	default:
		b.fixture.t.Fatalf("unknown fault %q", kind)
	}
	return err
}

func (b *chaosGateway) settlement(pair chaosPair) {
	f := b.fixture
	if f.service == nil {
		return
	}
	for id, actionID := range f.service.InterleavingSnapshot().Claimed {
		prefix := chaosID(v.PendingRecord{GroupID: pair.group, UserID: pair.user})
		if !strings.HasPrefix(id, prefix) {
			continue
		}
		var state string
		chaosRequire(f.t, f.db.QueryRow(context.Background(), "SELECT state FROM challenge WHERE id=$1", id).Scan(&state))
		if state == "superseded" || f.canceled[id] {
			cause := f.cancelKind[id]
			if cause == "" {
				cause = "durable-state"
			}
			f.findings = append(f.findings, chaosFinding{"superseded-settlement/" + cause, "gateway called for canceled challenge"})
		}
		if actionID != "" {
			var payload struct {
				Record v.PendingRecord `json:"record"`
			}
			var epoch uint64
			var encoded string
			chaosRequire(f.t, f.db.QueryRow(context.Background(),
				"SELECT a.payload,c.epoch FROM pending_action a JOIN challenge c ON c.id=a.challenge_id WHERE a.id=$1",
				actionID).Scan(&encoded, &epoch))
			chaosDecode(f.t, encoded, &payload)
			if payload.Record.Epoch != epoch {
				f.findings = append(f.findings, chaosFinding{"superseded-settlement/epoch", "gateway called with obsolete epoch"})
			}
		}
		if cause := f.noFault[id]; cause != "" {
			b.noFaultCalls[pair] = cause
		}
	}
}

func (b *chaosGateway) Send(ctx context.Context, message v.OutgoingMessage) (int, error) {
	_, err := b.Gateway.Send(ctx, message)
	if err != nil {
		return 0, err
	}
	b.messageID++
	if message.ChatID < 0 && len(message.Buttons) > 0 {
		b.questions[[2]int64{message.ChatID, int64(b.messageID)}] = true
		if b.afterGroupSend != nil {
			hook := b.afterGroupSend
			b.afterGroupSend = nil
			hook(message.ChatID, b.messageID)
		}
	}
	return b.messageID, nil
}

func (b *chaosGateway) SendHTMLFallback(ctx context.Context, id int64, rich, _ string) (int, error) {
	return b.Send(ctx, v.OutgoingMessage{ChatID: id, Text: rich})
}

func (b *chaosGateway) Notify(ctx context.Context, id int64, text string, _ int) {
	_, _ = b.Send(ctx, v.OutgoingMessage{ChatID: id, Text: text})
}

func (b *chaosGateway) Delete(ctx context.Context, group int64, message int) error {
	if err := b.fault("delete", chaosPair{group: group}); err != nil {
		return err
	}
	if err := b.Gateway.Delete(ctx, group, message); err != nil {
		return err
	}
	delete(b.questions, [2]int64{group, int64(message)})
	return nil
}

func (b *chaosGateway) ApproveJoin(ctx context.Context, group, user int64) error {
	pair := chaosPair{group, user}
	b.settlement(pair)
	if err := b.fault("approve", pair); err != nil {
		return err
	}
	if err := b.Gateway.ApproveJoin(ctx, group, user); err != nil {
		return err
	}
	b.members[pair] = chaosMember{present: true}
	b.admitted[pair] = b.fixture.now
	return nil
}

func (b *chaosGateway) DeclineJoin(ctx context.Context, group, user int64) error {
	pair := chaosPair{group, user}
	b.settlement(pair)
	if err := b.fault("decline", pair); err != nil {
		return err
	}
	return b.Gateway.DeclineJoin(ctx, group, user)
}

func (b *chaosGateway) Mute(ctx context.Context, group, user int64, seconds int) error {
	pair := chaosPair{group, user}
	if err := b.fault("restrict", pair); err != nil {
		return err
	}
	if err := b.Gateway.Mute(ctx, group, user, seconds); err != nil {
		return err
	}
	b.members[pair] = chaosMember{present: true, heldUntil: b.fixture.now.Unix() + int64(seconds)}
	return nil
}

func (b *chaosGateway) Unmute(ctx context.Context, group, user int64) error {
	pair := chaosPair{group, user}
	b.settlement(pair)
	if err := b.fault("unrestrict", pair); err != nil {
		return err
	}
	if err := b.Gateway.Unmute(ctx, group, user); err != nil {
		return err
	}
	b.members[pair] = chaosMember{present: true}
	// Cancellation releases are not admissions and must not extend recent-pass suppression.
	if b.fixture.service != nil {
		for id := range b.fixture.service.InterleavingSnapshot().Claimed {
			if strings.HasPrefix(id, chaosID(v.PendingRecord{GroupID: group, UserID: user})) && !b.fixture.canceled[id] {
				b.admitted[pair] = b.fixture.now
			}
		}
	}
	return nil
}

func (b *chaosGateway) Ban(ctx context.Context, group, user int64, seconds int, revoke bool) error {
	pair := chaosPair{group, user}
	b.settlement(pair)
	if err := b.Gateway.Ban(ctx, group, user, seconds, revoke); err != nil {
		return err
	}
	b.members[pair] = chaosMember{banned: true}
	return nil
}

func (b *chaosGateway) Unban(ctx context.Context, group, user int64, onlyIfBanned bool) error {
	pair := chaosPair{group, user}
	if err := b.fault("unban", pair); err != nil {
		return err
	}
	if err := b.Gateway.Unban(ctx, group, user, onlyIfBanned); err != nil {
		return err
	}
	member := b.members[pair]
	member.banned = false
	b.members[pair] = member
	return nil
}

func (b *chaosGateway) Member(_ context.Context, group, user int64) (v.ChatMember, error) {
	m := b.members[chaosPair{group, user}]
	if m.banned {
		return &v.ChatMemberBanned{Status: v.MemberStatusBanned}, nil
	}
	if !m.present {
		return &v.ChatMemberLeft{Status: v.MemberStatusLeft}, nil
	}
	if m.heldUntil > b.fixture.now.Unix() {
		return &v.ChatMemberRestricted{Status: v.MemberStatusRestricted, IsMember: true, UntilDate: m.heldUntil}, nil
	}
	return &v.ChatMemberMember{Status: v.MemberStatusMember}, nil
}
