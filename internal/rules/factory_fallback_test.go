package rules

import (
	"strings"
	"testing"
)

func TestFactoryFallbackQuestionsCoverEveryLocale(t *testing.T) {
	for _, locale := range factoryFallbackLocales {
		questions := FactoryFallbackQuestions(locale)
		if len(questions) != 2 {
			t.Fatalf("%s questions = %d, want 2", locale, len(questions))
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
