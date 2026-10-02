package verification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

type turnstileRoundTrip func(*http.Request) (*http.Response, error)

func (f turnstileRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTurnstileOutcomeClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		transport error
		want      TurnstileOutcome
	}{
		{"success", 200, `{"success":true,"hostname":"console.example","action":"verify"}`, nil, TurnstilePassed},
		{"hostname", 200, `{"success":true,"hostname":"elsewhere.example","action":"verify"}`, nil, TurnstileFailed},
		{"action", 200, `{"success":true,"hostname":"console.example","action":"login"}`, nil, TurnstileFailed},
		{"invalid", 200, `{"success":false,"error-codes":["invalid-input-response"]}`, nil, TurnstileFailed},
		{"missing", 200, `{"success":false,"error-codes":["missing-input-response"]}`, nil, TurnstileFailed},
		{"duplicate", 200, `{"success":false,"error-codes":["timeout-or-duplicate"]}`, nil, TurnstileFailed},
		{"unknown", 200, `{"success":false,"error-codes":["future-error"]}`, nil, TurnstileFailed},
		{"malformed", 200, `{`, nil, TurnstileFailed},
		{"HTTP 4xx", 400, `{}`, nil, TurnstileFailed},
		{"HTTP 5xx", 503, `{}`, nil, TurnstileOutage},
		{"internal", 200, `{"success":false,"error-codes":["internal-error"]}`, nil, TurnstileOutage},
		{"transport", 0, "", errors.New("transport failure"), TurnstileOutage},
		{"timeout", 0, "", context.DeadlineExceeded, TurnstileOutage},
		{"missing secret", 200, `{"success":false,"error-codes":["missing-input-secret"]}`, nil, TurnstileConfiguration},
		{"invalid secret", 200, `{"success":false,"error-codes":["invalid-input-secret"]}`, nil, TurnstileConfiguration},
		{"bad request", 400, `{"success":false,"error-codes":["bad-request"]}`, nil, TurnstileConfiguration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ids []string
			transport := turnstileRoundTrip(func(r *http.Request) (*http.Response, error) {
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("secret") != "test-secret" || r.Form.Get("response") != "proof" {
					t.Fatal("incorrect siteverify form")
				}
				if r.Form.Has("remoteip") || r.Form.Has("cdata") {
					t.Fatal("unexpected identity metadata")
				}
				id := r.Form.Get("idempotency_key")
				if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(id) {
					t.Fatalf("non-UUIDv4 key %q", id)
				}
				ids = append(ids, id)
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Fatal("missing provider timeout")
				}
				if tc.transport != nil {
					return nil, tc.transport
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			client, err := NewTurnstileClient("test-secret", "https://console.example:8443", &http.Client{Transport: transport})
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if got := client.Verify(context.Background(), "proof"); got != tc.want {
					t.Fatalf("outcome=%s, want %s", got, tc.want)
				}
			}
			if ids[0] == ids[1] {
				t.Fatal("idempotency key reused between submissions")
			}
		})
	}
}

func TestTurnstileCallerCancellationIsNotProviderOutage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := NewTurnstileClient("test-secret", "https://console.example", &http.Client{
		Transport: turnstileRoundTrip(func(*http.Request) (*http.Response, error) {
			cancel()
			return nil, context.Canceled
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := client.Verify(ctx, "proof"); got != TurnstileOutcome("canceled") {
		t.Fatalf("caller cancellation classified as %s", got)
	}
}

type timeoutTurnstileBody struct{}

func (timeoutTurnstileBody) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }
func (timeoutTurnstileBody) Close() error             { return nil }

func TestTurnstileBodyTimeoutAndCanonicalHostname(t *testing.T) {
	for _, test := range []struct {
		name string
		body io.ReadCloser
		want TurnstileOutcome
	}{
		{"body timeout", timeoutTurnstileBody{}, TurnstileOutage},
		{"canonical hostname", io.NopCloser(strings.NewReader(`{"success":true,"hostname":"CONSOLE.EXAMPLE.","action":"verify"}`)), TurnstilePassed},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewTurnstileClient("test-secret", "https://Console.Example.:8443", &http.Client{
				Transport: turnstileRoundTrip(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: test.body, Header: make(http.Header)}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := client.Verify(context.Background(), "proof"); got != test.want {
				t.Fatalf("outcome=%s, want %s", got, test.want)
			}
		})
	}
}

type interruptedTurnstileBody struct {
	err error
}

func (b interruptedTurnstileBody) Read([]byte) (int, error) { return 0, b.err }
func (interruptedTurnstileBody) Close() error               { return nil }

func TestTurnstileBodyTransportFailureDiffersFromMalformedJSONAndCancellation(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   io.ReadCloser
		cancel bool
		want   TurnstileOutcome
	}{
		{"connection reset", interruptedTurnstileBody{errors.New("connection reset by peer")}, false, TurnstileOutage},
		{"truncated transfer", interruptedTurnstileBody{io.ErrUnexpectedEOF}, false, TurnstileOutage},
		{"malformed JSON", io.NopCloser(strings.NewReader(`{"success":`)), false, TurnstileFailed},
		{"empty JSON", io.NopCloser(strings.NewReader("")), false, TurnstileFailed},
		{"caller canceled", interruptedTurnstileBody{context.Canceled}, true, TurnstileCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, err := NewTurnstileClient("test-secret", "https://console.example", &http.Client{
				Transport: turnstileRoundTrip(func(*http.Request) (*http.Response, error) {
					if test.cancel {
						cancel()
					}
					return &http.Response{StatusCode: http.StatusOK, Body: test.body, Header: make(http.Header)}, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := client.Verify(ctx, "proof"); got != test.want {
				t.Fatalf("outcome=%s, want %s", got, test.want)
			}
		})
	}
}
