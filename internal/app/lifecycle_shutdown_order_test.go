package app

import (
	"context"
	"testing"
	"time"
)

func TestVerificationStateFlushWaitsForEveryWorkerToStop(t *testing.T) {
	workers := []struct {
		name string
		hold func(chan struct{}, chan struct{}, chan struct{}) chan struct{}
	}{
		{
			name: "Telegram heartbeat",
			hold: func(heartbeat, expiry, action chan struct{}) chan struct{} {
				close(expiry)
				close(action)
				return heartbeat
			},
		},
		{
			name: "verification expiry scanner",
			hold: func(heartbeat, expiry, action chan struct{}) chan struct{} {
				close(heartbeat)
				close(action)
				return expiry
			},
		},
		{
			name: "verification action executor",
			hold: func(heartbeat, expiry, action chan struct{}) chan struct{} {
				close(heartbeat)
				close(expiry)
				return action
			},
		},
	}
	for _, worker := range workers {
		t.Run(worker.name, func(t *testing.T) {
			root, cancel := context.WithCancel(context.Background())
			cancel()
			heartbeatDone := make(chan struct{})
			expiryDone := make(chan struct{})
			actionDone := make(chan struct{})
			held := worker.hold(heartbeatDone, expiryDone, actionDone)
			admissionStopped := make(chan struct{})
			flushed := make(chan struct{})
			result := make(chan error, 1)
			go func() {
				result <- runRuntimeLifecycle(root, runtimeLifecycle{
					stopAdmission: func() error {
						close(admissionStopped)
						return nil
					},
					heartbeatDone: heartbeatDone,
					expiryDone:    expiryDone,
					actionDone:    actionDone,
					flushVerification: func() {
						flushed <- struct{}{}
					},
					shutdownDeadline: time.Second,
				})
			}()

			select {
			case <-admissionStopped:
			case <-time.After(time.Second):
				t.Fatal("shutdown did not begin stopping admission")
			}
			select {
			case <-flushed:
				t.Fatalf("verification state flushed while %s still ran; an in-flight lease could be persisted half-settled", worker.name)
			case <-time.After(100 * time.Millisecond):
			}
			close(held)
			select {
			case <-flushed:
			case <-time.After(time.Second):
				t.Fatalf("verification state did not flush after %s stopped", worker.name)
			}
			if err := <-result; err != nil {
				t.Fatal(err)
			}
		})
	}
}
