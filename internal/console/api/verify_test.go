package api

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestWebGETIsPublicNonConsumingAndLocalized(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "ja")
	first, second := f.request(http.MethodGet, f.raw, ""), f.request(http.MethodGet, f.raw, "")
	if first.Code != http.StatusOK || first.Body.String() != second.Body.String() || !strings.Contains(first.Body.String(), `lang="ja"`) {
		t.Fatal("reload changed the challenge or lost the applicant locale")
	}
	record, token, valid, err := f.service.ResolveWebToken(context.Background(), f.raw)
	if err != nil || !valid || record.Tries != 0 || token.TokenHash != f.token.TokenHash || token.Salt != f.token.Salt {
		t.Fatalf("GET mutated challenge: valid=%v tries=%d err=%v", valid, record.Tries, err)
	}
	if first.Header().Get("Referrer-Policy") != "no-referrer" || first.Header().Get("Cache-Control") != "no-store" || strings.Contains(first.Header().Get("Content-Security-Policy"), "https:") {
		t.Fatal("PoW page permits external assets or leaks referrers")
	}
}

func TestWebConcurrentPOSTHasOneWinnerAndLaterPOSTIsSettled(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "zh-CN")
	body := "nonce=" + apiWebNonce(t, f.token)
	replies := make([]*httptest.ResponseRecorder, 2)
	var wait sync.WaitGroup
	for index := range replies {
		wait.Add(1)
		go func() { defer wait.Done(); replies[index] = f.request(http.MethodPost, f.raw, body) }()
	}
	wait.Wait()
	terminal := f.request(http.MethodGet, strings.Repeat("0", 32), "")
	settled := 0
	for _, reply := range replies {
		if bytes.Equal(reply.Body.Bytes(), terminal.Body.Bytes()) {
			settled++
		}
	}
	if settled != 1 || f.gateway.approvals.Load() != 1 {
		t.Fatalf("settled replies=%d approvals=%d", settled, f.gateway.approvals.Load())
	}
	later := f.request(http.MethodPost, f.raw, body)
	if later.Code != terminal.Code || !bytes.Equal(later.Body.Bytes(), terminal.Body.Bytes()) {
		t.Fatal("later POST did not report the real settled post-condition")
	}
}

func TestWebUnavailableTokenStatesAreByteIdentical(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "ru")
	unknown := f.request(http.MethodGet, strings.Repeat("0", 32), "")
	used := f.raw
	var err error
	f.raw, _, err = f.service.IssueWebToken(context.Background(), f.token.ChallengeID)
	if err != nil {
		t.Fatal(err)
	}
	rotated := f.request(http.MethodGet, used, "")
	records, err := f.store.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	record := records[0]
	record.Deadline = time.Now().Unix()
	if updated, err := f.store.UpdatePending("", record.Ref(), record); err != nil || !updated {
		t.Fatalf("expiry=%v/%v", updated, err)
	}
	expired := f.request(http.MethodGet, f.raw, "")
	settledFixture := newAPIWebFixture(t, settings.ModePoW, "ja")
	settledFixture.request(http.MethodPost, settledFixture.raw, "nonce="+apiWebNonce(t, settledFixture.token))
	settled := settledFixture.request(http.MethodGet, settledFixture.raw, "")
	for _, response := range []*httptest.ResponseRecorder{rotated, expired, settled} {
		if response.Code != unknown.Code || !bytes.Equal(response.Body.Bytes(), unknown.Body.Bytes()) || !reflect.DeepEqual(response.Header(), unknown.Header()) {
			t.Fatal("unavailable token state disclosed a different response")
		}
	}
}

func TestWebPOSTAdmissionRejectsWithoutChargingAttempts(t *testing.T) {
	cases := []struct {
		name, contentType, site, origin, body string
		status                                int
	}{
		{"json", "application/json", "same-origin", "", `{}`, http.StatusUnsupportedMediaType},
		{"oversize", "application/x-www-form-urlencoded", "same-origin", "", "nonce=" + strings.Repeat("1", 4097), http.StatusRequestEntityTooLarge},
		{"missing-origin", "application/x-www-form-urlencoded", "", "", "nonce=0", http.StatusForbidden},
		{"cross-site", "application/x-www-form-urlencoded", "cross-site", "https://evil.example", "nonce=0", http.StatusForbidden},
		{"conflicting-origin", "application/x-www-form-urlencoded", "same-origin", "https://evil.example", "nonce=0", http.StatusForbidden},
	}
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	for _, row := range cases {
		t.Run(row.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/verify/"+f.raw, strings.NewReader(row.body))
			request.Header.Set("Content-Type", row.contentType)
			request.Header.Set("Sec-Fetch-Site", row.site)
			request.Header.Set("Origin", row.origin)
			response := httptest.NewRecorder()
			f.server.Handler().ServeHTTP(response, request)
			if response.Code != row.status {
				t.Fatalf("status=%d want %d", response.Code, row.status)
			}
		})
	}
	record, _, valid, err := f.service.ResolveWebToken(context.Background(), f.raw)
	if err != nil || !valid || record.Tries != 0 {
		t.Fatalf("admission charged tries=%d valid=%v err=%v", record.Tries, valid, err)
	}
	request := httptest.NewRequest(http.MethodPost, "/verify/"+f.raw, strings.NewReader("nonce="+apiWebNonce(t, f.token)))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	request.Header.Set("Origin", "https://console.example")
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || f.gateway.approvals.Load() != 1 {
		t.Fatal("same-origin fallback header did not admit a valid proof")
	}
}

func TestWebWrongProofExhaustsExistingAttemptLimit(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	for range 3 {
		f.request(http.MethodPost, f.raw, "nonce=invalid")
	}
	if f.gateway.approvals.Load() != 0 || f.gateway.declines.Load() != 1 {
		t.Fatal("invalid proof bypassed existing attempt exhaustion")
	}
	records, err := f.store.LoadPending("")
	if err != nil || len(records) != 0 {
		t.Fatalf("exhausted challenge remains pending: %v", err)
	}
}

func TestWebRequestLogsRedactBearerAndProof(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	var output bytes.Buffer
	server := New(Config{WebVerification: f.service, ConsoleURL: "https://console.example", RequestLog: log.New(&output, "", 0)})
	proof := "private-proof-value"
	request := httptest.NewRequest(http.MethodPost, "/verify/"+f.raw+"?nonce="+proof, strings.NewReader(url.Values{"cf-turnstile-response": {proof}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	server.Handler().ServeHTTP(httptest.NewRecorder(), request)
	if strings.Contains(output.String(), f.raw) || strings.Contains(output.String(), proof) || !strings.Contains(output.String(), "/verify/[redacted]") {
		t.Fatalf("request log failed redaction: %s", output.String())
	}
}

func TestWebNoReferrerFormAcceptsOpaqueOriginOnlyWithSameOriginMetadata(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	body := "nonce=" + apiWebNonce(t, f.token)
	request := httptest.NewRequest(http.MethodPost, "/verify/"+f.raw, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "null")
	response := httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || f.gateway.approvals.Load() != 0 {
		t.Fatal("opaque Origin alone admitted a request")
	}
	request = httptest.NewRequest(http.MethodPost, "/verify/"+f.raw, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "null")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response = httptest.NewRecorder()
	f.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || f.gateway.approvals.Load() != 1 {
		t.Fatal("no-referrer same-origin form could not submit")
	}
}
