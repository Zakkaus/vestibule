package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type queueApprovalGateway struct {
	trialParityGateway
	approvalError error
}

func (g *queueApprovalGateway) ApproveJoin(context.Context, int64, int64) error {
	g.approves++
	return g.approvalError
}

func (g *queueApprovalGateway) Member(context.Context, int64, int64) (verification.ChatMember, error) {
	return &verification.ChatMemberLeft{Status: verification.MemberStatusLeft}, nil
}

func (g *queueApprovalGateway) FailAlert(context.Context, int64, int64, string) {}

func TestQueueReleaseReportsActualApprovalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name, responseState, actionState string
		approvalError                    error
		attempts                         int
	}{
		{name: "confirmed", responseState: "approved", actionState: "done"},
		{name: "retry", responseState: "approval_pending_retry", actionState: "pending", attempts: 1, approvalError: errors.New("temporary approval failure")},
		{name: "gone", responseState: "approval_unconfirmed", actionState: "done", approvalError: &verification.GatewayError{Cause: errors.New("request gone"), Kinds: verification.FailureJoinRequestGone}},
		{name: "abandoned", responseState: "approval_unconfirmed", actionState: "failed", approvalError: &verification.GatewayError{Cause: errors.New("applicant gone"), Kinds: verification.FailureApplicantGone}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gateway := &queueApprovalGateway{approvalError: tc.approvalError}
			service, db := queueApprovalService(t, gateway)
			server, cookies, csrf := apiTestServer(t, &apiTestAdminChecker{allowed: true}, service, nil)
			entries, err := service.ConsoleQueue(context.Background(), apiSettingsGroupID)
			if err != nil || len(entries) != 1 {
				t.Fatalf("queue entries=%+v error=%v", entries, err)
			}
			response := postSettlement(server, cookies, csrf, apiSettingsGroupID, entries[0].ID)
			var body queueResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusOK || string(body.Result.State) != tc.responseState || body.Result.Reason != nil || body.RemainingSeconds != nil {
				t.Fatalf("release response status=%d body=%s", response.Code, response.Body.String())
			}
			rows, err := db.Query(context.Background(), `SELECT state, attempts FROM pending_action WHERE kind='settle_approve'`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var state string
			var attempts int
			if !rows.Next() {
				t.Fatal("release did not create a durable settlement action")
			}
			if err := rows.Scan(&state, &attempts); err != nil {
				t.Fatal(err)
			}
			if state != tc.actionState || gateway.approves != 1 || attempts != tc.attempts {
				t.Fatalf("action state=%q attempts=%d approval calls=%d", state, attempts, gateway.approves)
			}
			t.Logf("POST queue -> status=%d body=%s action=%s attempts=%d", response.Code, response.Body.String(), state, attempts)
		})
	}
}

func queueApprovalService(t *testing.T, gateway verification.Gateway) (*verification.Service, *database.Database) {
	t.Helper()
	ctx := context.Background()
	directory := t.TempDir()
	db, err := database.Open(ctx, database.Config{StateDirectory: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := database.NewVerificationStore(db)
	if changed, err := state.InsertPending("", verification.PendingRecord{
		GroupID: apiSettingsGroupID, UserID: 42, Nonce: "release", Name: "Applicant",
		CreatedAt: time.Now().Unix(), Deadline: time.Now().Add(time.Hour).Unix(), Mode: "quiz", Lang: "en",
		QText: "Pick one", QOpts: []string{"one", "two"},
	}); err != nil || !changed {
		t.Fatal(fmt.Errorf("insert applicant: changed=%t error=%v", changed, err))
	}
	_, _, _, settingsService, _ := apiSettingsTestServer(t, true)
	service, err := verification.New(settingsService.store, gateway, state,
		&settings.Config{GroupIDs: []int64{apiSettingsGroupID}}, &i18n.Messages, nil, verification.Identity{}, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	return service, db
}

type queueLeaseLostGateway struct {
	queueApprovalGateway
	db *database.Database
}

func (g *queueLeaseLostGateway) ApproveJoin(ctx context.Context, _, _ int64) error {
	g.approves++
	if _, err := g.db.Exec(ctx, `UPDATE pending_action SET claim_owner='other-worker' WHERE kind='settle_approve' AND state='pending'`); err != nil {
		return err
	}
	return g.approvalError
}

func TestQueueReleaseReportsAbandonedApprovalAfterLeaseLoss(t *testing.T) {
	gateway := &queueLeaseLostGateway{
		queueApprovalGateway: queueApprovalGateway{approvalError: errors.New("temporary approval failure")},
	}
	service, db := queueApprovalService(t, gateway)
	gateway.db = db
	server, cookies, csrf := apiTestServer(t, &apiTestAdminChecker{allowed: true}, service, nil)
	entries, err := service.ConsoleQueue(context.Background(), apiSettingsGroupID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("queue entries=%+v error=%v", entries, err)
	}
	response := postSettlement(server, cookies, csrf, apiSettingsGroupID, entries[0].ID)
	var body queueResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || body.Result.State != "approval_unconfirmed" || gateway.approves != 1 {
		t.Fatalf("abandoned approval status=%d body=%s calls=%d", response.Code, response.Body.String(), gateway.approves)
	}
	var state, owner string
	var attempts int
	if err := db.RawDB.QueryRowContext(context.Background(),
		`SELECT state, attempts, claim_owner FROM pending_action WHERE kind='settle_approve'`).Scan(&state, &attempts, &owner); err != nil {
		t.Fatal(err)
	}
	if state != "pending" || attempts != 0 || owner != "other-worker" {
		t.Fatalf("lost-lease action state=%s attempts=%d owner=%s", state, attempts, owner)
	}
	t.Logf("console release after lease loss -> HTTP %d state=%s; action=%s attempts=%d owner=%s",
		response.Code, body.Result.State, state, attempts, owner)
}
