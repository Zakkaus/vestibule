package lookup_test

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestBugLookupFailureMessage(t *testing.T) {
	const (
		id   = "123"
		link = "https://bugs.gentoo.org/123"
	)
	l := i18n.LangZH
	tests := []struct {
		Name  string
		State lookup.TestBugLookupState
		Want  string
	}{
		{
			Name:  "not found",
			State: lookup.TestBugLookupNotFound,
			Want:  i18n.Messages.LookupContent.Bug.NotFound.Render(l, id),
		},
		{
			Name:  "temporary failure",
			State: lookup.TestBugLookupUnavailable,
			Want:  i18n.Messages.LookupContent.Bug.Unavailable.Render(l, id, link),
		},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			if got := tgfmt.BugLookupFailureMessage(l, id, link, tt.State); got != tt.Want {
				t.Errorf("bugLookupFailureMessage() = %q, want %q", got, tt.Want)
			}
		})
	}
}
