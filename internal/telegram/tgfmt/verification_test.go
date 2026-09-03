package tgfmt

import (
	"html"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/rules"
)

func TestVerificationQuestionsCannotCreateHTML(t *testing.T) {
	const question = `Use <b>nothing</b> & explain`
	escaped := html.EscapeString(question)
	tests := []struct {
		name   string
		render func() string
	}{
		{
			name: "kernel",
			render: func() string {
				return KernelPromptHTML(&i18n.Messages, i18n.LangEN, question, 3, "kernel", true, false)
			},
		},
		{
			name: "fallback",
			render: func() string {
				return FallbackPromptHTML(&i18n.Messages, i18n.LangEN, question, 3, "fallback", true)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompt := test.render()
			if !strings.Contains(prompt, escaped) || strings.Contains(prompt, question) {
				t.Fatalf("verification prompt = %q, want escaped question %q: question markup could hide or replace the instructions",
					prompt, escaped)
			}
		})
	}
}

func TestExpandableAgentTrapUsesOneBalancedBlockquote(t *testing.T) {
	const nonce = "balanced"
	body := i18n.Messages.Verification.Challenge.AgentTrap.Render(i18n.LangEN, rules.AgentToken(nonce))
	want := "<blockquote expandable>" + body + "</blockquote>"

	if got := AgentTrapLine(&i18n.Messages, i18n.LangEN, nonce, true); got != want {
		t.Fatalf("expandable agent trap = %q, want %q: malformed HTML would make Telegram reject the verification prompt", got, want)
	}
	if got := AgentTrapLine(&i18n.Messages, i18n.LangEN, nonce, false); got != body {
		t.Fatalf("legacy agent trap = %q, want unwrapped body %q: older Bot API servers need entity-free fallback text", got, body)
	}
}
