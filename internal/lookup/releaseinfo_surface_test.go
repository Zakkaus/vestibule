package lookup_test

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestUbuntuRelabelStandardSupportEnd(t *testing.T) {
	lookup.TestWithUbuntuSupportEnd(t)
	tests := []struct {
		raw, officialSuffix  string
		standardSupportEnded bool
	}{
		{raw: "18.04", officialSuffix: " LTS", standardSupportEnded: true},
		{raw: "22.10", standardSupportEnded: true},
		{raw: "24.04", officialSuffix: " LTS"},
		{raw: "99.99"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			want := tt.raw + tt.officialSuffix
			if tt.standardSupportEnded {
				want += i18n.Messages.LookupDistros.Release.StandardSupportEnded.For(i18n.LangZH)
			}
			if got := tgfmt.ReleaseText(i18n.LangZH, lookup.TestUbuntuRelabel(tt.raw)); got != want {
				t.Errorf("ubuntuRelabel(%q) = %q, want catalogue rendering %q", tt.raw, got, want)
			}
		})
	}
}
