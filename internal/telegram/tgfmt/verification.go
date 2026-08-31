package tgfmt

import (
	"fmt"
	"html"

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

func VerificationBanDurationText(messages *i18n.Catalog, l i18n.Lang, seconds int) string {
	duration := &messages.Verification.Duration
	if seconds <= 0 {
		return duration.Permanent.For(l)
	}
	switch {
	case seconds%86400 == 0:
		return duration.Days.Render(l, seconds/86400)
	case seconds%3600 == 0:
		return duration.Hours.Render(l, seconds/3600)
	case seconds%60 == 0:
		return duration.Minutes.Render(l, seconds/60)
	default:
		return duration.Seconds.Render(l, seconds)
	}
}

// Spoilered names use one non-nested entity so hostile names cannot break challenge HTML.
// Admin buttons act by ID, so losing the clickable mention does not affect moderation.
func JoinerLabel(uid int64, name string, spoiler bool) string {
	esc := html.EscapeString(name)
	if spoiler {
		return "<tg-spoiler>" + esc + "</tg-spoiler>"
	}
	return fmt.Sprintf("<a href=\"tg://user?id=%d\">%s</a>", uid, esc)
}

func DisplayName(user *telego.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	return user.FirstName
}
