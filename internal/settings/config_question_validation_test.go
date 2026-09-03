package settings

import (
	"strings"
	"testing"
)

// Eighteen rules refuse a configuration file the instance cannot work from. Fourteen fail at
// least one test when made to accept everything. These four failed none, and all four are
// about the questions an applicant is actually asked -- a configuration that loads with them
// broken produces a challenge nobody can answer, which declines every real applicant.
func TestConfigRefusesQuestionsNobodyCanAnswer(t *testing.T) {
	question := func(options []string, answer int) map[string]any {
		return map[string]any{"q": "pick one", "options": options, "answer": answer}
	}
	for _, tc := range []struct {
		name   string
		config map[string]any
		want   string
	}{
		{
			name:   "a global question with one option",
			config: map[string]any{"questions": []any{question([]string{"only"}, 0)}},
			want:   "need at least 2 options",
		},
		{
			name:   "a global question whose answer is not one of its options",
			config: map[string]any{"questions": []any{question([]string{"a", "b"}, 2)}},
			want:   "answer index 2 out of range",
		},
		{
			name: "a group question with one option",
			config: map[string]any{
				"group_ids": []any{-1009000001200},
				"groups": []any{map[string]any{
					"id": -1009000001200, "questions": []any{question([]string{"only"}, 0)},
				}},
			},
			want: "need at least 2 options",
		},
		{
			name: "a fallback question with no answer",
			config: map[string]any{
				"fallback_questions": []any{map[string]any{"q": "name it", "answers": []any{}}},
			},
			want: "at least one answers entry",
		},
		{
			name: "a fallback answer that is blank",
			config: map[string]any{
				"fallback_questions": []any{map[string]any{"q": "name it", "answers": []any{"  "}}},
			},
			want: "must not contain an empty string",
		},
		{
			name:   "an overlay that is not owner/name",
			config: map[string]any{"overlays": []any{map[string]any{"repo": "gentoo-zh"}}},
			want:   `repo must be "owner/name"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, tc.config))
			if err == nil {
				t.Fatalf("the configuration loaded with %s; the instance would start and ask "+
					"something no applicant can answer", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal = %q, want it to name %q so an operator can find the entry",
					err, tc.want)
			}
		})
	}
}
