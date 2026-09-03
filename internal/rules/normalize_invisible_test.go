package rules

import "testing"

// An invisible character spliced into a banned phrase must not break the phrase apart. The reply
// renders identically to a moderator, so a reject rule that stops firing means the moderator sees
// a message they believe was refused while the applicant was only told to try again.
func TestInvisibleCharactersCannotSplitABannedPhrase(t *testing.T) {
	rule := Rule{
		Accept: []Condition{Contains{Value: "gentoo"}},
		Reject: []Condition{OneOf{Values: []string{"gentoo@emerge"}}},
	}
	if got := rule.Evaluate("gentoo@emerge"); got != Rejected {
		t.Fatalf("the plain banned phrase was not rejected: verdict = %v, want %v", got, Rejected)
	}
	for _, test := range []struct {
		name string
		text string
	}{
		{name: "byte order mark", text: "gentoo\ufeff@emerge"},
		{name: "variation selector 1", text: "gentoo\ufe00@emerge"},
		{name: "variation selector 16", text: "gentoo\ufe0f@emerge"},
		{name: "variation selector supplement first", text: "gentoo\U000e0100@emerge"},
		{name: "variation selector supplement last", text: "gentoo\U000e01ef@emerge"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := rule.Evaluate(test.text); got != Rejected {
				t.Errorf("Evaluate(%q) = %v, want %v: an invisible character let a banned reply through as a retryable answer", test.text, got, Rejected)
			}
		})
	}
	if got := rule.Evaluate("gentoo is fine"); got == Rejected {
		t.Error("an ordinary reply was rejected, so the rejections above prove nothing")
	}
}

// A reply typed on a Chinese or Japanese IME arrives with the ideographic space. Folding it to an
// ASCII space is what lets the stored phrase 'mac os' recognise 'ｍａｃ　ｏｓ'; without the fold the
// applicant is charged a wrong kernel answer instead of being offered the clarification, and three
// of those decline and ban them.
func TestFullWidthTypingStillMatchesAPhraseStoredWithASCIISpaces(t *testing.T) {
	phrase := Contains{Value: "mac os"}
	if !phrase.Matches("mac os") {
		t.Fatal("the phrase does not match its own ASCII spelling, so the fold below proves nothing")
	}
	for _, text := range []string{"ｍａｃ　ｏｓ", "mac　os", "ＭＡＣ　ＯＳ 12.7"} {
		if !phrase.Matches(text) {
			t.Errorf("Matches(%q) = false: a full-width reply was not recognised as the other-OS phrase", text)
		}
	}
	if phrase.Matches("macos") {
		t.Error("a spelling with no separator at all matched, so the fold is not what the test is measuring")
	}
}
