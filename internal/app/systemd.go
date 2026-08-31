package app

import (
	"context"
	"log"
	"net"
	"sync"
)

type systemdNotifier struct {
	conn *net.UnixConn

	readyOnce    sync.Once
	readyErr     error
	stoppingOnce sync.Once
	stoppingErr  error
	closeOnce    sync.Once
	closeErr     error
}

func newSystemdNotifier(socket string) (*systemdNotifier, error) {
	if socket == "" {
		return &systemdNotifier{}, nil
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		return nil, err
	}
	return &systemdNotifier{conn: conn}, nil
}

func (n *systemdNotifier) notify(state string) error {
	if n == nil || n.conn == nil {
		return nil
	}
	_, err := n.conn.Write([]byte(state))
	return err
}

func (n *systemdNotifier) ready() error {
	n.readyOnce.Do(func() { n.readyErr = n.notify("READY=1") })
	return n.readyErr
}

func (n *systemdNotifier) watchdog() error { return n.notify("WATCHDOG=1") }

func (n *systemdNotifier) stopping() error {
	n.stoppingOnce.Do(func() { n.stoppingErr = n.notify("STOPPING=1") })
	return n.stoppingErr
}

func (n *systemdNotifier) close() error {
	if n == nil || n.conn == nil {
		return nil
	}
	n.closeOnce.Do(func() { n.closeErr = n.conn.Close() })
	return n.closeErr
}

func runSystemdLifecycle(
	ctx context.Context,
	notifier *systemdNotifier,
	startupComplete <-chan struct{},
	progress <-chan struct{},
) error {
	select {
	case <-ctx.Done():
		return notifier.stopping()
	case <-startupComplete:
		if err := notifier.ready(); err != nil {
			return err
		}
	}
	for {
		select {
		case <-ctx.Done():
			return notifier.stopping()
		case _, ok := <-progress:
			if !ok {
				progress = nil
				continue
			}
			if err := notifier.watchdog(); err != nil {
				log.Printf("systemd watchdog notification failed: %v", err)
			}
		}
	}
}
