package ids

import (
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
)

// ReplyParameters binds a response to msgID and returns nil for an unbound response.
func ReplyParameters(msgID int) *telego.ReplyParameters {
	if msgID == 0 {
		return nil
	}
	return &telego.ReplyParameters{MessageID: msgID}
}

// MessageID returns a sent message ID or zero when Telegram returned no message.
func MessageID(message *telego.Message) int {
	if message == nil {
		return 0
	}
	return message.MessageID
}

// ParseChannelID canonicalizes Bot API -100 IDs and bare t.me/c IDs.
func ParseChannelID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}
	if id < 0 {
		return id, true
	}
	full, err := strconv.ParseInt("-100"+value, 10, 64)
	if err != nil {
		return 0, false
	}
	return full, true
}
