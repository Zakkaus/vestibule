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
