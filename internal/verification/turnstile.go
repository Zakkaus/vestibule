package verification

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TurnstileOutcome string

const (
	TurnstilePassed        TurnstileOutcome = "passed"
	TurnstileFailed        TurnstileOutcome = "failed"
	TurnstileOutage        TurnstileOutcome = "outage"
	TurnstileConfiguration TurnstileOutcome = "configuration"
	TurnstileCanceled      TurnstileOutcome = "canceled"
)

// TurnstileVerifier performs provider I/O outside any challenge transaction.
type TurnstileVerifier interface {
	Verify(context.Context, string) TurnstileOutcome
}

type TurnstileClient struct {
	secret, hostname string
	client           *http.Client
}
type turnstileResponse struct {
	Success  bool     `json:"success"`
	Hostname string   `json:"hostname"`
	Action   string   `json:"action"`
	Errors   []string `json:"error-codes"`
}

func NewTurnstileClient(secret, consoleURL string, client *http.Client) (*TurnstileClient, error) {
	u, err := url.Parse(consoleURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid console URL")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &TurnstileClient{secret: secret, hostname: canonicalHostname(u.Hostname()), client: client}, nil
}

func (c *TurnstileClient) Verify(ctx context.Context, response string) TurnstileOutcome {
	caller := ctx
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return TurnstileOutage
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	key := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	form := url.Values{"secret": {c.secret}, "response": {response}, "idempotency_key": {key}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return TurnstileOutage
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	result, err := c.client.Do(req)
	if err != nil {
		if caller.Err() != nil {
			return TurnstileCanceled
		}
		return TurnstileOutage
	}
	defer result.Body.Close()
	if result.StatusCode >= 500 {
		return TurnstileOutage
	}
	var payload turnstileResponse
	body := turnstileBodyReader{Reader: io.LimitReader(result.Body, 65536)}
	err = json.NewDecoder(&body).Decode(&payload)
	if caller.Err() != nil {
		return TurnstileCanceled
	}
	if body.err != nil || ctx.Err() != nil {
		return TurnstileOutage
	}
	if err != nil {
		return TurnstileFailed
	}
	return classifyTurnstile(result.StatusCode, payload, c.hostname)
}

type turnstileBodyReader struct {
	io.Reader
	err error
}

func (r *turnstileBodyReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		r.err = err
	}
	return n, err
}

func canonicalHostname(host string) string {
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

func classifyTurnstile(status int, response turnstileResponse, hostname string) TurnstileOutcome {
	for _, code := range response.Errors {
		switch code {
		case "missing-input-secret", "invalid-input-secret", "bad-request":
			return TurnstileConfiguration
		}
	}
	for _, code := range response.Errors {
		if code == "internal-error" {
			return TurnstileOutage
		}
	}
	if status < 200 || status >= 300 || !response.Success || len(response.Errors) != 0 || canonicalHostname(response.Hostname) != hostname || response.Action != "verify" {
		return TurnstileFailed
	}
	return TurnstilePassed
}
