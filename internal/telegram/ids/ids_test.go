package ids

import (
	"slices"
	"testing"

	"github.com/mymmrac/telego"
)

func TestParseChannelID(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int64
		ok    bool
	}{
		{input: "-1009999900006", want: -1009999900006, ok: true},
		{input: "9999900006", want: -1009999900006, ok: true},
		{input: " 9999900006 ", want: -1009999900006, ok: true},
		{input: "-100999999999", want: -100999999999, ok: true},
		{input: "999999999", want: -100999999999, ok: true},
		{input: ""},
		{input: "abc"},
		{input: "-100abc"},
		{input: "99999999999999999999"},
	} {
		got, ok := ParseChannelID(test.input)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("ParseChannelID(%q) = (%d, %v), want (%d, %v)", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestBareChannelIDsWhoseCanonicalFormOverflowsAreRejected(t *testing.T) {
	const valid = "9999900006"
	if got, ok := ParseChannelID(valid); !ok || got != -1009999900006 {
		t.Fatalf("valid bare channel ID %q = (%d, %v), want (-1009999900006, true)", valid, got, ok)
	}

	const tooLarge = "9223372036854775807"
	if got, ok := ParseChannelID(tooLarge); ok {
		t.Fatalf("overflowing bare channel ID %q was accepted as %d: an administrator could whitelist the wrong sender chat",
			tooLarge, got)
	}
}

func TestMissingTelegramMessagesHaveNoIdentifier(t *testing.T) {
	const messageID = 73
	if got := MessageID(&telego.Message{MessageID: messageID}); got != messageID {
		t.Fatalf("returned Telegram message ID = %d, want %d", got, messageID)
	}
	if got := MessageID(nil); got != 0 {
		t.Fatalf("missing Telegram message ID = %d, want 0: failed sends could schedule deletion of an unrelated message", got)
	}
}

func TestUpdateChannelWhitelistBoundsNewestEntries(t *testing.T) {
	for _, test := range []struct {
		name  string
		extra int
	}{
		{name: "one over cap", extra: 1},
		{name: "multiple over cap", extra: 19},
	} {
		t.Run(test.name, func(t *testing.T) {
			var whitelist []int64
			for index := range channelWhitelistMax + test.extra {
				whitelist = UpdateChannelWhitelist(whitelist, -1000000-int64(index), true)
			}
			if len(whitelist) != channelWhitelistMax {
				t.Fatalf("whitelist entries = %d, want %d", len(whitelist), channelWhitelistMax)
			}
			for index := range test.extra {
				for _, senderID := range whitelist {
					if senderID == -1000000-int64(index) {
						t.Errorf("oldest whitelist entry %d was not evicted", index)
					}
				}
			}
			if whitelist[len(whitelist)-1] != -1000000-int64(channelWhitelistMax+test.extra-1) {
				t.Error("newest whitelist entry was evicted")
			}
		})
	}
}

func TestDenyingAbsentChannelLeavesWhitelistUnchanged(t *testing.T) {
	current := []int64{-1009000001611, -1009000001612}
	denied := int64(-1009000001613)

	got := UpdateChannelWhitelist(slices.Clone(current), denied, false)
	if !slices.Equal(got, current) {
		t.Fatalf("denying an absent channel changed the whitelist to %v; want unchanged %v", got, current)
	}

	got = UpdateChannelWhitelist(slices.Clone(current), denied, true)
	want := append(slices.Clone(current), denied)
	if !slices.Equal(got, want) {
		t.Fatalf("allowing an absent channel produced %v; want %v", got, want)
	}
}

func TestDenyingPresentChannelRemovesOnlyThatEntry(t *testing.T) {
	denied := int64(-1009000001622)
	current := []int64{-1009000001621, denied, -1009000001623}

	got := UpdateChannelWhitelist(slices.Clone(current), denied, false)
	want := []int64{-1009000001621, -1009000001623}
	if !slices.Equal(got, want) {
		t.Fatalf("denied channel remained whitelisted: got %v, want %v", got, want)
	}
}
