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
