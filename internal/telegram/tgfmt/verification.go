package tgfmt

import (
	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

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
