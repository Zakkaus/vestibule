package rules

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var factoryFallbackLocales = [...]string{"zh", "zh-Hant", "en"}

// FallbackQuestion is one answer-hidden question from factory provisioning.
type FallbackQuestion struct {
	Prompt  string
	Answers []string
}

type provisionedFallbackQuestion struct {
	ID      string            `json:"id"`
	Prompts map[string]string `json:"prompts"`
	Answers []string          `json:"answers"`
}

type factoryFallbackFile struct {
	FallbackQuestions []provisionedFallbackQuestion `json:"fallback_questions"`
}

//go:embed provisioning/fallback_questions.json
var factoryFallbackData []byte

var factoryFallbackQuestions = mustLoadFactoryFallback(factoryFallbackData)

// FactoryFallbackQuestions returns a detached copy of the answer-hidden factory bank for locale.
func FactoryFallbackQuestions(locale string) []FallbackQuestion {
	questions := factoryFallbackQuestions[locale]
	out := make([]FallbackQuestion, len(questions))
	for i, question := range questions {
		out[i] = FallbackQuestion{Prompt: question.Prompt, Answers: append([]string(nil), question.Answers...)}
	}
	return out
}

func mustLoadFactoryFallback(data []byte) map[string][]FallbackQuestion {
	file := decodeFactoryFallback(data)
	if len(file.FallbackQuestions) == 0 {
		panic("rules: factory fallback provisioning has no questions")
	}

	questions := make(map[string][]FallbackQuestion, len(factoryFallbackLocales))
	seenIDs := make(map[string]bool, len(file.FallbackQuestions))
	for index, question := range file.FallbackQuestions {
		id := validateFallbackQuestionID(index, question.ID, seenIDs)
		validateFallbackPrompts(id, question.Prompts)
		answers := normalizeFallbackAnswers(id, question.Answers)
		appendFactoryFallback(questions, id, question.Prompts, answers)
	}
	return questions
}

func decodeFactoryFallback(data []byte) factoryFallbackFile {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var file factoryFallbackFile
	if err := decoder.Decode(&file); err != nil {
		panic(fmt.Errorf("rules: decode factory fallback provisioning: %w", err))
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		panic("rules: factory fallback provisioning has trailing data")
	}
	return file
}

func validateFallbackQuestionID(index int, rawID string, seen map[string]bool) string {
	id := strings.TrimSpace(rawID)
	if id == "" || seen[id] {
		panic(fmt.Sprintf("rules: factory fallback question %d has an empty or duplicate id", index))
	}
	seen[id] = true
	return id
}

func validateFallbackPrompts(id string, prompts map[string]string) {
	if len(prompts) != len(factoryFallbackLocales) {
		panic(fmt.Sprintf("rules: factory fallback question %q does not define every locale", id))
	}
	for locale, prompt := range prompts {
		if !isFactoryFallbackLocale(locale) || strings.TrimSpace(prompt) == "" {
			panic(fmt.Sprintf("rules: factory fallback question %q has invalid locale %q", id, locale))
		}
	}
}

func isFactoryFallbackLocale(locale string) bool {
	for _, known := range factoryFallbackLocales {
		if locale == known {
			return true
		}
	}
	return false
}

func normalizeFallbackAnswers(id string, rawAnswers []string) []string {
	if len(rawAnswers) == 0 {
		panic(fmt.Sprintf("rules: factory fallback question %q has no answers", id))
	}
	answers := make([]string, len(rawAnswers))
	seen := make(map[string]bool, len(rawAnswers))
	for index, rawAnswer := range rawAnswers {
		answer := strings.TrimSpace(rawAnswer)
		key := strings.ToLower(answer)
		if answer == "" || seen[key] {
			panic(fmt.Sprintf("rules: factory fallback question %q has an empty or duplicate answer", id))
		}
		seen[key] = true
		answers[index] = answer
	}
	return answers
}

func appendFactoryFallback(
	questions map[string][]FallbackQuestion,
	id string,
	prompts map[string]string,
	answers []string,
) {
	for _, locale := range factoryFallbackLocales {
		prompt := strings.TrimSpace(prompts[locale])
		if promptExposesFallbackAnswer(prompt, answers) {
			panic(fmt.Sprintf("rules: factory fallback question %q exposes its answer in %s", id, locale))
		}
		questions[locale] = append(questions[locale], FallbackQuestion{
			Prompt: prompt, Answers: answers,
		})
	}
}

func promptExposesFallbackAnswer(prompt string, answers []string) bool {
	for _, answer := range answers {
		if strings.EqualFold(prompt, answer) {
			return true
		}
	}
	return false
}
