package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// cmd/bot had no test of any kind, and everything the package does is a promise to whatever starts
// it. scripts/check-binary-contract.py compares the names — the flag the unit passes, the
// environment the unit and the compose file set, the signal systemd sends. These two drive the
// behaviour that only a running process shows.
func buildBot(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "vestibule")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building cmd/bot: %v\n%s", err, out)
	}
	return binary
}

func freeAddress(t *testing.T) string {
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

func TestVersionPrintsAndExitsWithoutStarting(t *testing.T) {
	out, err := exec.Command(buildBot(t), "-version").CombinedOutput()
	if err != nil {
		t.Fatalf("-version exited %v, want success: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != version {
		t.Fatalf("-version printed %q, want %q", strings.TrimSpace(string(out)), version)
	}
}

// An instance with no token waits to be claimed rather than exiting, which is what makes a fresh
// install possible: the operator claims it from the console afterwards. That wait is exactly where
// stopping the service has to keep working. systemd sends SIGTERM and kills at TimeoutStopSec=30s,
// so an unhandled signal leaves an operator watching `systemctl stop` hang for half a minute and
// the process killed outright, with whatever it was doing cut off rather than shut down.
//
// The first version of this test asserted that a missing configuration exits non-zero. It does not
// — it waits, deliberately — and the test hung for its whole timeout. The property was invented
// before reading internal/app/app.go:154.
func TestAnUnclaimedInstanceStopsOnTheSignalSystemdSends(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "config.json")
	if err := os.WriteFile(config, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(buildBot(t), "--config", config)
	command.Env = append(os.Environ(),
		"STATE_DIRECTORY="+directory, "BOT_TOKEN=", "SETUP_TOKEN=", "NOTIFY_SOCKET=",
		// The console's default address is a fixed port, and a developer machine may already be
		// using it. Take a free one so the test measures the signal and not the port.
		"CONSOLE_ADDR="+freeAddress(t))
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()

	select {
	case err := <-exited:
		t.Fatalf("the process ended before it was signalled: %v", err)
	case <-time.After(2 * time.Second):
	}
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("sending SIGTERM: %v", err)
	}
	select {
	case err := <-exited:
		// Exiting is not enough: with no handler installed the kernel's default action for
		// SIGTERM also ends the process, so a test that only waits for it to stop passes either
		// way. What separates the two is how it ended -- shut down, or terminated by the signal.
		if err != nil {
			t.Fatalf("the process was ended by the signal rather than shutting down: %v", err)
		}
	case <-time.After(25 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("still running 25 seconds after SIGTERM; systemd would kill it at TimeoutStopSec")
	}
}

// A startup that failed has to exit non-zero. Restart=always brings the process back either way,
// but `systemctl start` reports the result of the first attempt, and a deployment health gate that
// reads a clean exit concludes the bot came up when it never did.
func TestAStartupFailureExitsNonZero(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "config.json")
	if err := os.WriteFile(config, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(buildBot(t), "--config", config)
	command.Env = append(os.Environ(),
		"STATE_DIRECTORY="+directory, "BOT_TOKEN=", "SETUP_TOKEN=", "NOTIFY_SOCKET=",
		"CONSOLE_ADDR="+freeAddress(t),
		// A driver name no dialect answers to. The failure is deterministic, needs no network,
		// and happens while the service graph is being assembled -- which is where a real
		// misconfigured deployment fails too.
		"VT_DATABASE_TYPE=not-a-dialect")
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("the process exited successfully after failing to start:\n%s", out)
	}
	if !strings.Contains(string(out), "not-a-dialect") {
		t.Errorf("output does not say what failed:\n%s", out)
	}
	// TestVersionPrintsAndExitsWithoutStarting is the control: a run that does what it was asked
	// exits zero through the same main.
}

// syntheticToken has the shape telego requires and the shape the redacting writer looks for. It is
// not a real credential; the digits and the suffix are made up.
const syntheticToken = "9999999999:AAFnotarealtokennotarealtoken000000"

// Every Telegram client error carries the API URL, and the URL carries the token, so an ordinary
// log.Printf("...: %v", err) prints the credential. The unit runs under DynamicUser, whose journal
// is readable by anyone in systemd-journal, and the token is enough to take the bot over. The
// guard is the writer the process installs before anything can log, so no call site has to
// remember -- and nothing checked that cmd/bot still installs it.
func TestTheBotTokenNeverReachesTheLog(t *testing.T) {
	directory := t.TempDir()
	config := filepath.Join(directory, "config.json")
	if err := os.WriteFile(config, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(buildBot(t), "--config", config)
	command.Env = append(os.Environ(),
		"STATE_DIRECTORY="+directory, "BOT_TOKEN="+syntheticToken, "SETUP_TOKEN=", "NOTIFY_SOCKET=",
		"CONSOLE_ADDR="+freeAddress(t),
		// Port 1 refuses immediately, so the first call the bot makes fails with an error that
		// quotes the request URL -- the token in it and all.
		"TELEGRAM_API_URL=http://127.0.0.1:1")
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("the bot started against a closed port:\n%s", out)
	}
	if strings.Contains(string(out), syntheticToken) {
		t.Errorf("the bot token was written to the log verbatim:\n%s", out)
	}
	// Without this the test would also pass if the process logged nothing at all, or if the
	// failure happened before any Telegram call was attempted.
	if !strings.Contains(string(out), "/bot<redacted>") {
		t.Errorf("no redacted API URL in the output, so nothing proves a token-bearing line was "+
			"logged and stripped:\n%s", out)
	}
}
