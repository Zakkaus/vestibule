package lookup

import (
	"context"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestWriteExpandFlags(t *testing.T) {
	many := make([]useFlag, 0, 20)
	for i := range 20 {
		many = append(many, useFlag{name: "lang" + string(rune('a'+i))})
	}
	groups := []useExpandGroup{
		{name: "llvm_slot", flags: []useFlag{{name: "20"}, {name: "21", def: true}, {name: "22"}}},
		{name: "l10n", flags: many},
	}
	for _, l := range i18n.Languages() {
		var b strings.Builder
		writeExpandFlags(&b, l, groups)
		out := b.String()
		messages := i18n.Messages.LookupPackages.Use

		llvmHeader := "<b>LLVM_SLOT</b>" + messages.Count.Render(l, 3) + messages.ValueSeparator.For(l)
		if !strings.Contains(out, llvmHeader) {
			t.Errorf("missing uppercased llvm_slot header with count %q: %q", llvmHeader, out)
		}
		if !strings.Contains(out, "+21") {
			t.Errorf("a default value must be marked +21: %q", out)
		}
		l10nHeader := "<b>L10N</b>" + messages.Count.Render(l, 20) + messages.ValueSeparator.For(l)
		if !strings.Contains(out, l10nHeader) {
			t.Errorf("missing l10n header with full count %q: %q", l10nHeader, out)
		}
		truncatedCount := messages.TruncatedCount.Render(l, 20)
		if !strings.Contains(out, truncatedCount) {
			t.Errorf("a group past expandCap must truncate with tail %q: %q", truncatedCount, out)
		}
		if n := strings.Count(out, "lang"); n != expandCap {
			t.Errorf("l10n should render exactly expandCap=%d values, got %d", expandCap, n)
		}
	}
}

func TestRenderUseIncludesExpand(t *testing.T) {
	info := pkgFullInfo{
		atom:   "www-client/firefox",
		expand: []useExpandGroup{{name: "l10n", flags: []useFlag{{name: "zh-CN"}, {name: "en", def: true}}}},
	}
	out := renderUse(i18n.LangZH, info, "", "", false, nil)
	if !strings.Contains(out, "L10N") {
		t.Errorf("renderUse should include the L10N use_expand group: %q", out)
	}
	if strings.Contains(out, i18n.Messages.LookupPackages.Use.NoFlags.For(i18n.LangZH)) {
		t.Error("a package with use_expand must not be reported as having no USE flags")
	}
}

func TestRenderUseRichIncludesExpand(t *testing.T) {
	info := pkgFullInfo{
		atom:   "www-client/firefox",
		expand: []useExpandGroup{{name: "llvm_slot", flags: []useFlag{{name: "20"}, {name: "21", desc: "Use LLVM 21.", def: true}}}},
	}
	out := renderUseRich(i18n.LangZH, info, "", "https://packages.gentoo.org/packages/www-client/firefox", false, nil)
	if !strings.Contains(out, "<details>") || !strings.Contains(out, "LLVM_SLOT") {
		t.Errorf("renderUseRich should put USE_EXPAND in a <details> block, got %q", out)
	}
	if !strings.Contains(out, "+21") || !strings.Contains(out, "Use LLVM 21.") {
		t.Errorf("rich USE_EXPAND should show the default marker + description, got %q", out)
	}
}

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
				func(context.Context, string) (pkgFullInfo, bool, bool) {
					return pkgFullInfo{}, tc.found, tc.officialOK
				},
				func(context.Context, string) ([]string, bool) {
					return tc.atoms, tc.officialOK
				},
			)
			if len(srcs) != tc.wantSources {
				t.Errorf("resolveUseSourcesWith() returned %d sources, want %d", len(srcs), tc.wantSources)
			}
			if got := availability.anyUnavailable(); got != tc.unavailable {
				t.Errorf("availability.anyUnavailable() = %v, want %v", got, tc.unavailable)
			}
		})
	}
}

func TestResolveUseSourcesAcceptsOnlyExactPackageNames(t *testing.T) {
	resetLookupPackageCaches(t)
	pkgC.mu.Lock()
	pkgC.pkgs["guru"] = map[string]string{
		"app-editors/vim":      "9.1",
		"app-editors/vim-core": "9.1",
	}
	pkgC.mu.Unlock()

	srcs, _ := resolveUseSourcesWith(
		context.Background(),
		"vim",
		map[string]bool{"guru": true},
		func(context.Context, string) (pkgFullInfo, bool, bool) {
			return pkgFullInfo{}, false, true
		},
		func(context.Context, string) ([]string, bool) {
			return []string{"app-editors/vim", "app-editors/neovim"}, true
		},
	)

	exact, ok := srcs["app-editors/vim"]
	if !ok || !exact.official || len(exact.ovs) != 1 || exact.ovs[0] != "guru" {
		t.Fatalf("valid exact /use match was not retained from both sources: %+v", exact)
	}
	for atom := range srcs {
		if atom != "app-editors/vim" {
			t.Errorf("bare /use vim treated fuzzy package %q as an exact match", atom)
		}
	}
}

