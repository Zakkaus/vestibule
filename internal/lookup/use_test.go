package lookup

import (
	"context"
	"testing"
)

func TestResolveUseSourcesAvailability(t *testing.T) {
	for _, tc := range []struct {
		name        string
		query       string
		atoms       []string
		found       bool
		officialOK  bool
		wantSources int
		unavailable bool
	}{
		{name: "bare outage", query: "vim", unavailable: true},
		{name: "bare answered miss", query: "vim", officialOK: true},
		{name: "bare found", query: "vim", atoms: []string{"app-editors/vim"}, officialOK: true, wantSources: 1},
		{name: "exact outage", query: "app-editors/vim", unavailable: true},
		{name: "exact 404", query: "app-editors/vim", officialOK: true},
		{name: "exact found", query: "app-editors/vim", found: true, officialOK: true, wantSources: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srcs, availability := resolveUseSourcesWith(
				context.Background(),
				tc.query,
				map[string]bool{"guru": true},
				func(context.Context, string) (PkgFullInfo, bool, bool) {
					return PkgFullInfo{}, tc.found, tc.officialOK
				},
				func(context.Context, string) ([]string, bool) {
					return tc.atoms, tc.officialOK
				},
			)
			if len(srcs) != tc.wantSources {
				t.Errorf("resolveUseSourcesWith() returned %d sources, want %d", len(srcs), tc.wantSources)
			}
			if got := availability.AnyUnavailable(); got != tc.unavailable {
				t.Errorf("availability.anyUnavailable() = %v, want %v", got, tc.unavailable)
			}
		})
	}
}

func TestResolveUseSourcesAcceptsOnlyExactPackageNames(t *testing.T) {
	resetLookupPackageCaches(t)
	PkgC.mu.Lock()
	PkgC.pkgs["guru"] = map[string]string{
		"app-editors/vim":      "9.1",
		"app-editors/vim-core": "9.1",
	}
	PkgC.mu.Unlock()

	srcs, _ := resolveUseSourcesWith(
		context.Background(),
		"vim",
		map[string]bool{"guru": true},
		func(context.Context, string) (PkgFullInfo, bool, bool) {
			return PkgFullInfo{}, false, true
		},
		func(context.Context, string) ([]string, bool) {
			return []string{"app-editors/vim", "app-editors/neovim"}, true
		},
	)

	exact, ok := srcs["app-editors/vim"]
	if !ok || !exact.Official || len(exact.Ovs) != 1 || exact.Ovs[0] != "guru" {
		t.Fatalf("valid exact /use match was not retained from both sources: %+v", exact)
	}
	for atom := range srcs {
		if atom != "app-editors/vim" {
			t.Errorf("bare /use vim treated fuzzy package %q as an exact match", atom)
		}
	}
}

func TestUSEFlagSignsAreRemovedAndPlusMeansDefaultEnabled(t *testing.T) {
	got := toUseFlags([]useEntry{
		{Name: "+ssl", Description: "TLS support"},
		{Name: "-bindist", Description: "Distribution restriction"},
		{Name: "nls", Description: "Native language support"},
	})
	want := []struct {
		name string
		def  bool
	}{
		{name: "ssl", def: true},
		{name: "bindist", def: false},
		{name: "nls", def: false},
	}
	if len(got) != len(want) {
		t.Fatalf("toUseFlags() returned %d flags, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i].name || got[i].Def != want[i].def {
			t.Errorf(
				"USE flag %q became name=%q default=%v, want name=%q default=%v; +/- prefixes are metadata, not part of the linked flag name",
				got[i].Name, got[i].Name, got[i].Def, want[i].name, want[i].def,
			)
		}
	}
}
