package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"
)

type deleteClient struct {
	mu     sync.Mutex
	errors []error
	calls  []telego.DeleteMessageParams
	called chan struct{}
}

func (c *deleteClient) DeleteMessage(_ context.Context, params *telego.DeleteMessageParams) error {
	c.mu.Lock()
	c.calls = append(c.calls, *params)
	var err error
	if len(c.errors) > 0 {
		err = c.errors[0]
		c.errors = c.errors[1:]
	}
	c.mu.Unlock()
	if c.called != nil {
		select {
		case c.called <- struct{}{}:
		default:
		}
	}
	return err
}

func (c *deleteClient) snapshot() []telego.DeleteMessageParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]telego.DeleteMessageParams(nil), c.calls...)
}

func withShortDeleteRetry(t *testing.T) {
	t.Helper()
	previous := deleteRetryDelay
	deleteRetryDelay = time.Millisecond
	t.Cleanup(func() { deleteRetryDelay = previous })
}

func waitForDeleteCalls(t *testing.T, client *deleteClient, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(client.snapshot()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("delete calls = %d, want %d", len(client.snapshot()), want)
}

func waitForPending(t *testing.T, queue *DeleteQueue, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if queue.Pending() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("pending cleanup = %d, want %d", queue.Pending(), want)
}

func TestDeleteRetriesATransientFailure(t *testing.T) {
	withShortDeleteRetry(t)
	client := &deleteClient{errors: []error{errors.New("unexpected EOF")}}
	queue := NewDeleteQueue(client)
	queue.Delete(context.Background(), -100, 42)
	waitForDeleteCalls(t, client, 2)
	waitForPending(t, queue, 0)
}

func TestDeleteDoesNotRetryAMessageThatIsAlreadyGone(t *testing.T) {
	withShortDeleteRetry(t)
	client := &deleteClient{errors: []error{errors.New("Bad Request: message to delete not found")}}
	queue := NewDeleteQueue(client)
	queue.Delete(context.Background(), -100, 42)
	time.Sleep(20 * time.Millisecond)
	if got := len(client.snapshot()); got != 1 {
		t.Fatalf("delete calls = %d, want 1", got)
	}
}

func TestDeleteGivesUpAfterItsRetries(t *testing.T) {
	withShortDeleteRetry(t)
	client := &deleteClient{errors: []error{
		errors.New("unexpected EOF"),
		errors.New("unexpected EOF"),
		errors.New("unexpected EOF"),
		errors.New("unexpected EOF"),
	}}
	queue := NewDeleteQueue(client)
	queue.Delete(context.Background(), -100, 42)
	waitForDeleteCalls(t, client, deleteRetries+1)
	waitForPending(t, queue, 0)
	time.Sleep(20 * time.Millisecond)
	if got := len(client.snapshot()); got != deleteRetries+1 {
		t.Fatalf("delete calls = %d, want %d", got, deleteRetries+1)
	}
}

func TestDeleteIgnoresAZeroMessageID(t *testing.T) {
	client := &deleteClient{}
	NewDeleteQueue(client).Delete(context.Background(), -100, 0)
	if got := len(client.snapshot()); got != 0 {
		t.Fatalf("delete calls = %d for zero message ID", got)
	}
}

func TestScheduleRetainsPrivateMessagesAndDeletesGroupMessagesInOrder(t *testing.T) {
	client := &deleteClient{}
	queue := NewDeleteQueue(client)
	queue.Schedule(42, 10, 9, time.Millisecond)
	if queue.Pending() != 0 {
		t.Fatalf("private cleanup pending = %d, want 0", queue.Pending())
	}
	queue.Schedule(-100, 10, 9, time.Millisecond)
	waitForDeleteCalls(t, client, 2)
	waitForPending(t, queue, 0)
	calls := client.snapshot()
	if calls[0].MessageID != 10 || calls[1].MessageID != 9 {
		t.Fatalf("delete order = %d, %d; want 10, 9", calls[0].MessageID, calls[1].MessageID)
	}
}

func TestScheduleBoundsPendingCleanup(t *testing.T) {
	client := &deleteClient{}
	queue := NewDeleteQueue(client)
	queue.pending.Store(cleanupTimerMax)
	queue.Schedule(-100, 10, 9, time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	if got := len(client.snapshot()); got != 0 {
		t.Fatalf("delete calls = %d at capacity, want 0", got)
	}
	if got := queue.Pending(); got != cleanupTimerMax {
		t.Fatalf("pending cleanup = %d, want %d", got, cleanupTimerMax)
	}
}

func TestDeleteRetryRequiresReservedSlot(t *testing.T) {
	withShortDeleteRetry(t)
	retryErr := errors.New("unexpected EOF")

	fullClient := &deleteClient{}
	fullQueue := NewDeleteQueue(fullClient)
	fullQueue.pending.Store(cleanupTimerMax)
	fullQueue.afterDelete(-1009000001681, 42, retryErr, 1)
	time.Sleep(20 * time.Millisecond)
	if got := fullQueue.Pending(); got != cleanupTimerMax {
		t.Fatalf("unreserved delete retry unbalanced pending count to %d; want %d", got, cleanupTimerMax)
	}
	if got := len(fullClient.snapshot()); got != 0 {
		t.Fatalf("delete retry ran %d times without a reserved timer slot", got)
	}

	availableClient := &deleteClient{}
	availableQueue := NewDeleteQueue(availableClient)
	availableQueue.pending.Store(cleanupTimerMax - 1)
	availableQueue.afterDelete(-1009000001682, 43, retryErr, 1)
	waitForDeleteCalls(t, availableClient, 1)
	waitForPending(t, availableQueue, cleanupTimerMax-1)
}
