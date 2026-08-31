package ids

import "testing"

func TestParseChannelID(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int64
		ok    bool
	}{
		{input: "-1001234567890", want: -1001234567890, ok: true},
		{input: "1234567890", want: -1001234567890, ok: true},
		{input: " 1234567890 ", want: -1001234567890, ok: true},
		{input: "-100123456789", want: -100123456789, ok: true},
		{input: "123456789", want: -100123456789, ok: true},
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
