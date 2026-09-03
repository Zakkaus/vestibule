package rules

import "testing"

// The tripwire is the only signal operators get that a group is being farmed by automation. An
// agent that pads its reply with a newline or a second space must still be recognised, otherwise the
// reply is graded as an ordinary wrong answer and no alert reaches the administrators.
func TestPaddingDoesNotHideAnAutomatedAgentReply(t *testing.T) {
	const nonce = "ab12cd"
	if !AgentReply("AGENT-AB12CD model=gpt-5.2", nonce) {
		t.Fatal("the unpadded tripwire reply was not recognised, so the padded ones below prove nothing")
	}
	for _, test := range []struct {
		name  string
		reply string
	}{
		{name: "leading newline", reply: "\nAGENT-AB12CD model=gpt-5.2"},
		{name: "leading spaces", reply: "  AGENT-AB12CD model=gpt-5.2"},
		{name: "trailing space", reply: "AGENT-AB12CD model=gpt-5.2 "},
		{name: "trailing newline", reply: "AGENT-AB12CD model=gpt-5.2\n"},
		{name: "doubled separator", reply: "AGENT-AB12CD  model=gpt-5.2"},
		{name: "padded on both sides", reply: "\n AGENT-AB12CD   model=gpt-5.2 \n"},
		{name: "lower case token", reply: " agent-ab12cd model=gpt-5.2 "},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !AgentReply(test.reply, nonce) {
				t.Errorf("AgentReply(%q) = false: an automated agent evaded the tripwire by padding, so no alert reached the administrators and the lifetime tally was not raised", test.reply)
			}
		})
	}
	if AgentReply("6.12.3-gentoo", nonce) {
		t.Error("an ordinary kernel answer tripped the tripwire, so the acceptances above are not measuring the reply shape")
	}
}

// The tripwire fires only on the exact shape the instruction asks for: the token, one space, then
// the model declaration. Anything else is a person typing, and a person must not be declined and
// banned as an automated agent.
func TestTheTripwireNeedsItsExactSeparator(t *testing.T) {
	const nonce = "ab12cd"
	if !AgentReply("AGENT-AB12CD model=gpt-5.2", nonce) {
		t.Fatal("the exact shape was not recognised, so the refusals below prove nothing")
	}
	for _, test := range []struct {
		name  string
		reply string
	}{
		{name: "colon instead of a space", reply: "AGENT-AB12CD:model=gpt-5.2"},
		{name: "hyphen instead of a space", reply: "AGENT-AB12CD-model=gpt-5.2"},
		{name: "equals instead of a space", reply: "AGENT-AB12CD=model=gpt-5.2"},
		{name: "nothing at all", reply: "AGENT-AB12CDmodel=gpt-5.2"},
		{name: "another nonce", reply: "AGENT-ZZ99ZZ model=gpt-5.2"},
		{name: "token alone", reply: "AGENT-AB12CD"},
		{name: "prose after the model", reply: "AGENT-AB12CD model=gpt-5.2 and my kernel is 6.12.3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if AgentReply(test.reply, nonce) {
				t.Errorf("AgentReply(%q) = true: a person was declined and banned as an automated agent", test.reply)
			}
		})
	}
}

// A pending challenge restored without a nonce must not collapse the token to the prefix "AGENT-".
// That stub is printed to the applicant in the instruction, so it would be both guessable and
// liable to fire on a reply that merely starts with it.
func TestAnEmptyNonceFallsBackToTheFixedToken(t *testing.T) {
	if got := AgentToken("ab12cd"); got != "AGENT-AB12CD" {
		t.Fatalf("AgentToken(%q) = %q, want %q", "ab12cd", got, "AGENT-AB12CD")
	}
	if got := AgentToken(""); got != "AGENT-STOP" {
		t.Errorf("AgentToken(\"\") = %q, want %q: the instruction shows a degenerate token that any reply can guess", got, "AGENT-STOP")
	}
	if AgentReply("AGENT- model=gpt-5.2", "") {
		t.Error("AgentReply(\"AGENT- model=gpt-5.2\", \"\") = true: a reply that only starts with the prefix was declined and banned as an automated agent")
	}
	if !AgentReply("AGENT-STOP model=gpt-5.2", "") {
		t.Error("the fixed token no longer catches an agent that answered the instruction as printed")
	}
}
