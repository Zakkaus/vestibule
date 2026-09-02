package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const appSetupLinkToken = "app-setup-link-token"

var appSetupBotToken = "1:" + strings.Repeat("a", 35)

func TestRunStartsUnclaimedHTTPWithoutTelegram(t *testing.T) {
	telegram, calls := newSetupTelegramAPI(t)
	address := reserveSetupConsoleAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ConfigPath:     filepath.Join(t.TempDir(), "missing-config.json"),
			StateDirectory: t.TempDir(),
			ConsoleAddr:    address,
			SetupToken:     appSetupLinkToken,
			TelegramAPIURL: telegram.URL,
		})
	}()

	baseURL := "http://" + address
	live := waitForSetupHTTP(t, baseURL+"/livez", http.StatusOK)
	ready := waitForSetupHTTP(t, baseURL+"/readyz", http.StatusServiceUnavailable)
	setup := waitForSetupHTTP(t, baseURL+"/setup/"+appSetupLinkToken, http.StatusOK)
	if live.StatusCode != http.StatusOK || ready.StatusCode != http.StatusServiceUnavailable || setup.StatusCode != http.StatusOK {
		t.Fatalf("unclaimed responses livez=%d readyz=%d setup=%d", live.StatusCode, ready.StatusCode, setup.StatusCode)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("unclaimed startup contacted Telegram %d times", got)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("unclaimed Run error: %v", err)
	}
}

func TestRunClaimsWithoutRestartAndRemovesSetupRoute(t *testing.T) {
	telegram, calls := newSetupTelegramAPI(t)
	stateDirectory := t.TempDir()
	address := reserveSetupConsoleAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ConfigPath:     filepath.Join(stateDirectory, "missing-config.json"),
			StateDirectory: stateDirectory,
			ConsoleAddr:    address,
			SetupToken:     appSetupLinkToken,
			TelegramAPIURL: telegram.URL,
		})
	}()
	baseURL := "http://" + address
	waitForSetupHTTP(t, baseURL+"/setup/"+appSetupLinkToken, http.StatusOK)

	claim := postSetupForm(t, baseURL+"/setup/"+appSetupLinkToken, appSetupBotToken)
	if claim.StatusCode != http.StatusOK {
		t.Fatalf("claim status=%d, want 200", claim.StatusCode)
	}
	waitForSetupHTTP(t, baseURL+"/readyz", http.StatusOK)
	afterClaim := waitForSetupHTTP(t, baseURL+"/setup/"+appSetupLinkToken, http.StatusNotFound)
	if afterClaim.StatusCode != http.StatusNotFound {
		t.Fatalf("setup route after claim=%d, want 404", afterClaim.StatusCode)
	}
	select {
	case err := <-done:
		t.Fatalf("process exited during in-place claim: %v", err)
	default:
	}
	if got := calls.Load(); got == 0 {
		t.Fatal("claim did not establish a Telegram channel")
	}
	claimState := filepath.Join(stateDirectory, claimStateFile)
	info, err := os.Stat(claimState)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("claim state mode=%04o, want 0600", got)
	}
	contents, err := os.ReadFile(claimState)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), appSetupBotToken) || strings.Contains(string(contents), appSetupLinkToken) {
		t.Fatal("claim state did not retain only the bot credential")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("claimed Run error: %v", err)
	}
}

func newSetupTelegramAPI(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.Path, "/getMe"):
			_, _ = io.WriteString(writer, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
		case strings.HasSuffix(request.URL.Path, "/getUpdates"):
			time.Sleep(10 * time.Millisecond)
			_, _ = io.WriteString(writer, `{"ok":true,"result":[]}`)
		default:
			_, _ = io.WriteString(writer, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func reserveSetupConsoleAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func waitForSetupHTTP(t *testing.T, endpoint string, wantStatus int) *http.Response {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get(endpoint)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if response.StatusCode == wantStatus {
				return response
			}
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("GET %s did not respond with %d: %v", endpoint, wantStatus, err)
			}
			t.Fatalf("GET %s status=%d, want %d", endpoint, response.StatusCode, wantStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func postSetupForm(t *testing.T, endpoint, botToken string) *http.Response {
	t.Helper()
	form := url.Values{"bot_token": {botToken}}
	response, err := http.Post(endpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return response
}
