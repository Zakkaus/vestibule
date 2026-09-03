package app

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"
)

func TestSystemdNotifierUnsetIsNoop(t *testing.T) {
	notifier, err := newSystemdNotifier("")
	if err != nil {
		t.Fatal(err)
	}
	if err := notifier.ready(); err != nil {
		t.Fatalf("ready without NOTIFY_SOCKET: %v", err)
	}
	if err := notifier.watchdog(); err != nil {
		t.Fatalf("watchdog without NOTIFY_SOCKET: %v", err)
	}
	if err := notifier.stopping(); err != nil {
		t.Fatalf("stopping without NOTIFY_SOCKET: %v", err)
	}
}

func TestSystemdLifecycleFollowsStartupAndProgress(t *testing.T) {
	path := t.TempDir() + "/notify.sock"
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	notifier, err := newSystemdNotifier(path)
	if err != nil {
		t.Fatal(err)
	}
	defer notifier.close()

	ctx, cancel := context.WithCancel(context.Background())
	startupComplete := make(chan struct{})
	progress := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runSystemdLifecycle(ctx, notifier, startupComplete, progress)
	}()

	expectNoNotify(t, listener)
	close(startupComplete)
	expectNotify(t, listener, "READY=1")
	if err := notifier.ready(); err != nil {
		t.Fatal(err)
	}
	expectNoNotify(t, listener)

	progress <- struct{}{}
	expectNotify(t, listener, "WATCHDOG=1")
	expectNoNotify(t, listener)

	cancel()
	expectNotify(t, listener, "STOPPING=1")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	expectNoNotify(t, listener)
}

func TestSystemdNotifierRejectsUnavailableSocket(t *testing.T) {
	unavailable := t.TempDir() + "/notify.sock"
	if _, err := newSystemdNotifier(unavailable); err == nil {
		t.Fatal("systemd notifier accepted an unavailable socket; startup would run without readiness reporting")
	}

	path := t.TempDir() + "/notify.sock"
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	notifier, err := newSystemdNotifier(path)
	if err != nil {
		t.Fatalf("systemd notifier rejected a live socket: %v", err)
	}
	defer notifier.close()
}

func TestSystemdLifecycleReportsStoppingBeforeStartup(t *testing.T) {
	path := t.TempDir() + "/notify.sock"
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	notifier, err := newSystemdNotifier(path)
	if err != nil {
		t.Fatal(err)
	}
	defer notifier.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runSystemdLifecycle(ctx, notifier, make(chan struct{}), make(chan struct{})); err != nil {
		t.Fatalf("systemd lifecycle before startup: %v", err)
	}
	expectNotify(t, listener, "STOPPING=1")
	expectNoNotify(t, listener)
}

func TestRunReportsReadinessProgressAndStoppingToSystemd(t *testing.T) {
	telegram, _ := newSetupTelegramAPI(t)
	stateDirectory := t.TempDir()
	path := t.TempDir() + "/notify.sock"
	listener, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	finished := false
	defer func() {
		if finished {
			return
		}
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run after test cleanup: %v", err)
			}
		case <-time.After(shutdownDeadline + time.Second):
			t.Error("Run did not stop after test cleanup")
		}
	}()
	go func() {
		done <- Run(ctx, Options{
			ConfigPath:     stateDirectory + "/missing-config.json",
			StateDirectory: stateDirectory,
			Token:          appSetupBotToken,
			TelegramAPIURL: telegram.URL,
			NotifySocket:   path,
			ConsoleAddr:    reserveSetupConsoleAddress(t),
		})
	}()

	expectNotify(t, listener, "READY=1")
	expectNotify(t, listener, "WATCHDOG=1")
	cancel()
	expectNotifyEventually(t, listener, "STOPPING=1")
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	finished = true
}

func expectNotify(t *testing.T, listener *net.UnixConn, want string) {
	t.Helper()
	if err := listener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set systemd notification deadline for %q: %v", want, err)
	}
	buf := make([]byte, 128)
	n, _, err := listener.ReadFromUnix(buf)
	if err != nil {
		t.Fatalf("systemd did not receive %q: %v", want, err)
	}
	if got := string(buf[:n]); got != want {
		t.Fatalf("notification = %q, want %q", got, want)
	}
}

func expectNotifyEventually(t *testing.T, listener *net.UnixConn, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if err := listener.SetReadDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 128)
		n, _, err := listener.ReadFromUnix(buf)
		if err != nil {
			t.Fatalf("systemd did not receive %q: %v", want, err)
		}
		if got := string(buf[:n]); got == want {
			return
		} else if got != "WATCHDOG=1" {
			t.Fatalf("notification = %q while waiting for %q", got, want)
		}
	}
}

func expectNoNotify(t *testing.T, listener *net.UnixConn) {
	t.Helper()
	if err := listener.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	_, _, err := listener.ReadFromUnix(buf)
	if err == nil {
		t.Fatal("unexpected systemd notification")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("read notification: %v", err)
		}
	}
}
