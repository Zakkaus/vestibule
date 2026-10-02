package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestPkgKeywordLegend(t *testing.T) {
	renderers := []struct {
		Name string
		Fn   func(i18n.Lang) string
	}{
		{
			Name: "plain",
			Fn: func(l i18n.Lang) string {
				return tgfmt.RenderPkg(l, "vim", lookup.PackageResult{Official: []string{"app-editors/vim"}, Versions: map[string][2]string{"app-editors/vim": {"", "9.1"}}, Availability: lookup.TestPkgLookupAvailability{Official: true}})
			},
		},
		{
			Name: "rich",
			Fn: func(l i18n.Lang) string {
				return tgfmt.RenderPkgRich(l, "vim", lookup.PackageResult{Official: []string{"app-editors/vim"}, Versions: map[string][2]string{"app-editors/vim": {"", "9.1"}}, Availability: lookup.TestPkgLookupAvailability{Official: true}})
			},
		},
	}
	for _, l := range i18n.Languages() {
		for _, tt := range renderers {
			t.Run(l.String()+"/"+tt.Name, func(t *testing.T) {
				got := tt.Fn(l)
				legend := i18n.Messages.LookupPackages.Pkg.KeywordLegend.For(l)
				if !strings.Contains(got, legend) {
					t.Errorf("rendered package result %q does not contain legend %q", got, legend)
				}
				if !strings.Contains(got, "~9.1") {
					t.Errorf("rendered package result does not mark the no-amd64-stable latest version: %q", got)
				}
			})
		}
	}
}
