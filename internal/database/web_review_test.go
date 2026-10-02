package database

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestCanceledCaptchaNeverAppliesUnavailablePolicy(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	view, _ := f.settings.Settings(f.record.GroupID)
	next, policy := view.Overrides(), settings.CaptchaApprove
	next.CaptchaUnavailable = &policy
	if _, err := f.settings.Update(f.record.GroupID, view.Revision(), next); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.service.SetTurnstileVerifier(webVerifier(func(context.Context, string) verification.TurnstileOutcome {
		cancel()
		return verification.TurnstileOutage
	}))
	_, err := f.service.AnswerWeb(ctx, verification.ChallengeID(f.record.Ref()), verification.WebProof{Token: f.token})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled answer error=%v", err)
	}
	records, err := f.state.LoadPending("")
	if err != nil || len(records) != 1 || records[0].Tries != 0 || f.gateway.approvals.Load() != 0 || f.gateway.declines.Load() != 0 {
		t.Fatalf("canceled request settled: records=%v error=%v approvals=%d declines=%d", records, err, f.gateway.approvals.Load(), f.gateway.declines.Load())
	}
}

func TestExpiryLeaseDoesNotReopenWebAnswerDeadline(t *testing.T) {
	f := newWebFixture(t, settings.ModePoW)
	ctx := context.Background()
	now := time.Now().Unix()
	records, err := f.state.LoadPending("")
	if err != nil || len(records) != 1 {
		t.Fatalf("pending=%v/%v", records, err)
	}
	expired := records[0]
	expired.Deadline = now
	if changed, err := f.state.UpdatePending("", expired.Ref(), expired); err != nil || !changed {
		t.Fatalf("expire=%v/%v", changed, err)
	}
	claimed, err := f.state.ClaimExpired("", now, now+30, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("lease=%v/%v", claimed, err)
	}
	if _, valid, err := f.state.ResolveWebToken(ctx, f.proof.TokenHash, now); err != nil || valid {
		t.Fatalf("expired token reopened: valid=%v error=%v", valid, err)
	}
	transition := verification.ChallengeTransition{Expected: claimed[0].Ref(), Record: claimed[0], From: verification.ChallengePending, To: verification.ChallengeApproved, SettledAt: now, WebClaim: &verification.WebClaim{TokenHash: f.proof.TokenHash, Now: now}}
	if changed, err := f.state.TransitionChallenge("", transition); err != nil || changed {
		t.Fatalf("expired answer claimed: %v/%v", changed, err)
	}
	if again, err := f.state.ClaimExpired("", now, now+30, 10); err != nil || len(again) != 0 {
		t.Fatalf("active lease reclaimed: %v/%v", again, err)
	}
	if again, err := f.state.ClaimExpired("", now+30, now+60, 10); err != nil || len(again) != 1 {
		t.Fatalf("crashed worker lease not reclaimed: %v/%v", again, err)
	}
}

func TestC5RejectedQuestionKeepsQuestionAndOptions(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	if changed, err := f.service.FallbackWeb(context.Background(), verification.ChallengeID(f.record.Ref())); err != nil || !changed {
		t.Fatalf("fallback=%v/%v", changed, err)
	}
	before, err := f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	f.gateway.sendErr = &verification.GatewayError{Code: 403, Kinds: verification.FailureBlockedByUser}
	f.service.SendDMChallenge(context.Background(), f.record.UserID, "en", f.record.GroupID)
	after, err := f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || !after[0].FallbackPending || after[0].QText != before[0].QText || !reflect.DeepEqual(after[0].QOpts, before[0].QOpts) || after[0].CorrectIdx != before[0].CorrectIdx {
		t.Fatalf("C5 question corrupted after rejected DM: before=%v after=%v", before, after)
	}
}

func TestRestoredC5DeliveryStartsFullAnswerWindow(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	f.service.Shutdown()
	f.restoreWithCapabilities(t, settings.WebCapabilities{ConsoleURL: "https://console.example"})
	records, err := f.state.LoadPending("")
	if err != nil || len(records) != 1 {
		t.Fatalf("restored=%v/%v", records, err)
	}
	view, _ := f.settings.Settings(f.record.GroupID)
	minimum := time.Now().Unix() + int64(view.TimeoutSeconds().Value) - 2
	if records[0].FallbackPending || records[0].Deadline < minimum {
		t.Fatalf("delivered C5 window=%v, minimum deadline=%d", records[0], minimum)
	}
}

func TestIssueWebTokenBindsChallengeApplicant(t *testing.T) {
	f := newWebFixture(t, settings.ModePoW)
	if _, err := f.db.Exec(context.Background(), "UPDATE challenge SET user_id=user_id+1 WHERE id=$1", verification.ChallengeID(f.record.Ref())); err != nil {
		t.Fatal(err)
	}
	if raw, _, err := f.service.IssueWebToken(context.Background(), verification.ChallengeID(f.record.Ref())); err != nil || raw != "" {
		t.Fatalf("issued token for mismatched applicant: raw=%q error=%v", raw, err)
	}
}
