package rules

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeSpamFixtures(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{name: "zero-width-space.txt", want: "gentoo"},
		{name: "zero-width-non-joiner.txt", want: "gentoo"},
		{name: "word-joiner.txt", want: "gentoo"},
		{name: "cjk-separators.txt", want: "验证码答"},
		{name: "fullwidth.txt", want: "gentoo@emerge"},
	} {
		t.Run(test.name, func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join("..", "..", "testdata", "spam", test.name))
			if err != nil {
				t.Fatal(err)
			}
			if got := Normalize(string(text)); got != test.want {
				t.Errorf("Normalize(%q) = %q, want %q", text, got, test.want)
			}
		})
	}
}

func TestRuleRejectsBeforeAccept(t *testing.T) {
	rule := Rule{
		Accept: []Condition{VersionRange{Intervals: []VersionInterval{{
			Minimum: Version{Major: 6, Minor: 0},
			Maximum: Version{Major: 7, Minor: 99},
		}}}},
		Reject: []Condition{OneOf{Values: []string{"7.1.30"}}},
	}
	if got := rule.Evaluate("7.1.30"); got != Rejected {
		t.Errorf("bait verdict = %v, want %v", got, Rejected)
	}
	if got := rule.Evaluate("6.12.3-gentoo"); got != Accepted {
		t.Errorf("release verdict = %v, want %v", got, Accepted)
	}
}

func TestVersionRangeAcceptsNormalizedProcVersionOutput(t *testing.T) {
	kernel := VersionRange{Intervals: []VersionInterval{{
		Minimum: Version{Major: 6, Minor: 0},
		Maximum: Version{Major: 6, Minor: 99},
	}}}
	if !kernel.Matches("Linux version 6.12.3 (gcc 15.2.0)") {
		t.Error("/proc/version output was not accepted after normalization")
	}
}

func TestOneOfAnswerNormalization(t *testing.T) {
	condition := OneOf{Values: []string{"gentoozh.org"}}
	for _, text := range []string{"GentooZH.org", "https://www.gentoozh.org/", "（gentoozh.org。）"} {
		if !condition.MatchesAnswer(text) {
			t.Errorf("fallback answer %q did not match", text)
		}
	}
}

func TestContainsAndNumberRange(t *testing.T) {
	if !(Contains{Value: "无 Linux 设备", CompactWhitespace: true}).Matches("无　Linux　设备") {
		t.Error("contains condition did not fold whitespace")
	}
	if got, ok := (NumberRange{Minimum: 0, Maximum: 59}).Last("minute １２ then ４５"); !ok || got != 45 {
		t.Errorf("last minute = (%d, %t), want (45, true)", got, ok)
	}
}

func TestAgentReplyAndMinuteProof(t *testing.T) {
	if !AgentReply("agent-abc123 model=gpt-5.2", "abc123") {
		t.Error("nonce-bound agent reply did not match")
	}
	if AgentReply("AGENT-ABC123 model=gpt-5.2 extra", "abc123") {
		t.Error("agent reply with trailing prose matched")
	}
	now := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	if !MinuteProof("现在是３０分", now) {
		t.Error("fullwidth current minute did not match")
	}
}
