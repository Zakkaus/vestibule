package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestRenderPkgAvailability(t *testing.T) {
	renderers := []struct {
		Name string
		Fn   func(i18n.Lang, []string, lookup.TestPkgLookupAvailability) string
	}{
		{
			Name: "plain",
			Fn: func(l i18n.Lang, main []string, availability lookup.TestPkgLookupAvailability) string {
				return tgfmt.RenderPkg(l, "vim", lookup.PackageResult{Official: main, Versions: map[string][2]string{}, Availability: availability})
			},
		},
		{
			Name: "rich",
			Fn: func(l i18n.Lang, main []string, availability lookup.TestPkgLookupAvailability) string {
				return tgfmt.RenderPkgRich(l, "vim", lookup.PackageResult{Official: main, Versions: map[string][2]string{}, Availability: availability})
			},
		},
	}
	cases := []struct {
		Name         string
		Main         []string
		Availability lookup.TestPkgLookupAvailability
		Want         func(i18n.Lang, lookup.TestPkgLookupAvailability) string
		NotWant      func(i18n.Lang, lookup.TestPkgLookupAvailability) string
	}{
		{
			Name:         "complete miss",
			Availability: lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"guru": true}},
			Want: func(l i18n.Lang, _ lookup.TestPkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.NotFound.For(l)
			},
			NotWant: func(l i18n.Lang, availability lookup.TestPkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.Unavailable.Render(l, tgfmt.UnavailableSources(l, availability))
			},
		},
		{
			Name:         "lookup unavailable",
			Availability: lookup.TestPkgLookupAvailability{Overlays: map[string]bool{"guru": true}},
			Want: func(l i18n.Lang, availability lookup.TestPkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.Unavailable.Render(l, tgfmt.UnavailableSources(l, availability))
			},
			NotWant: func(l i18n.Lang, _ lookup.TestPkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Pkg.NotFound.For(l)
			},
		},
		{
			Name:         "partial hit",
			Main:         []string{"app-editors/vim"},
			Availability: lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"guru": false}},
			Want: func(l i18n.Lang, availability lookup.TestPkgLookupAvailability) string {
				return i18n.Messages.LookupPackages.Source.PartialResults.Render(l, tgfmt.UnavailableSources(l, availability))
			},
		},
	}
	for _, l := range i18n.Languages() {
		for _, renderer := range renderers {
			for _, tc := range cases {
				t.Run(l.String()+"/"+renderer.Name+"/"+tc.Name, func(t *testing.T) {
					got := renderer.Fn(l, tc.Main, tc.Availability)
					want := tc.Want(l, tc.Availability)
					if !strings.Contains(got, want) {
						t.Errorf("rendered result %q does not contain %q", got, want)
					}
					if tc.NotWant != nil {
						notWant := tc.NotWant(l, tc.Availability)
						if strings.Contains(got, notWant) {
							t.Errorf("rendered result %q unexpectedly contains %q", got, notWant)
						}
					}
				})
			}
		}
	}
}
