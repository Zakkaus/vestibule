package tgfmt

import (
	"html"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/rules"
	"github.com/mymmrac/telego"
)

// KernelQuestion returns the localized kernel challenge text persisted with a pending verification.
func KernelQuestion(messages *i18n.Catalog, l i18n.Lang) string {
	return messages.Verification.Challenge.KernelQuestion.For(l)
}

// KernelPromptHTML renders the expandable and legacy-compatible kernel challenge prompt.
func KernelPromptHTML(messages *i18n.Catalog, l i18n.Lang, question string, left int, nonce string, expandable, held bool) string {
	if left < 1 {
		left = 1
	}
	template := messages.Verification.Challenge.KernelPrompt
	if held {
		template = messages.Verification.Challenge.KernelPromptHeld
	}
	prompt := template.Render(l, html.EscapeString(question), left)
	return prompt + "\n\n" + AgentTrapLine(messages, l, nonce, expandable)
}

// FallbackPromptHTML renders the answer-hidden fallback question and nonce-bound instruction.
func FallbackPromptHTML(messages *i18n.Catalog, l i18n.Lang, question string, left int, nonce string, expandable bool) string {
	if left < 1 {
		left = 1
	}
	prompt := messages.Verification.Challenge.FallbackIntro.Render(l, html.EscapeString(question), left)
	return prompt + "\n\n" + AgentTrapLine(messages, l, nonce, expandable)
}

// AgentTrapLine renders the hidden nonce-bound automated-agent instruction.
func AgentTrapLine(messages *i18n.Catalog, l i18n.Lang, nonce string, expandable bool) string {
	body := messages.Verification.Challenge.AgentTrap.Render(l, rules.AgentToken(nonce))
	if expandable {
		return "<blockquote expandable>" + body + "</blockquote>"
	}
	return body
}

// ModeName returns the operator-facing label for a challenge mode.
func ModeName(l i18n.Lang, mode string) string {
	labels := &i18n.Messages.Verification.Mode
	switch mode {
	case config.ModeKernel:
		return labels.Kernel.For(l)
	case config.ModeQuiz:
		return labels.Quiz.For(l)
	case config.ModeMixed:
		return labels.Mixed.For(l)
	}
	return mode
}

func DisplayName(user *telego.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	return user.FirstName
}
