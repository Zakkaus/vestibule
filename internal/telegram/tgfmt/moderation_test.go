package tgfmt

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestModerationBanDurationText(t *testing.T) {
	for seconds, want := range map[int]string{
		0:      i18n.Messages.Moderate.Duration.Permanent.For(i18n.LangZH),
		-1:     i18n.Messages.Moderate.Duration.Permanent.For(i18n.LangZH),
		604800: i18n.Messages.Moderate.Duration.Days.Render(i18n.LangZH, 7),
		43200:  i18n.Messages.Moderate.Duration.Hours.Render(i18n.LangZH, 12),
		1800:   i18n.Messages.Moderate.Duration.Minutes.Render(i18n.LangZH, 30),
		90:     i18n.Messages.Moderate.Duration.Seconds.Render(i18n.LangZH, 90),
	} {
		if got := ModerationBanDurationText(i18n.LangZH, seconds); got != want {
			t.Errorf("ModerationBanDurationText(%d) = %q, want %q", seconds, got, want)
		}
	}
}
