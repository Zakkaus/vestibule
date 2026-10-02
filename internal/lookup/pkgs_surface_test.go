package lookup_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

type repologyAvailabilityCase struct {
	Name       string
	ExactRows  []lookup.TestRepologyPkg
	ExactErr   error
	SearchRows map[string][]lookup.TestRepologyPkg
	SearchErr  error
	Available  bool
	WantPkgs   int
	WantText   string
	NotWant    string
}

func repologyAvailabilityCases(lookupName string) []repologyAvailabilityCase {
	messages := i18n.Messages.LookupDistros.Pkgs
	notFound := messages.RepologyNotFound.Render(i18n.LangZH, lookupName)
	unavailable := messages.RepologyUnavailable.Render(i18n.LangZH, lookupName)
	return []repologyAvailabilityCase{
		{
			Name:      "exact result",
			ExactRows: []lookup.TestRepologyPkg{{Repo: "gentoo", Version: "9.1"}},
			Available: true,
			WantPkgs:  1,
		},
		{
			Name:      "answered miss",
			ExactErr:  &lookup.TestHttpStatusError{Url: "u", Code: 404},
			Available: true,
			WantText:  notFound,
			NotWant:   unavailable,
		},
		{
			Name:     "rate limited",
			ExactErr: &lookup.TestHttpStatusError{Url: "u", Code: 429},
			WantText: unavailable,
			NotWant:  notFound,
		},
		{
			Name:     "server failure",
			ExactErr: &lookup.TestHttpStatusError{Url: "u", Code: 503},
			WantText: unavailable,
			NotWant:  notFound,
		},
		{
			Name:     "network failure",
			ExactErr: errors.New("connection reset"),
			WantText: unavailable,
			NotWant:  notFound,
		},
		{
			Name:     "outbound busy",
			ExactErr: &lookup.TestHttpBusyError{Url: "u"},
			WantText: unavailable,
			NotWant:  notFound,
		},
		{
			Name:       "search failure",
			ExactErr:   &lookup.TestHttpStatusError{Url: "u", Code: 404},
			SearchErr:  &lookup.TestHttpStatusError{Url: "u", Code: 503},
			WantText:   unavailable,
			NotWant:    notFound,
			SearchRows: map[string][]lookup.TestRepologyPkg{},
		},
	}
}

func TestFetchRepologyAvailability(t *testing.T) {
	const lookupName = "vim"
	for _, tc := range repologyAvailabilityCases(lookupName) {
		t.Run(tc.Name, func(t *testing.T) {
			_, gotPkgs, _, _, available := lookup.TestFetchRepologyWith(
				context.Background(),
				lookupName,
				func(_ context.Context, url string, dst any) error {
					if strings.Contains(url, "/project/") {
						*dst.(*[]lookup.TestRepologyPkg) = tc.ExactRows
						return tc.ExactErr
					}
					*dst.(*map[string][]lookup.TestRepologyPkg) = tc.SearchRows
					return tc.SearchErr
				},
			)
			if available != tc.Available || len(gotPkgs) != tc.WantPkgs {
				t.Errorf("fetchRepologyWith() returned len=%d available=%v, want len=%d available=%v",
					len(gotPkgs), available, tc.WantPkgs, tc.Available)
			}
			if tc.WantText != "" {
				got := tgfmt.RenderRepologyLookupMiss(i18n.LangZH, lookupName, available)
				if got != tc.WantText {
					t.Errorf("renderRepologyLookupMiss() = %q, want %q", got, tc.WantText)
				}
				if strings.Contains(got, tc.NotWant) {
					t.Errorf("renderRepologyLookupMiss() = %q, unwanted substring %q", got, tc.NotWant)
				}
			}
		})
	}
}