func TestRenderUseLookupMiss(t *testing.T) {
	for _, l := range i18n.Languages() {
		for _, tc := range []struct {
			name         string
			availability pkgLookupAvailability
			want         func(pkgLookupAvailability) string
			notWant      func(pkgLookupAvailability) string
		}{
			{
				name:         "answered miss",
				availability: pkgLookupAvailability{official: true, overlays: map[string]bool{"guru": true}},
				want: func(_ pkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.NotFound.Render(l, "vim")
				},
				notWant: func(availability pkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.Unavailable.Render(l, "vim", availability.unavailableSources(l))
				},
			},
			{
				name:         "source unavailable",
				availability: pkgLookupAvailability{overlays: map[string]bool{"guru": true}},
				want: func(availability pkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.Unavailable.Render(l, "vim", availability.unavailableSources(l))
				},
				notWant: func(_ pkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.NotFound.Render(l, "vim")
				},
			},
		} {
			t.Run(l.String()+"/"+tc.name, func(t *testing.T) {
				got := renderUseLookupMiss(l, "vim", tc.availability)
				want := tc.want(tc.availability)
				if got != want {
					t.Errorf("renderUseLookupMiss() = %q, want %q", got, want)
				}
				notWant := tc.notWant(tc.availability)
				if strings.Contains(got, notWant) {
					t.Errorf("renderUseLookupMiss() = %q, unwanted text %q", got, notWant)
				}
			})
		}
	}
}

func TestAppendUseAvailabilityNote(t *testing.T) {
	for _, l := range i18n.Languages() {
		for _, tc := range []struct {
			name         string
			availability pkgLookupAvailability
			wantNote     bool
		}{
			{
				name:         "all answered",
				availability: pkgLookupAvailability{official: true, overlays: map[string]bool{"guru": true}},
			},
			{
				name:         "overlay failed",
				availability: pkgLookupAvailability{official: true, overlays: map[string]bool{"guru": false}},
				wantNote:     true,
			},
		} {
			t.Run(l.String()+"/"+tc.name, func(t *testing.T) {
				plain, rich := appendUseAvailabilityNote(l, "plain result", "<p>rich result</p>", tc.availability)
				note := i18n.Messages.LookupPackages.Source.PartialResults.Render(l, tc.availability.unavailableSources(l))
				for label, got := range map[string]string{"plain": plain, "rich": rich} {
					hasNote := strings.Contains(got, note)
					if hasNote != tc.wantNote {
						t.Errorf("%s output %q contains partial note=%v, want %v", label, got, hasNote, tc.wantNote)
					}
				}
			})
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
		if got[i].name != want[i].name || got[i].def != want[i].def {
			t.Errorf(
				"USE flag %q became name=%q default=%v, want name=%q default=%v; +/- prefixes are metadata, not part of the linked flag name",
				got[i].name, got[i].name, got[i].def, want[i].name, want[i].def,
			)
		}
	}
}

func TestUseRenderingLimitsLocalFlagsToTwelve(t *testing.T) {
	flags := make([]useFlag, 13)
	for i := range flags {
		flags[i] = useFlag{name: "flag-" + string(rune('a'+i))}
	}

	got := renderUse(i18n.LangEN, pkgFullInfo{atom: "app-editors/example", local: flags}, "", "", false, nil)
	if count := strings.Count(got, "\n • "); count != 12 {
		t.Errorf("/use rendered %d local flags; more than twelve risks a Telegram message rejection", count)
	}
	truncated := i18n.Messages.LookupPackages.Use.TruncatedCount.Render(i18n.LangEN, len(flags))
	if !strings.Contains(got, truncated) {
		t.Errorf("/use omitted its local flag truncation notice %q", truncated)
	}
	if strings.Contains(got, ">flag-m</a>") {
		t.Error("/use showed a thirteenth local flag instead of keeping the reply compact")
	}
}

func TestUseRenderingKeepsFlagDescriptionsURLFreeAndBrief(t *testing.T) {
	longDescription := strings.Repeat("界", 65)
	tests := []struct {
		name, description, want, unwanted string
	}{
		{
			name:        "removes URLs",
			description: "Enables diagnostics http://localhost/path",
			want:        "Enables diagnostics",
			unwanted:    "localhost",
		},
		{
			name:        "keeps only the first sentence",
			description: "Enables compact replies. This second sentence must not be shown",
			want:        "Enables compact replies",
			unwanted:    "second sentence",
		},
		{
			name:        "caps a long rune sequence",
			description: longDescription,
			want:        strings.Repeat("界", 64) + "…",
			unwanted:    longDescription,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderUse(i18n.LangEN, pkgFullInfo{
				atom:  "app-editors/example",
				local: []useFlag{{name: "example", desc: tt.description}},
			}, "", "", false, nil)
			if !strings.Contains(got, tt.want) {
				t.Errorf("/use omitted expected compact description %q: %q", tt.want, got)
			}
			if strings.Contains(got, tt.unwanted) {
				t.Errorf("/use kept %q in a local flag description; it can turn a compact reply into a wall of text", tt.unwanted)
			}
		})
	}
}
