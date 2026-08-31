package queue

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego/telegoapi"
)

// ErrorCode returns a structured Telegram Bot API error code or zero.
func ErrorCode(err error) int {
	var apiErr *telegoapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode
	}
	return 0
}

// GroupUnreachable reports a chat the bot can no longer act in at all.
func GroupUnreachable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bot is not a member of") ||
		strings.Contains(message, "bot was kicked from") ||
		strings.Contains(message, "chat not found") ||
		strings.Contains(message, "the group chat was deleted")
}

// RetryAfter returns Telegram's requested 429 delay or zero when none is available.
func RetryAfter(err error) time.Duration {
	var apiErr *telegoapi.Error
	if errors.As(err, &apiErr) && apiErr.Parameters != nil && apiErr.Parameters.RetryAfter > 0 {
		return time.Duration(apiErr.Parameters.RetryAfter) * time.Second
	}
	if err == nil {
		return 0
	}
	message := strings.ToLower(err.Error())
	const marker = "retry after"
	index := strings.Index(message, marker)
	if index < 0 {
		return 0
	}
	remainder := strings.TrimLeft(message[index+len(marker):], " \t\r\n:,.;")
	end := 0
	for end < len(remainder) && remainder[end] >= '0' && remainder[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	seconds, parseErr := strconv.Atoi(remainder[:end])
	if parseErr != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// MessageAlreadyGone reports a delete that has already reached the requested outcome.
func MessageAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "message to delete not found") ||
		strings.Contains(message, "message can't be deleted") ||
		strings.Contains(message, "message identifier is not specified") ||
		strings.Contains(message, "message_id_invalid")
}

// IsNotModified reports an edit that already has the requested text.
func IsNotModified(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "message is not modified")
}

// PermanentEditError reports errors proving that one specific message can never be edited.
func PermanentEditError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "message to edit not found") ||
		strings.Contains(message, "message can't be edited") ||
		strings.Contains(message, "message_id_invalid")
}

// destinationError reports failures caused by the destination chat rather than one item.
func destinationError(err error) bool {
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"chat not found",
		"migrate to chat",
		"not enough rights",
		"have no rights",
		"chat_write_forbidden",
		"chat_send_plain_forbidden",
		"chat_restricted",
		"topic_closed",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// CountablePermanentEditError reports deterministic unclassified 400 edit rejections.
func CountablePermanentEditError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if destinationError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	code := ErrorCode(err)
	return code == 400 || code == 0 && strings.Contains(message, "bad request")
}

// IsRateLimited reports Telegram throttling from a structured 429 or retry-after message.
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	if ErrorCode(err) == 429 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "too many requests") || strings.Contains(message, "retry after")
}

// PermanentPostError reports deterministic item rejection without treating destination failures as permanent.
func PermanentPostError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if destinationError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	code := ErrorCode(err)
	return code == 400 || code == 0 && strings.Contains(message, "bad request")
}

// Pace waits for a per-chat send pause or returns false when ctx is cancelled first.
func Pace(ctx context.Context, pause time.Duration) bool {
	if pause <= 0 {
		return true
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}
