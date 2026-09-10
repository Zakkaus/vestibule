package rules

import (
	"strings"
	"testing"
)

// The count is compared between locales rather than against a literal: what has
// to hold is that adding a question adds it everywhere, and a literal turns that
// into a number somebody edits alongside the file it was meant to guard.
func TestFactoryFallbackQuestionsCoverEveryLocale(t *testing.T) {
	want := len(FactoryFallbackQuestions(factoryFallbackLocales[0]))
	if want == 0 {
		t.Fatal("the factory fallback bank is empty, so no applicant without Linux can be asked anything")
	}
	for _, locale := range factoryFallbackLocales {
		questions := FactoryFallbackQuestions(locale)
		if len(questions) != want {
			t.Fatalf("%s questions = %d, want %d — a question exists in one locale and not another",
				locale, len(questions), want)
		}
		for index, question := range questions {
			if strings.TrimSpace(question.Prompt) == "" || len(question.Answers) == 0 {
				t.Errorf("%s question %d is incomplete: %#v", locale, index, question)
			}
			for _, answer := range question.Answers {
				if strings.EqualFold(strings.TrimSpace(question.Prompt), strings.TrimSpace(answer)) {
					t.Errorf("%s question %d exposes its answer", locale, index)
				}
			}
		}
	}
}

func TestFactoryFallbackQuestionsReturnsDetachedAnswers(t *testing.T) {
	questions := FactoryFallbackQuestions("en")
	questions[0].Answers[0] = "changed"
	if got := FactoryFallbackQuestions("en")[0].Answers[0]; got != "kernel.org" {
		t.Fatalf("factory answer mutated through caller: %q", got)
	}
}
