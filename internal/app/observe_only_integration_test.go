package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const (
	observationDecisionGroup   = int64(-1009000000631)
	observationDecisionTrusted = int64(-1009000000632)
	observationDecisionUser    = int64(9000000633)
)

type observationDecisionGateway struct {
	verification.Gateway
	approvals int
}

func (g *observationDecisionGateway) Member(_ context.Context, chatID, userID int64) (verification.ChatMember, error) {
	if chatID != observationDecisionTrusted || userID != observationDecisionUser {
		return nil, nil
	}
	return &verification.ChatMemberMember{
		Status: verification.MemberStatusMember,
		User:   verification.User{ID: userID},
	}, nil
}

func (g *observationDecisionGateway) ApproveJoin(context.Context, int64, int64) error {
	g.approvals++
	return nil
}

func TestObserveOnlyTrustedDecisionIsComputedAndPersistedWithoutApproval(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	live := &observationDecisionGateway{}
	gateway, err := verificationGatewayForMode(
		ctx, &settings.Config{ObserveOnly: true}, db, live,
	)
	if err != nil {
		t.Fatal(err)
	}
	service := newObservationDecisionService(t, gateway)

	if err = service.OnJoinRequest(
		verification.NewHandlerContext(ctx, gateway), observationDecisionUpdate(),
	); err != nil {
		t.Fatal(err)
	}
	_, approved, declined := service.Stats()
	if approved != 1 || declined != 0 {
		t.Fatalf("decision tally = approved:%d declined:%d, want 1:0", approved, declined)
	}
	if live.approvals != 0 {
		t.Fatalf("live approvals = %d, want 0", live.approvals)
	}
	recorder, err := database.NewObservationStore(ctx, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := recorder.LoadObservedActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Operation != verification.ObservedApproveJoin ||
		actions[0].ChatID != observationDecisionGroup || actions[0].UserID != observationDecisionUser {
		t.Fatalf("stored decisions = %+v, want one approve_join for the applicant", actions)
	}
}

func TestDisabledObservationModeKeepsTrustedApprovalBehavior(t *testing.T) {
	live := &observationDecisionGateway{}
	gateway, err := verificationGatewayForMode(context.Background(), &settings.Config{}, nil, live)
	if err != nil {
		t.Fatal(err)
	}
	if gateway != live {
		t.Fatal("disabled observation mode did not preserve the live gateway")
	}
	service := newObservationDecisionService(t, gateway)
	if err := service.OnJoinRequest(
		verification.NewHandlerContext(context.Background(), gateway), observationDecisionUpdate(),
	); err != nil {
		t.Fatal(err)
	}
	_, approved, declined := service.Stats()
	if live.approvals != 1 || approved != 1 || declined != 0 {
		t.Fatalf("live approvals/tally = %d/%d/%d, want 1/1/0", live.approvals, approved, declined)
	}
}

func newObservationDecisionService(t *testing.T, gateway verification.Gateway) *verification.Service {
	t.Helper()
	cfg := &settings.Config{
		GroupIDs:              []int64{observationDecisionGroup},
		TrustedMemberGroupIDs: []int64{observationDecisionTrusted},
		VerifyMode:            settings.ModeKernel,
	}
	store, err := settings.NewStore("", botTestSettingsBaseline(t, cfg), nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(
		store, gateway, database.NewVerificationJSONStore(), cfg, &i18n.Messages,
		nil, verification.Identity{}, "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	return service
}

func observationDecisionUpdate() verification.Update {
	return verification.Update{ChatJoinRequest: &verification.ChatJoinRequest{
		Chat: verification.Chat{ID: observationDecisionGroup, Type: verification.ChatTypeSupergroup},
		From: verification.User{ID: observationDecisionUser, FirstName: "Applicant", LanguageCode: "en"},
	}}
}

func TestObserveOnlyModeReturnsStartupErrorWithoutDurableRecorder(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	live := &observationDecisionGateway{}
	gateway, panicValue, err := gatewayForModeWithoutPanic(
		ctx, &settings.Config{ObserveOnly: true}, db, live,
	)
	if panicValue != nil || err != nil || gateway == nil || gateway == live {
		t.Fatalf("healthy observe-only startup returned gateway=%T error=%v panic=%v", gateway, err, panicValue)
	}

	gateway, panicValue, err = gatewayForModeWithoutPanic(
		ctx, &settings.Config{ObserveOnly: true}, nil, live,
	)
	if panicValue != nil {
		t.Fatalf("missing durable observation store panicked instead of returning a startup error: %v", panicValue)
	}
	if gateway != nil || err == nil || !strings.Contains(err.Error(), "observation store requires a database") {
		t.Fatalf("startup without durable observation store returned gateway=%T error=%v, want a recorder error", gateway, err)
	}
}

func gatewayForModeWithoutPanic(
	ctx context.Context,
	cfg *settings.Config,
	db *database.Database,
	live verification.Gateway,
) (gateway verification.Gateway, panicValue any, err error) {
	defer func() { panicValue = recover() }()
	gateway, err = verificationGatewayForMode(ctx, cfg, db, live)
	return gateway, nil, err
}
