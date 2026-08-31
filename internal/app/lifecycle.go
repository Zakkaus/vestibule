package app

import (
	"context"
	"fmt"
	"log"
	"time"
)

const shutdownDeadline = 20 * time.Second

type runtimeLifecycle struct {
	handlerDone       <-chan error
	stopHandlers      func(context.Context) error
	waitRegistration  func()
	heartbeatDone     <-chan struct{}
	flushVerification func()
	feedDone          <-chan struct{}
	notifierDone      <-chan error
	shutdownDeadline  time.Duration
}

func runRuntimeLifecycle(ctx context.Context, lifecycle runtimeLifecycle) error {
	handlerErr, handlerStopped := waitForRuntimeStop(ctx, lifecycle.handlerDone)
	if handlerStopped && streamEndedUnexpectedly(ctx.Err()) {
		return unexpectedHandlerError(handlerErr)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), lifecycle.deadline())
	defer cancel()
	log.Printf("shutdown: waiting up to %s to drain fetched updates and in-flight update handlers", lifecycle.deadline())
	handlerErr, _ = waitForHandlerShutdown(shutdownCtx, lifecycle.handlerDone, handlerErr, handlerStopped)
	stopUpdateHandlers(shutdownCtx, lifecycle.stopHandlers)
	waitForRegistration(shutdownCtx, lifecycle.waitRegistration)
	waitForShutdownComponent(shutdownCtx, "Telegram heartbeat", lifecycle.heartbeatDone)
	log.Printf("shutdown: flushing verification state")
	if lifecycle.flushVerification != nil {
		lifecycle.flushVerification()
	}
	waitForShutdownComponent(shutdownCtx, "feed state flush", lifecycle.feedDone)
	waitForNotifier(shutdownCtx, lifecycle.notifierDone)
	return nil
}

// A live context means the update stream died unexpectedly and the supervisor must restart it.
func streamEndedUnexpectedly(ctxErr error) bool { return ctxErr == nil }

func (l runtimeLifecycle) deadline() time.Duration {
	if l.shutdownDeadline > 0 {
		return l.shutdownDeadline
	}
	return shutdownDeadline
}

func waitForRuntimeStop(ctx context.Context, done <-chan error) (error, bool) {
	select {
	case err := <-done:
		return err, true
	case <-ctx.Done():
		return nil, false
	}
}

func unexpectedHandlerError(err error) error {
	if err != nil {
		return fmt.Errorf("handler stopped unexpectedly: %w", err)
	}
	return fmt.Errorf("update stream ended without a shutdown signal — exiting non-zero so systemd restarts us")
}

func waitForHandlerShutdown(
	ctx context.Context,
	done <-chan error,
	current error,
	stopped bool,
) (error, bool) {
	if !stopped {
		select {
		case current = <-done:
			stopped = true
		case <-ctx.Done():
			log.Printf("shutdown: fetched updates did not drain before deadline: %v", ctx.Err())
		}
	}
	if stopped && current != nil {
		log.Printf("shutdown: handler loop stopped: %v", current)
	}
	return current, stopped
}

func stopUpdateHandlers(ctx context.Context, stop func(context.Context) error) {
	if stop == nil {
		return
	}
	if err := stop(ctx); err != nil {
		log.Printf("shutdown: update handlers did not stop cleanly: %v", err)
	}
}

func waitForRegistration(ctx context.Context, wait func()) {
	if wait == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	waitForShutdownComponent(ctx, "registration timers", done)
}

func waitForNotifier(ctx context.Context, done <-chan error) {
	if done == nil {
		return
	}
	select {
	case err := <-done:
		if err != nil {
			log.Printf("shutdown: systemd notification failed: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutdown: systemd notifier did not stop before deadline: %v", ctx.Err())
	}
}

func waitForShutdownComponent(ctx context.Context, name string, done <-chan struct{}) {
	if done == nil {
		return
	}
	log.Printf("shutdown: waiting for %s", name)
	select {
	case <-done:
		log.Printf("shutdown: %s complete", name)
	case <-ctx.Done():
		log.Printf("shutdown: %s timed out: %v", name, ctx.Err())
	}
}
