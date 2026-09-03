package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Telegram has four ways of saying the message is not there to delete, and each one means the
// chat already reached the state the caller wanted. Only the first of them was held by a test,
// and the other three cannot stand in for each other: none of the four descriptions contains
// any of the others. An unrecognised one costs two pointless retries and a "delete message
// failed" line for a message nobody could have deleted anyway.
func TestDeleteTreatsEveryAlreadyGoneAnswerAsDone(t *testing.T) {
	for _, description := range []string{
		"message to delete not found",
		"message can't be deleted",
		"message identifier is not specified",
		"MESSAGE_ID_INVALID",
	} {
		t.Run(description, func(t *testing.T) {
			withShortDeleteRetry(t)
			client := &deleteClient{errors: []error{errors.New("Bad Request: " + description)}}
			queue := NewDeleteQueue(client)
			queue.Delete(context.Background(), -100, 42)
			time.Sleep(20 * time.Millisecond)
			if got := len(client.snapshot()); got != 1 {
				t.Fatalf("delete calls after %q = %d, want 1", description, got)
			}
		})
	}
}

// The negative control: an answer that does not say the message is gone must still be retried,
// so the test above is measuring recognition rather than a queue that never retries anything.
func TestDeleteStillRetriesAnAnswerThatIsNotAboutAMissingMessage(t *testing.T) {
	withShortDeleteRetry(t)
	client := &deleteClient{errors: []error{errors.New("Bad Request: message text is empty")}}
	queue := NewDeleteQueue(client)
	queue.Delete(context.Background(), -100, 42)
	waitForDeleteCalls(t, client, 2)
}
