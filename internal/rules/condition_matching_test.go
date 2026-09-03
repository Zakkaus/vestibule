package rules

import "testing"

// Settings validation accepts any answer with a non-space character, so an administrator can save a
// fallback answer made only of punctuation. The answer normalizer strips it to nothing, and if an
// empty normalized answer were allowed to match, a bot replying "..." would be approved into the
// group without having answered anything.
func TestAFallbackAnswerThatNormalizesToNothingApprovesNobody(t *testing.T) {
	if !(OneOf{Values: []string{"gentoozh.org"}}).MatchesAnswer("（gentoozh.org。）") {
		t.Fatal("a real fallback answer did not match, so the refusals below prove nothing")
	}
	for _, test := range []struct {
		name  string
		saved string
		reply string
	}{
		{name: "ideographic full stops", saved: "。。。", reply: "。。。"},
		{name: "question marks", saved: "???", reply: "??"},
		{name: "punctuation answer against a punctuation reply", saved: "！！", reply: "..."},
		{name: "empty answer", saved: "", reply: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if (OneOf{Values: []string{test.saved}}).MatchesAnswer(test.reply) {
				t.Errorf("saved answer %q accepted the reply %q: an applicant who typed only punctuation was let into the group", test.saved, test.reply)
			}
		})
	}
}

// The kernel rule rejects the prompt's impossible sample so that copying it is spent on the one free
// nudge rather than on an attempt. Grading it through the answer normalizer instead would strip the
// punctuation a chat client adds, so an ordinary wrong answer such as "(X.Y.Z)" would burn the nudge
// that a genuinely confused applicant needs.
func TestTheSampleRejectionComparesTheReplyAsItWasTyped(t *testing.T) {
	rule := Rule{
		Accept: []Condition{VersionRange{Intervals: []VersionInterval{{
			Minimum: Version{Major: 3},
			Maximum: Version{Major: 30, Minor: 99},
		}}}},
		Reject: []Condition{OneOf{Values: []string{"X.Y.Z"}}},
	}
	if got := rule.Evaluate("X.Y.Z"); got != Rejected {
		t.Fatalf("the sample itself was not rejected: verdict = %v, want %v", got, Rejected)
	}
	for _, text := range []string{"(X.Y.Z)", "X.Y.Z.", "「x.y.z」", "https://x.y.z"} {
		if got := rule.Evaluate(text); got == Rejected {
			t.Errorf("Evaluate(%q) = %v: a reply that is not the sample spent the applicant's one free copied-sample nudge", text, got)
		}
	}
}

// A Contains value that normalizes away matches every possible input. One empty string left in a
// locale's no-Linux or other-OS list would route every applicant, spam accounts included, straight
// to the answer-hidden fallback instead of asking them for a kernel version.
func TestAContainsPhraseThatNormalizesToNothingMatchesNobody(t *testing.T) {
	if !(Contains{Value: "no linux"}).Matches("i have no linux here") {
		t.Fatal("a real phrase did not match, so the refusals below prove nothing")
	}
	if !(Contains{Value: "no linux", CompactWhitespace: true}).Matches("i have no　linux here") {
		t.Fatal("a real whitespace-folded phrase did not match, so the refusals below prove nothing")
	}
	for _, test := range []struct {
		name      string
		condition Contains
	}{
		{name: "empty value", condition: Contains{Value: ""}},
		{name: "spaces only", condition: Contains{Value: "   "}},
		{name: "invisible characters only", condition: Contains{Value: "\ufeff\u200b"}},
		{name: "empty value with whitespace folding", condition: Contains{Value: "", CompactWhitespace: true}},
		{name: "spaces only with whitespace folding", condition: Contains{Value: " 　 ", CompactWhitespace: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, text := range []string{"6.12.3-gentoo", "", "买粉丝"} {
				if test.condition.Matches(text) {
					t.Errorf("Matches(%q) = true: an empty phrase declared every reply a no-Linux answer, so nobody is asked for a kernel version", text)
				}
				if test.condition.MatchesNormalized(Normalize(text)) {
					t.Errorf("MatchesNormalized(%q) = true: the pre-normalized path used by the no-Linux check also matched everything", text)
				}
			}
		})
	}
}
