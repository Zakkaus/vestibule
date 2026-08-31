package queue

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	cleanupTimerMax = 256
	deleteRetries   = 2
	deleteRetryCap  = 30 * time.Second
)

// A variable so tests can retry without waiting out the production delay.
var deleteRetryDelay = 2 * time.Second

// DeleteClient is the Telegram operation used by DeleteQueue.
type DeleteClient interface {
	DeleteMessage(context.Context, *telego.DeleteMessageParams) error
}

// DeleteQueue bounds scheduled cleanup and retries Telegram 429 and transient failures.
type DeleteQueue struct {
	client  DeleteClient
	pending atomic.Int32
}

// NewDeleteQueue creates a deletion queue over one Telegram client.
func NewDeleteQueue(client DeleteClient) *DeleteQueue {
	return &DeleteQueue{client: client}
}

// Pending returns the number of scheduled cleanup and retry operations.
func (q *DeleteQueue) Pending() int32 {
	return q.pending.Load()
}

// Delete removes one message and treats a zero message ID as a no-op.
func (q *DeleteQueue) Delete(ctx context.Context, chatID int64, messageID int) {
	if messageID == 0 {
		return
	}
	err := q.client.DeleteMessage(ctx, &telego.DeleteMessageParams{ChatID: tu.ID(chatID), MessageID: messageID})
	q.afterDelete(chatID, messageID, err, deleteRetries)
}

// Schedule deletes group messages after the requested delay. Private-chat messages are retained.
func (q *DeleteQueue) Schedule(chatID int64, firstMessageID, secondMessageID int, after time.Duration) {
	if chatID >= 0 || after < 0 || firstMessageID == 0 {
		return
	}
	if !q.reserve() {
		log.Printf("cleanup timer for message %d in chat %d dropped: %d timers already pending", firstMessageID, chatID, cleanupTimerMax)
		return
	}
	time.AfterFunc(after, func() {
		defer q.pending.Add(-1)
		q.Delete(context.Background(), chatID, firstMessageID)
		q.Delete(context.Background(), chatID, secondMessageID)
	})
}

func (q *DeleteQueue) afterDelete(chatID int64, messageID int, err error, remaining int) {
	if err == nil || MessageAlreadyGone(err) {
		return
	}
	if GroupUnreachable(err) || remaining <= 0 {
		log.Printf("delete message %d in chat %d failed: %v", messageID, chatID, err)
		return
	}
	wait := RetryAfter(err)
	if wait <= 0 {
		wait = deleteRetryDelay
	}
	if wait > deleteRetryCap {
		log.Printf("delete message %d in chat %d: Telegram asked for %s, longer than cleanup waits: %v", messageID, chatID, wait, err)
		return
	}
	if !q.reserve() {
		log.Printf("delete retry for message %d in chat %d dropped: %d timers already pending: %v", messageID, chatID, cleanupTimerMax, err)
		return
	}
	time.AfterFunc(wait, func() {
		defer q.pending.Add(-1)
		retryErr := q.client.DeleteMessage(context.Background(), &telego.DeleteMessageParams{
			ChatID:    tu.ID(chatID),
			MessageID: messageID,
		})
		q.afterDelete(chatID, messageID, retryErr, remaining-1)
	})
}

func (q *DeleteQueue) reserve() bool {
	for {
		count := q.pending.Load()
		if count >= cleanupTimerMax {
			return false
		}
		if q.pending.CompareAndSwap(count, count+1) {
			return true
		}
	}
}
