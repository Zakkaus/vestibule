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

// Long enough to be a credential rather than a word: the process refuses a
// setup token short enough to guess, because whoever opens that link owns
// the instance.
const appSetupLinkToken = "app-setup-link-token-for-tests-only"

const replacementSetupToken = "a-different-setup-token-for-tests"

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

func TestOnlyRecordedSetupTokenOpensClaimPage(t *testing.T) {
	telegram, _ := newSetupTelegramAPI(t)
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
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("unclaimed Run error: %v", err)
		}
	})

	baseURL := "http://" + address
	waitForSetupHTTP(t, baseURL+"/setup/"+appSetupLinkToken, http.StatusOK)
	response, err := http.Get(baseURL + "/setup/unrecorded-setup-token")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unrecorded setup token exposed the instance-claim page: status=%d, want 404", response.StatusCode)
	}
}

func TestClaimedRestartRecoversStoredBotToken(t *testing.T) {
	telegram, calls := newSetupTelegramAPI(t)
	stateDirectory := t.TempDir()
	claimPath := filepath.Join(stateDirectory, claimStateFile)
	if err := os.WriteFile(claimPath, []byte(`{"bot_token":"`+appSetupBotToken+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	address := reserveSetupConsoleAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{
			ConfigPath:     filepath.Join(stateDirectory, "missing-config.json"),
			StateDirectory: stateDirectory,
			ConsoleAddr:    address,
			TelegramAPIURL: telegram.URL,
		})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("claimed Run error: %v", err)
		}
	})

	baseURL := "http://" + address
	waitForSetupHTTP(t, baseURL+"/livez", http.StatusOK)
	response, err := http.Get(baseURL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("claimed restart stayed unready; the bot token in claim.json was not recovered: status=%d, want 200", response.StatusCode)
	}
	if got := calls.Load(); got == 0 {
		t.Fatal("claimed restart became ready without reconnecting the bot token preserved in claim.json")
	}
}

func TestFailedClaimReopensSetupForValidBotToken(t *testing.T) {
	rejectedBotToken := "2:" + strings.Repeat("b", 35)
	telegram := newSetupTelegramAPIForBotToken(t, appSetupBotToken)
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
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("setup Run error: %v", err)
		}
	})

	baseURL := "http://" + address
	setupURL := baseURL + "/setup/" + appSetupLinkToken
	waitForSetupHTTP(t, setupURL, http.StatusOK)
	if response := postSetupForm(t, setupURL, rejectedBotToken); response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("rejected bot token claim status=%d, want 422", response.StatusCode)
	}
	retry, err := http.Get(setupURL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, retry.Body)
	_ = retry.Body.Close()
	if retry.StatusCode != http.StatusOK {
		t.Fatalf("failed bot-token claim closed setup permanently: retry status=%d, want 200", retry.StatusCode)
	}
	if response := postSetupForm(t, setupURL, appSetupBotToken); response.StatusCode != http.StatusOK {
		t.Fatalf("valid bot token could not claim the instance after a failed attempt: status=%d, want 200", response.StatusCode)
	}
	waitForSetupHTTP(t, baseURL+"/readyz", http.StatusOK)
}

func TestStartupRejectsReadableClaimState(t *testing.T) {
	stateDirectory := t.TempDir()
	claimPath := filepath.Join(stateDirectory, claimStateFile)
	if err := os.WriteFile(claimPath, []byte(`{"bot_token":"`+appSetupBotToken+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(claimPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openSetupState(stateDirectory, ""); err == nil {
		t.Fatal("startup accepted a readable claim state; another local account could copy the live bot token")
	}
	if err := os.Chmod(claimPath, 0o600); err != nil {
		t.Fatal(err)
	}
	state, _, err := openSetupState(stateDirectory, "")
	if err != nil {
		t.Fatalf("startup rejected a private claim state: %v", err)
	}
	if got := state.BotToken(); got != appSetupBotToken {
		t.Fatalf("private claim state bot token=%q, want preserved credential", got)
	}
}

func TestStartupRejectsMismatchedSetupToken(t *testing.T) {
	stateDirectory := t.TempDir()
	if _, _, err := openSetupState(stateDirectory, appSetupLinkToken); err != nil {
		t.Fatalf("start with the recorded setup token: %v", err)
	}
	if _, _, err := openSetupState(stateDirectory, replacementSetupToken); err == nil {
		t.Fatal("startup accepted a replacement SETUP_TOKEN while the old claim link remained active")
	}
	if _, _, err := openSetupState(stateDirectory, appSetupLinkToken); err != nil {
		t.Fatalf("startup rejected the setup token recorded in claim.json: %v", err)
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

func newSetupTelegramAPIForBotToken(t *testing.T, acceptedToken string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(request.URL.Path, "/getMe"):
			if !strings.Contains(request.URL.Path, "/bot"+acceptedToken+"/") {
				_, _ = io.WriteString(writer, `{"ok":false,"error_code":401,"description":"Unauthorized"}`)
				return
			}
			_, _ = io.WriteString(writer, `{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Test","username":"test_bot"}}`)
		case strings.HasSuffix(request.URL.Path, "/getUpdates"):
			time.Sleep(10 * time.Millisecond)
			_, _ = io.WriteString(writer, `{"ok":true,"result":[]}`)
		default:
			_, _ = io.WriteString(writer, `{"ok":true,"result":true}`)
		}
	}))
	t.Cleanup(server.Close)
	return server
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

// The claim link is the whole instance: whoever opens it hands the process a bot
// token and becomes its owner. Before this, an instance with no SETUP_TOKEN had
// no way in at all, which pushed operators into choosing the token by hand -- and
// a token chosen by hand is a token somebody else can guess.
func TestStartupGeneratesAClaimLinkNobodyCanGuess(t *testing.T) {
	first, firstLink, err := openSetupState(t.TempDir(), "")
	if err != nil {
		t.Fatalf("startup without a configured setup token failed: %v", err)
	}
	if !firstLink.Generated || firstLink.Token == "" {
		t.Fatal("startup left an unclaimed instance with no claim link, so nobody can claim it")
	}
	if len(firstLink.Token) < minimumSetupTokenLength {
		t.Fatalf("the generated claim token is %d characters, under the floor of %d",
			len(firstLink.Token), minimumSetupTokenLength)
	}
	if !first.setupAvailable(firstLink.Token) {
		t.Fatal("the generated token does not open the setup route it was generated for")
	}
	if first.setupAvailable(firstLink.Token + "x") {
		t.Fatal("a token that is not the generated one still opens the setup route")
	}

	_, secondLink, err := openSetupState(t.TempDir(), "")
	if err != nil {
		t.Fatalf("second startup failed: %v", err)
	}
	if secondLink.Token == firstLink.Token {
		t.Fatal("two instances generated the same claim token; it is not random")
	}
}

// A short token is the hole this closes, and refusing to start is the only
// answer that does not leave a guessable claim link listening.
func TestStartupRefusesAGuessableSetupToken(t *testing.T) {
	if _, _, err := openSetupState(t.TempDir(), "vestibule"); err == nil {
		t.Fatal("startup accepted a nine-character SETUP_TOKEN; anyone who guesses it owns the instance")
	}
	long := strings.Repeat("k", minimumSetupTokenLength)
	if _, _, err := openSetupState(t.TempDir(), long); err != nil {
		t.Fatalf("startup refused a token at the documented floor: %v", err)
	}
}
