package lookup_test

import (
	"context"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestLookupArmAvailability(t *testing.T) {
	for _, l := range i18n.Languages() {
		for _, tc := range []struct {
			name      string
			atoms     []string
			available bool
			want      func() string
			notWant   func() string
			wantHTML  bool
		}{
			{
				name: "search unavailable",
				want: func() string {
					return i18n.Messages.LookupPackages.Arm.OfficialUnavailable.For(l)
				},
				notWant: func() string {
					return i18n.Messages.LookupPackages.Arm.NotFound.Render(l, "firefox")
				},
			},
			{
				name:      "answered miss",
				available: true,
				want: func() string {
					return i18n.Messages.LookupPackages.Arm.NotFound.Render(l, "firefox")
				},
				notWant: func() string {
					return i18n.Messages.LookupPackages.Arm.OfficialUnavailable.For(l)
				},
			},
			{
				name:      "package found",
				atoms:     []string{"www-client/firefox"},
				available: true,
				want: func() string {
					return i18n.Messages.LookupPackages.Arm.StableOnly.Render(l, "140.12.0")
				},
				wantHTML: true,
			},
		} {
			t.Run(l.String()+"/"+tc.name, func(t *testing.T) {
				got, useHTML := tgfmt.RenderArm(l, "firefox", lookup.LookupArm(context.Background(), "firefox", func(context.Context, string) ([]string, bool) { return tc.atoms, tc.available }, func(context.Context, string) (string, string, bool) { return "140.12.0", "", true }))
				if useHTML != tc.wantHTML {
					t.Errorf("lookupArm() useHTML = %v, want %v", useHTML, tc.wantHTML)
				}
				want := tc.want()
				if !strings.Contains(got, want) {
					t.Errorf("lookupArm() = %q, want substring %q", got, want)
				}
				if tc.notWant != nil {
					notWant := tc.notWant()
					if strings.Contains(got, notWant) {
						t.Errorf("lookupArm() = %q, unwanted substring %q", got, notWant)
					}
				}
			})
		}
	}
}
