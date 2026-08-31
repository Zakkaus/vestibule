package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mymmrac/telego/telegoapi"
)

func TestRateLimitClassification(t *testing.T) {
	rateErr := &telegoapi.Error{
		ErrorCode:   429,
		Description: "Too Many Requests",
		Parameters:  &telegoapi.ResponseParameters{RetryAfter: 30},
	}
	if !IsRateLimited(rateErr) || RetryAfter(rateErr) != 30*time.Second {
		t.Errorf("429 classification = rateLimited %v retryAfter %v", IsRateLimited(rateErr), RetryAfter(rateErr))
	}
	if RetryAfter(errors.New("Too Many Requests: retry after: 7")) != 7*time.Second {
		t.Error("text retry-after was not extracted")
	}
}

func TestEditAndPostErrorClassification(t *testing.T) {
	if PermanentEditError(errors.New("Bad Request: chat not found")) {
		t.Error("chat not found must remain transient for edits")
	}
	for _, message := range []string{"message to edit not found", "message can't be edited", "MESSAGE_ID_INVALID"} {
		if !PermanentEditError(errors.New("Bad Request: " + message)) {
			t.Errorf("%q should be a permanent edit error", message)
		}
	}
	if CountablePermanentEditError(context.Canceled) || CountablePermanentEditError(errors.New("Bad Gateway")) {
		t.Error("cancellation and 5xx-like errors must not count as deterministic 400s")
	}
	if !CountablePermanentEditError(&telegoapi.Error{ErrorCode: 400, Description: "Bad Request"}) {
		t.Error("structured 400 should count as deterministic")
	}
	if PermanentPostError(errors.New("Bad Request: chat not found")) || !PermanentPostError(errors.New("Bad Request: invalid entity")) {
		t.Error("post permanence classification changed")
	}
}

func TestDestinationFailuresAreNotPermanentPerMessage(t *testing.T) {
	destinations := []string{
		"Bad Request: not enough rights to send text messages to the chat",
		"Bad Request: have no rights to send a message",
		"Bad Request: CHAT_WRITE_FORBIDDEN",
		"Bad Request: CHAT_SEND_PLAIN_FORBIDDEN",
		"Bad Request: TOPIC_CLOSED",
		"Bad Request: chat not found",
		"Bad Request: group chat was upgraded to a supergroup chat, migrate to chat id",
	}
	for _, description := range destinations {
		err := &telegoapi.Error{ErrorCode: 400, Description: description}
		if CountablePermanentEditError(err) {
			t.Errorf("CountablePermanentEditError(%q) = true, want false", description)
		}
		if PermanentPostError(err) {
			t.Errorf("PermanentPostError(%q) = true, want false", description)
		}
	}
	own := &telegoapi.Error{ErrorCode: 400, Description: "Bad Request: message is too long"}
	if !CountablePermanentEditError(own) || !PermanentPostError(own) {
		t.Error("an unclassified 400 is a problem with the message itself and must still count")
	}
}

func TestGroupUnreachable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New(`api: 403 "Forbidden: bot is not a member of the supergroup chat"`), true},
		{errors.New(`api: 403 "Forbidden: bot was kicked from the supergroup chat"`), true},
		{errors.New(`api: 400 "Bad Request: chat not found"`), true},
		{errors.New(`api: 400 "Bad Request: not enough rights"`), false},
		{errors.New("connection reset by peer"), false},
		{nil, false},
	}
	for _, test := range tests {
		if got := GroupUnreachable(test.err); got != test.want {
			t.Errorf("GroupUnreachable(%v) = %v, want %v", test.err, got, test.want)
		}
	}
}

func TestPace(t *testing.T) {
	if !Pace(context.Background(), 0) {
		t.Error("disabled pacing should return immediately")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Pace(ctx, time.Hour) {
		t.Error("cancelled pacing should return false")
	}
}
