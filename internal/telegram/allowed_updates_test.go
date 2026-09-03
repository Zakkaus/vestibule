package telegram

import (
	"slices"
	"testing"
)

// Long polling asks Telegram for exactly these update kinds. Removing any one of them left
// every test in the repository passing, and the bot would keep running: it polls, logs
// nothing unusual, and simply never sees that kind of event again.
//
// Telegram's own rule makes the list load-bearing rather than decorative. An empty list means
// every kind except chat_member, message_reaction and message_reaction_count; omitting the
// field entirely means "keep whatever this token was last set to", which is state on
// Telegram's side that a fresh deployment does not control. So the list is both the
// subscription and the only record of what this bot needs.
func TestAllowedUpdateTypesCoverEveryHandledEvent(t *testing.T) {
	allowed := AllowedUpdateTypes()
	for _, required := range []struct {
		kind string
		why  string
	}{
		{"chat_join_request", "the join request every verification starts from; without it " +
			"nobody is ever challenged and nobody is ever admitted"},
		{"chat_member", "an applicant entering or leaving mid-verification, and the one kind " +
			"Telegram leaves out of its default set, so naming it here is the only way to get it"},
		{"callback_query", "the applicant's answer and the administrator's approve and reject " +
			"buttons; without it every challenge times out"},
		{"message", "answers sent as text, and every command the bot answers"},
		{"my_chat_member", "the bot being added to or removed from a chat, which is how " +
			"registration and auto-leave learn where the bot is"},
	} {
		if !slices.Contains(allowed, required.kind) {
			t.Errorf("allowed updates %v do not include %q — %s",
				allowed, required.kind, required.why)
		}
	}
}

// The list has to reach the poll. Returning nothing would leave the request without the
// field, and Telegram would keep this token's previous setting rather than take ours.
func TestAllowedUpdateTypesIsNotEmpty(t *testing.T) {
	if len(AllowedUpdateTypes()) == 0 {
		t.Fatal("allowed updates is empty; the poll would carry no subscription and Telegram " +
			"would apply whatever this bot token was last set to")
	}
}

// The caller gets its own slice: a caller that sorts or truncates its copy must not change
// what the next poll subscribes to.
func TestAllowedUpdateTypesReturnsACopy(t *testing.T) {
	first := AllowedUpdateTypes()
	if len(first) == 0 {
		t.Fatal("nothing to copy")
	}
	first[0] = "mutated"
	if AllowedUpdateTypes()[0] == "mutated" {
		t.Error("a caller's edit reached the shared list; the next poll would subscribe to it")
	}
}
