package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type webVerifier func(context.Context, string) verification.TurnstileOutcome

func (f webVerifier) Verify(ctx context.Context, response string) verification.TurnstileOutcome {
	return f(ctx, response)
}

func TestWebWrongAttemptsUseExistingTerminalBehavior(t *testing.T) {
	for _, mode := range []string{settings.ModePoW, settings.ModeCaptcha} {
		t.Run(mode, func(t *testing.T) {
			f := newWebFixture(t, mode)
			f.service.SetTurnstileVerifier(webVerifier(func(context.Context, string) verification.TurnstileOutcome { return verification.TurnstileFailed }))
			for attempt := range 3 {
				result, err := f.service.AnswerWeb(context.Background(), verification.ChallengeID(f.record.Ref()), verification.WebProof{Token: f.token, Nonce: "invalid"})
				if err != nil {
					t.Fatal(err)
				}
				want := verification.WebWrong
				if attempt == 2 {
					want = verification.WebReceived
				}
				if result.Outcome != want {
					t.Fatalf("attempt %d=%s, want %s", attempt+1, result.Outcome, want)
				}
				var tries int
				if err := f.db.QueryRow(context.Background(), "SELECT attempts FROM challenge").Scan(&tries); err != nil {
					t.Fatal(err)
				}
				if tries != attempt+1 {
					t.Fatalf("wrong proof charged %d tries, want %d", tries, attempt+1)
				}
			}
			var state, reason string
			if err := f.db.QueryRow(context.Background(), "SELECT state, reason FROM challenge").Scan(&state, &reason); err != nil {
				t.Fatal(err)
			}
			if state != "declined" || reason != "wrong_answer" || f.gateway.declines.Load() != 1 {
				t.Fatalf("terminal answer=%s/%s declines=%d", state, reason, f.gateway.declines.Load())
			}
		})
	}
}

func TestCaptchaUnavailablePoliciesAndConfigurationOverride(t *testing.T) {
	for _, outcome := range []verification.TurnstileOutcome{verification.TurnstileOutage, verification.TurnstileConfiguration} {
		for _, policy := range []string{settings.CaptchaFallback, settings.CaptchaApprove, settings.CaptchaDecline} {
			t.Run(string(outcome)+"/"+policy, func(t *testing.T) {
				f := newWebFixture(t, settings.ModeCaptcha)
				view, _ := f.settings.Settings(f.record.GroupID)
				next := view.Overrides()
				next.CaptchaUnavailable = &policy
				if _, err := f.settings.Update(f.record.GroupID, view.Revision(), next); err != nil {
					t.Fatal(err)
				}
				f.service.SetTurnstileVerifier(webVerifier(func(context.Context, string) verification.TurnstileOutcome { return outcome }))
				result, err := f.service.AnswerWeb(context.Background(), verification.ChallengeID(f.record.Ref()), verification.WebProof{Token: f.token, CaptchaResponse: "provider-proof"})
				if err != nil {
					t.Fatal(err)
				}
				configuration := outcome == verification.TurnstileConfiguration
				if result.OperatorAlert != configuration {
					t.Fatalf("operator alert=%v, want %v", result.OperatorAlert, configuration)
				}
				var state, kind string
				if err := f.db.QueryRow(context.Background(), "SELECT state, kind FROM challenge").Scan(&state, &kind); err != nil {
					t.Fatal(err)
				}
				assertCaptchaPolicy(t, f, policy, configuration, result.Outcome, state, kind)
			})
		}
	}
}

func assertCaptchaPolicy(t *testing.T, f *webFixture, policy string, configuration bool, outcome verification.WebAnswer, state, kind string) {
	t.Helper()
	if configuration || policy == settings.CaptchaFallback {
		if outcome != verification.WebFallback || state != "pending" || kind != "rule" || f.gateway.approvals.Load() != 0 {
			t.Fatalf("fallback=%s %s/%s approvals=%d", outcome, state, kind, f.gateway.approvals.Load())
		}
		return
	}
	expected := "declined"
	if policy == settings.CaptchaApprove {
		expected = "approved"
	}
	if state != expected || kind != "captcha" || outcome != verification.WebReceived {
		t.Fatalf("policy=%s state=%s kind=%s outcome=%s", policy, state, kind, outcome)
	}
}

func TestCaptchaRotationDuringProviderCallCannotClaim(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	id := verification.ChallengeID(f.record.Ref())
	f.service.SetTurnstileVerifier(webVerifier(func(ctx context.Context, _ string) verification.TurnstileOutcome {
		if _, _, err := f.service.IssueWebToken(ctx, id); err != nil {
			t.Fatal(err)
		}
		return verification.TurnstilePassed
	}))
	result, err := f.service.AnswerWeb(context.Background(), id, verification.WebProof{Token: f.token})
	if err != nil || result.Outcome != verification.WebSettled || f.gateway.approvals.Load() != 0 {
		t.Fatalf("rotated claim=%#v error=%v approvals=%d", result, err, f.gateway.approvals.Load())
	}
}
