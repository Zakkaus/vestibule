package telegram

import (
	"strings"

	"github.com/Zakkaus/vestibule/internal/telegram/queue"
)

// MarkupRejected reports errors that may indicate rejected Telegram HTML entities.
func MarkupRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "parse") || strings.Contains(message, "entit") || strings.Contains(message, "bad request")
}

// CannotInitiateConversation reports the ordinary 403 returned before a user has started the bot.
func CannotInitiateConversation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "bot can't initiate conversation with a user") {
		return false
	}
	code := queue.ErrorCode(err)
	return code == 0 || code == 403
}

// BotWasBlockedByUser reports the distinct 403 returned after a user blocks the bot.
func BotWasBlockedByUser(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "bot was blocked by the user") {
		return false
	}
	code := queue.ErrorCode(err)
	return code == 0 || code == 403
}

// JoinRequestGone reports a join-request action that Telegram rejected because it no
// longer holds the request: already settled, withdrawn by the applicant, or the user is
// a member by now. Retrying such a call can never succeed.
func JoinRequestGone(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "hide_requester_missing") ||
		strings.Contains(message, "user_already_participant") ||
		strings.Contains(message, "participant_id_invalid")
}

// ApplicantGone reports a target whose Telegram account no longer exists. Neither approving nor
// declining their join request can ever succeed, and no administrator can settle it by hand
// either, so the attempt is spent at once and nobody is asked to look at it.
func ApplicantGone(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "user is deactivated") && !strings.Contains(message, "user_deactivated") {
		return false
	}
	code := queue.ErrorCode(err)
	return code == 0 || code == 403
}

// IsBlocked reports Telegram 403 responses indicating that the bot cannot contact the target.
func IsBlocked(err error) bool {
	if err == nil {
		return false
	}
	if queue.ErrorCode(err) == 403 {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bot was blocked") || strings.Contains(message, "forbidden: bot")
}
