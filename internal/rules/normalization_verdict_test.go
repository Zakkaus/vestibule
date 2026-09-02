package rules

import "testing"

func TestRuleVerdictsAreStableAcrossNormalizedSpellings(t *testing.T) {
	kernel := VersionRange{Intervals: []VersionInterval{{
		Minimum: Version{Major: 6, Minor: 0},
		Maximum: Version{Major: 6, Minor: 99},
	}}}
	tests := []struct {
		name      string
		rule      Rule
		canonical string
		variants  []string
	}{
		{
			name: "whole value rejection",
			rule: Rule{
				Accept: []Condition{Contains{Value: "gentoo"}},
				Reject: []Condition{OneOf{Values: []string{"gentoo@emerge"}}},
			},
			canonical: "gentoo@emerge",
			variants:  []string{"Ｇｅｎｔｏｏ＠Ｅｍｅｒｇｅ", "gen\u200btoo@emerge"},
		},
		{
			name:      "CJK substring acceptance",
			rule:      Rule{Accept: []Condition{Contains{Value: "验证码答"}}},
			canonical: "验证码答",
			variants:  []string{"验·证*码-答", "验　证_码.答"},
		},
		{
			name:      "numeric acceptance",
			rule:      Rule{Accept: []Condition{NumberRange{Minimum: 45, Maximum: 45}}},
			canonical: "minute 45",
			variants:  []string{"ＭＩＮＵＴＥ　４５", "minute 4\u20605"},
		},
		{
			name:      "kernel release acceptance",
			rule:      Rule{Accept: []Condition{kernel}},
			canonical: "6.12.3-gentoo",
			variants:  []string{"６．１２．３－ＧＥＮＴＯＯ", "6.\u200b12.3-gentoo"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := test.rule.Evaluate(test.canonical)
			if want == NoMatch {
				t.Fatalf("canonical input %q did not establish a verdict", test.canonical)
			}
			for _, variant := range test.variants {
				if got := test.rule.Evaluate(variant); got != want {
					t.Errorf("Evaluate(%q) = %v, want %v from canonical %q: spelling changed the rule conclusion", variant, got, want, test.canonical)
				}
			}
		})
	}
}
