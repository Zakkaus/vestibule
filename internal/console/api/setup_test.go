package api

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/status"
)

const (
	setupTestLinkToken = "setup-test-link-token"
	setupTestBotToken  = "123456:setup-test-bot-token"
)

type setupTestService struct {
	linkToken string
	result    SetupResult
	err       error
	claims    []string
}

func (s *setupTestService) SetupAvailable(token string) bool {
	return token == s.linkToken
}

func (s *setupTestService) Claim(_ context.Context, token string) (SetupResult, error) {
	s.claims = append(s.claims, token)
	if s.err != nil {
		return SetupResult{}, s.err
	}
	return s.result, nil
}

func newClaimableSetupServer(config Config) *Server {
	server := New(config)
	config.SetupClaimed = func() { server.ReplaceRoutes(Config{}) }
	server.ReplaceRoutes(config)
	return server
}

func TestSetupServesUnclaimedHealth(t *testing.T) {
	health := status.NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	service := &setupTestService{linkToken: setupTestLinkToken}
	server := newClaimableSetupServer(Config{Health: health, Setup: service})

	live := getPath(server, "/livez")
	ready := getPath(server, "/readyz")
	setup := getPath(server, "/setup/"+setupTestLinkToken)
	if live.Code != http.StatusOK || ready.Code != http.StatusServiceUnavailable || setup.Code != http.StatusOK {
		t.Fatalf("unclaimed responses livez=%d readyz=%d setup=%d, want 200, 503, 200", live.Code, ready.Code, setup.Code)
	}
	if strings.Contains(setup.Body.String(), setupTestLinkToken) {
		t.Fatal("setup page reflected its one-time link token")
	}
	if got := setup.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("setup Referrer-Policy=%q, want no-referrer", got)
	}
}

func TestSetupClaimsOnlyOnce(t *testing.T) {
	service := &setupTestService{
		linkToken: setupTestLinkToken,
		result:    SetupResult{Claimed: true, BindingURL: "https://t.me/test_bot?start=owner_test"},
	}
	server := newClaimableSetupServer(Config{Setup: service})

	first := postSetup(server, setupTestLinkToken, setupTestBotToken)
	second := postSetup(server, setupTestLinkToken, setupTestBotToken)
	if first.Code != http.StatusOK || second.Code != http.StatusNotFound || len(service.claims) != 1 {
		t.Fatalf("claim responses first=%d second=%d claims=%d, want 200, 404, 1", first.Code, second.Code, len(service.claims))
	}
}

func TestSetupRouteDisappearsAfterClaim(t *testing.T) {
	service := &setupTestService{linkToken: setupTestLinkToken, result: SetupResult{Claimed: true}}
	server := newClaimableSetupServer(Config{Setup: service})

	if response := postSetup(server, setupTestLinkToken, setupTestBotToken); response.Code != http.StatusOK {
		t.Fatalf("claim status=%d, want 200", response.Code)
	}
	afterClaim := getPath(server, "/setup/"+setupTestLinkToken)
	if afterClaim.Code != http.StatusNotFound || decodeError(afterClaim) != "not_found" {
		t.Fatalf("setup after claim status=%d error=%q, want 404/not_found", afterClaim.Code, decodeError(afterClaim))
	}
}

func TestSetupDoesNotReflectCredentialInResponseOrLogs(t *testing.T) {
	credential := "123456:setup-sensitive-test-token"
	service := &setupTestService{
		linkToken: setupTestLinkToken,
		err:       errors.New("Telegram rejected " + credential),
	}
	server := newClaimableSetupServer(Config{Setup: service})
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	response := postSetup(server, setupTestLinkToken, credential)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("claim failure status=%d, want 422", response.Code)
	}
	if strings.Contains(response.Body.String(), credential) || strings.Contains(logs.String(), credential) {
		t.Fatal("setup credential appeared in an HTTP response or log")
	}
}

func postSetup(server *Server, linkToken, botToken string) *httptest.ResponseRecorder {
	form := url.Values{"bot_token": {botToken}}
	request := httptest.NewRequest(http.MethodPost, "/setup/"+linkToken, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	return response
}
