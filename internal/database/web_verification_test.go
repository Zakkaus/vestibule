package database

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type webGateway struct {
	verification.Gateway
	approvals, declines atomic.Int32
}

func (g *webGateway) ApproveJoin(context.Context, int64, int64) error { g.approvals.Add(1); return nil }
func (g *webGateway) DeclineJoin(context.Context, int64, int64) error { g.declines.Add(1); return nil }
func (*webGateway) Member(context.Context, int64, int64) (verification.ChatMember, error) {
	return &verification.ChatMemberLeft{Status: verification.MemberStatusLeft}, nil
}
func (*webGateway) Send(context.Context, verification.OutgoingMessage) (int, error) { return 1, nil }
func (*webGateway) SendHTMLFallback(context.Context, int64, string, string) (int, error) {
	return 1, nil
}
func (*webGateway) Alert(context.Context, int64, string)     {}
func (*webGateway) Delete(context.Context, int64, int) error { return nil }

type webFixture struct {
	db       *Database
	state    *VerificationStore
	service  *verification.Service
	gateway  *webGateway
	config   Config
	settings *settings.Store
	record   verification.PendingRecord
	token    string
	proof    verification.WebTokenRecord
}

func newWebFixture(t *testing.T, mode string) *webFixture {
	t.Helper()
	f := &webFixture{config: testSQLiteConfig(t), gateway: &webGateway{}}
	var err error
	f.db, err = Open(context.Background(), f.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	f.state = NewVerificationStore(f.db)
	f.record = verification.PendingRecord{GroupID: -1009000000777, UserID: 777, Nonce: "web", Mode: mode,
		Lang: "ja", Deadline: time.Now().Add(time.Hour).Unix(), ChallengeDelivered: true, CorrectIdx: -1}
	if inserted, err := f.state.InsertPending("", f.record); err != nil || !inserted {
		t.Fatalf("seed challenge: %v, %v", inserted, err)
	}
	f.restore(t)
	t.Cleanup(func() { f.service.Shutdown() })
	f.token, f.proof, err = f.service.IssueWebToken(context.Background(), verification.ChallengeID(f.record.Ref()))
	if err != nil || f.token == "" {
		t.Fatalf("issue token: %q, %v", f.token, err)
	}
	return f
}

func (f *webFixture) restore(t *testing.T) {
	t.Helper()
	f.restoreWithCapabilities(t, settings.WebCapabilities{ConsoleURL: "https://console.example", TurnstileAvailable: true})
}

func (f *webFixture) restoreWithCapabilities(t *testing.T, capabilities settings.WebCapabilities) {
	t.Helper()
	bits := 12
	cfg := &settings.Config{GroupIDs: []int64{f.record.GroupID}, Groups: []settings.GroupConfig{{ID: f.record.GroupID, VerifyMode: f.record.Mode, DeliveryMode: settings.DeliveryDM, PoWBits: &bits}}}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.settings, err = settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.settings.SetWebCapabilities(capabilities)
	f.service, err = verification.New(f.settings, f.gateway, f.state, cfg, &i18n.Messages, nil, verification.Identity{}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
}

func solveWebPoW(t *testing.T, token verification.WebTokenRecord) string {
	t.Helper()
	for n := range uint64(1 << 24) {
		nonce := strconv.FormatUint(n, 10)
		if verification.VerifyPoW(token.Salt, nonce, token.PoWBits) {
			return nonce
		}
	}
	t.Fatal("PoW fixture exceeded search range")
	return ""
}

func TestAnswerWebConcurrentAfterSQLiteRestart(t *testing.T) {
	f := newWebFixture(t, settings.ModePoW)
	f.service.Shutdown()
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = Open(context.Background(), f.config)
	if err != nil {
		t.Fatal(err)
	}
	f.state = NewVerificationStore(f.db)
	f.restore(t)
	nonce := solveWebPoW(t, f.proof)
	results := make(chan verification.WebResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			result, err := f.service.AnswerWeb(context.Background(), verification.ChallengeID(f.record.Ref()), verification.WebProof{Token: f.token, Nonce: nonce})
			results <- result
			errors <- err
		}()
	}
	wins, settled := 0, 0
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
		switch result := <-results; result.Outcome {
		case verification.WebReceived:
			wins++
		case verification.WebSettled:
			settled++
		default:
			t.Fatalf("unexpected answer: %#v", result)
		}
	}
	if wins != 1 || settled != 1 || f.gateway.approvals.Load() != 1 {
		t.Fatalf("wins=%d settled=%d approvals=%d", wins, settled, f.gateway.approvals.Load())
	}
	var state, kind, action string
	err = f.db.QueryRow(context.Background(), `SELECT challenge.state, challenge.kind, pending_action.state FROM challenge JOIN pending_action ON challenge.id=pending_action.challenge_id`).Scan(&state, &kind, &action)
	if err != nil {
		t.Fatal(err)
	}
	if state != "approved" || kind != "pow" || action != "done" {
		t.Fatalf("durable settlement=%s/%s/%s", state, kind, action)
	}
}
