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
func expectNotify(t *testing.T, listener *net.UnixConn, want string) {
	t.Helper()
	if err := listener.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 128)
	n, _, err := listener.ReadFromUnix(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != want {
		t.Fatalf("notification = %q, want %q", got, want)
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
