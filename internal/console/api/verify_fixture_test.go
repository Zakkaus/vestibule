package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const apiWebChatID = int64(-1009000000781)

type apiWebGateway struct {
	verification.Gateway
	approvals, declines atomic.Int32
	channelMember       atomic.Bool
	channelReads        atomic.Int32
	messages            []verification.OutgoingMessage
}

func (g *apiWebGateway) ApproveJoin(context.Context, int64, int64) error {
	g.approvals.Add(1)
	return nil
}
func (g *apiWebGateway) DeclineJoin(context.Context, int64, int64) error {
	g.declines.Add(1)
	return nil
}
func (g *apiWebGateway) Member(_ context.Context, chatID, _ int64) (verification.ChatMember, error) {
	if chatID == apiWebChatID-1 {
		g.channelReads.Add(1)
		if g.channelMember.Load() {
			return &verification.ChatMemberMember{Status: verification.MemberStatusMember}, nil
		}
	}
	return &verification.ChatMemberLeft{Status: verification.MemberStatusLeft}, nil
}
func (g *apiWebGateway) Send(_ context.Context, message verification.OutgoingMessage) (int, error) {
	g.messages = append(g.messages, message)
	return len(g.messages), nil
}
func (*apiWebGateway) SendHTMLFallback(context.Context, int64, string, string) (int, error) {
	return 1, nil
}
func (*apiWebGateway) Delete(context.Context, int64, int) error { return nil }
func (*apiWebGateway) Alert(context.Context, int64, string)     {}

type apiWebFixture struct {
	db       *database.Database
	store    *database.VerificationStore
	settings *settings.Store
	service  *verification.Service
	gateway  *apiWebGateway
	server   *Server
	raw      string
	token    verification.WebTokenRecord
}

func newAPIWebFixture(t *testing.T, mode, language string) *apiWebFixture {
	t.Helper()
	ctx := context.Background()
	f := &apiWebFixture{gateway: &apiWebGateway{}}
	var err error
	f.db, err = database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.db.Close() })
	f.store = database.NewVerificationStore(f.db)
	record := verification.PendingRecord{GroupID: apiWebChatID, UserID: 781, Nonce: "surface", Mode: mode, Lang: language, Deadline: time.Now().Add(time.Hour).Unix(), ChallengeDelivered: true, CorrectIdx: -1}
	if inserted, err := f.store.InsertPending("", record); err != nil || !inserted {
		t.Fatalf("seed=%v/%v", inserted, err)
	}
	bits := 12
	cfg := &settings.Config{GroupIDs: []int64{apiWebChatID}, Groups: []settings.GroupConfig{{ID: apiWebChatID, VerifyMode: mode, DeliveryMode: settings.DeliveryDM, PoWBits: &bits}}}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.settings, err = settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.settings.SetWebCapabilities(settings.WebCapabilities{ConsoleURL: "https://console.example", TurnstileAvailable: true})
	f.service, err = verification.New(f.settings, f.gateway, f.store, cfg, &i18n.Messages, nil, verification.Identity{}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.service.Shutdown)
	f.raw, f.token, err = f.service.IssueWebToken(ctx, verification.ChallengeID(record.Ref()))
	if err != nil || f.raw == "" {
		t.Fatalf("token issuance failed: %v", err)
	}
	f.server = New(Config{WebVerification: f.service, ConsoleURL: "https://console.example", TurnstileSiteKey: "1x00000000000000000000AA", RequestLog: log.New(io.Discard, "", 0)})
	return f
}
func (f *apiWebFixture) request(method, raw, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/verify/"+raw, strings.NewReader(body))
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	return response
}
func apiWebNonce(t *testing.T, token verification.WebTokenRecord) string {
	t.Helper()
	for n := range uint64(1 << 24) {
		nonce := strconv.FormatUint(n, 10)
		if verification.VerifyPoW(token.Salt, nonce, token.PoWBits) {
			return nonce
		}
	}
	t.Fatal("proof fixture search exhausted")
	return ""
}
