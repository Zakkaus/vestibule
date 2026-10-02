package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type apiCaptchaVerifier verification.TurnstileOutcome

func (v apiCaptchaVerifier) Verify(context.Context, string) verification.TurnstileOutcome {
	return verification.TurnstileOutcome(v)
}

func TestWebCaptchaOutageUsesGroupPolicyAndDeliversFallback(t *testing.T) {
	for _, policy := range []string{settings.CaptchaFallback, settings.CaptchaApprove, settings.CaptchaDecline} {
		t.Run(policy, func(t *testing.T) {
			f := newAPIWebFixture(t, settings.ModeCaptcha, "zh-CN")
			group, _ := f.settings.Settings(apiWebChatID)
			if _, err := f.settings.Update(apiWebChatID, group.Revision(), settings.GroupOverrides{CaptchaUnavailable: &policy}); err != nil {
				t.Fatal(err)
			}
			f.service.SetTurnstileVerifier(apiCaptchaVerifier(verification.TurnstileOutage))
			response := f.request(http.MethodPost, f.raw, "cf-turnstile-response=fake")
			if response.Code != http.StatusOK {
				t.Fatalf("response=%d", response.Code)
			}
			records, err := f.store.LoadPending("")
			if err != nil {
				t.Fatal(err)
			}
			switch policy {
			case settings.CaptchaFallback:
				requireWebQuizFallback(t, f, records)
			case settings.CaptchaApprove:
				if len(records) != 0 || f.gateway.approvals.Load() != 1 || f.gateway.declines.Load() != 0 {
					t.Fatal("approve policy was not applied once")
				}
			case settings.CaptchaDecline:
				if len(records) != 0 || f.gateway.approvals.Load() != 0 || f.gateway.declines.Load() != 1 {
					t.Fatal("decline policy was not applied once")
				}
			}
		})
	}
}

func TestWebCaptchaConfigurationFailureFallsBackAndAlerts(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModeCaptcha, "en")
	policy := settings.CaptchaApprove
	view, _ := f.settings.Settings(apiWebChatID)
	if _, err := f.settings.Update(apiWebChatID, view.Revision(), settings.GroupOverrides{CaptchaUnavailable: &policy}); err != nil {
		t.Fatal(err)
	}
	f.service.SetTurnstileVerifier(apiCaptchaVerifier(verification.TurnstileConfiguration))
	alerts := 0
	f.server.ReplaceRoutes(Config{WebVerification: f.service, ConsoleURL: "https://console.example", WebOperatorAlert: func(_ context.Context, group int64) {
		if group != apiWebChatID {
			t.Fatalf("alert group=%d", group)
		}
		alerts++
	}})
	f.request(http.MethodPost, f.raw, "cf-turnstile-response=fake")
	records, err := f.store.LoadPending("")
	if err != nil || len(records) != 1 || records[0].Mode != settings.ModeQuiz || alerts != 1 || f.gateway.approvals.Load() != 0 {
		t.Fatalf("configuration failure applied approve policy: alerts=%d records=%+v err=%v", alerts, records, err)
	}
}

func TestWebCaptchaFailedProofChargesAttemptAndRendersFreshWidget(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModeCaptcha, "ru")
	f.service.SetTurnstileVerifier(apiCaptchaVerifier(verification.TurnstileFailed))
	response := f.request(http.MethodPost, f.raw, "cf-turnstile-response=never-reflect-this")
	record, _, valid, err := f.service.ResolveWebToken(context.Background(), f.raw)
	if err != nil || !valid || record.Tries != 1 || f.gateway.approvals.Load() != 0 {
		t.Fatalf("failed captcha tries=%d valid=%v err=%v", record.Tries, valid, err)
	}
	if !strings.Contains(response.Body.String(), `class="cf-turnstile"`) || strings.Contains(response.Body.String(), "never-reflect-this") || !strings.Contains(response.Header().Get("Content-Security-Policy"), "frame-src https://challenges.cloudflare.com") {
		t.Fatal("failed response reused the provider response or omitted a fresh widget")
	}
}

func requireWebQuizFallback(t *testing.T, f *apiWebFixture, records []verification.PendingRecord) {
	t.Helper()
	view, _ := f.settings.Settings(apiWebChatID)
	if len(records) != 1 || records[0].Mode != settings.ModeQuiz || records[0].FallbackPending || records[0].Deadline < time.Now().Unix()+int64(view.TimeoutSeconds().Value)-2 {
		t.Fatalf("C5 did not deliver with a full answer window: %+v", records)
	}
	if f.gateway.approvals.Load() != 0 || f.gateway.declines.Load() != 0 {
		t.Fatal("fallback settled instead of asking a question")
	}
}
