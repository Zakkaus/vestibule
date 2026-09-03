package rules

import (
	"strings"
	"testing"
)

// The fallback bank is the escape hatch offered to an applicant who declares they have no Linux
// machine. It ships inside the binary, so nothing downstream can reject a bad edit to it; the
// loader's refusals at package init are the only thing standing between a mistake in the
// provisioning file and every such applicant being handed a broken or self-answering question.

const wellFormedFallbackFile = `{
  "fallback_questions": [
    {
      "id": "linux-kernel-website",
      "prompts": {
        "zh": "问题一",
        "zh-Hant": "問題一",
        "en": "What is the domain name of the official Linux kernel website?"
      },
      "answers": ["kernel.org", "www.kernel.org"]
    }
  ]
}`

func loadFactoryFallback(t *testing.T, data string) (questions map[string][]FallbackQuestion, refusal string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			refusal = strings.TrimSpace(strings.Join(strings.Fields(panicMessage(recovered)), " "))
		}
	}()
	return mustLoadFactoryFallback([]byte(data)), ""
}

// panicMessage renders whatever the loader panicked with, so a refusal can be reported as text.
func panicMessage(value any) string {
	if err, ok := value.(error); ok {
		return err.Error()
	}
	if text, ok := value.(string); ok {
		return text
	}
	return "non-string panic value"
}

// The positive control: a well-formed file loads, every locale gets the question, and the answers
// arrive trimmed. Without this the refusals below would prove only that the loader rejects
// everything.
func TestAWellFormedProvisioningFileLoads(t *testing.T) {
	questions, refusal := loadFactoryFallback(t, wellFormedFallbackFile)
	if refusal != "" {
		t.Fatalf("a well-formed provisioning file was refused: %s", refusal)
	}
	for _, locale := range factoryFallbackLocales {
		got := questions[locale]
		if len(got) != 1 {
			t.Fatalf("%s questions = %d, want 1: an applicant with no Linux would be offered no escape at all", locale, len(got))
		}
		if got[0].Prompt == "" {
			t.Errorf("%s prompt is empty", locale)
		}
		if len(got[0].Answers) != 2 || got[0].Answers[0] != "kernel.org" {
			t.Errorf("%s answers = %#v, want the two provisioned answers", locale, got[0].Answers)
		}
	}
}

// Answers are trimmed before they are stored, so an answer typed with a stray space still matches
// the applicant's reply and cannot slip past the duplicate check as a second distinct answer.
func TestProvisionedAnswersAreTrimmedBeforeTheyAreStored(t *testing.T) {
	padded := strings.Replace(wellFormedFallbackFile, `"answers": ["kernel.org", "www.kernel.org"]`,
		`"answers": ["  kernel.org  ", "www.kernel.org"]`, 1)
	questions, refusal := loadFactoryFallback(t, padded)
	if refusal != "" {
		t.Fatalf("a padded answer was refused outright: %s", refusal)
	}
	if got := questions["en"][0].Answers[0]; got != "kernel.org" {
		t.Errorf("stored answer = %q, want %q: an applicant who types the answer exactly is graded wrong", got, "kernel.org")
	}
}

// replaceInFixture edits the well-formed provisioning file, failing loudly if the anchor moved.
func replaceInFixture(old, new string) string {
	if !strings.Contains(wellFormedFallbackFile, old) {
		panic("the provisioning fixture no longer contains " + old)
	}
	return strings.Replace(wellFormedFallbackFile, old, new, 1)
}

// badProvisioningFile is one edit to the provisioning file the loader must refuse, and what an
// applicant loses when it ships instead.
type badProvisioningFile struct {
	name string
	harm string
	data string
}

var badProvisioningFiles = []badProvisioningFile{
	{
		name: "no questions at all",
		harm: "an applicant with no Linux is offered an empty question bank",
		data: `{"fallback_questions": []}`,
	},
	{
		name: "an unknown field",
		harm: "a mistyped key is silently ignored and the question it was meant to configure ships unset",
		data: replaceInFixture(`"id": "linux-kernel-website",`, `"identifier": "linux-kernel-website", "id": "linux-kernel-website",`),
	},
	{
		name: "trailing data after the document",
		harm: "half the file is loaded and the questions after it never reach an applicant",
		data: wellFormedFallbackFile + "\n{}",
	},
	{
		name: "a blank question id",
		harm: "two questions collapse into one and the duplicate check has nothing to compare",
		data: replaceInFixture(`"id": "linux-kernel-website"`, `"id": "   "`),
	},
	{
		name: "a duplicate question id",
		harm: "the same question is drawn twice and the bank is half the size it claims",
		data: replaceInFixture(`  ]
}`, `  ,
    {
      "id": "linux-kernel-website",
      "prompts": {"zh": "问题二", "zh-Hant": "問題二", "en": "Second question?"},
      "answers": ["gnu.org"]
    }
  ]
}`),
	},
	{
		name: "a missing locale",
		harm: "an applicant reading that locale is shown an empty question",
		data: replaceInFixture(`"zh-Hant": "問題一",
        `, ``),
	},
	{
		name: "an unknown locale",
		harm: "the prompt is written for a locale nobody is served in, so one real locale is missing",
		data: replaceInFixture(`"zh-Hant": "問題一"`, `"ja": "問題一"`),
	},
	{
		name: "a blank prompt",
		harm: "the applicant is asked nothing and can only guess",
		data: replaceInFixture(`"en": "What is the domain name of the official Linux kernel website?"`, `"en": "   "`),
	},
	{
		name: "no answers",
		harm: "no reply can ever be right, so the applicant is declined and banned after three tries",
		data: replaceInFixture(`"answers": ["kernel.org", "www.kernel.org"]`, `"answers": []`),
	},
	{
		name: "a blank answer",
		harm: "an empty answer normalizes away, and a reply that also normalizes away would be graded correct",
		data: replaceInFixture(`"kernel.org", "www.kernel.org"`, `"kernel.org", "  "`),
	},
	{
		name: "a duplicate answer",
		harm: "the bank claims more accepted spellings than it has",
		data: replaceInFixture(`"kernel.org", "www.kernel.org"`, `"kernel.org", "KERNEL.ORG"`),
	},
	{
		name: "a duplicate answer that differs only by spacing",
		harm: "the same answer is stored twice because it was never trimmed",
		data: replaceInFixture(`"kernel.org", "www.kernel.org"`, `"kernel.org", " kernel.org "`),
	},
	{
		name: "a prompt that states its own answer",
		harm: "the escape hatch tells every spam account the answer and lets it into the group",
		data: replaceInFixture(`"en": "What is the domain name of the official Linux kernel website?"`, `"en": "kernel.org"`),
	},
}

func TestABadProvisioningFileIsRefusedAtStartup(t *testing.T) {
	for _, test := range badProvisioningFiles {
		t.Run(test.name, func(t *testing.T) {
			if _, refusal := loadFactoryFallback(t, test.data); refusal == "" {
				t.Errorf("the provisioning file was accepted with %s: %s", test.name, test.harm)
			}
		})
	}
}
