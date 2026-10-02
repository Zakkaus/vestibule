package lookup_test

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestWriteExpandFlags(t *testing.T) {
	many := make([]lookup.TestUseFlag, 0, 20)
	for i := range 20 {
		many = append(many, lookup.TestUseFlag{Name: "lang" + string(rune('a'+i))})
	}
	groups := []lookup.TestUseExpandGroup{
		{Name: "llvm_slot", Flags: []lookup.TestUseFlag{{Name: "20"}, {Name: "21", Def: true}, {Name: "22"}}},
		{Name: "l10n", Flags: many},
	}
	for _, l := range i18n.Languages() {
		var b strings.Builder
		tgfmt.WriteExpandFlags(&b, l, groups)
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
		if n := strings.Count(out, "lang"); n != lookup.TestExpandCap {
			t.Errorf("l10n should render exactly expandCap=%d values, got %d", lookup.TestExpandCap, n)
		}
	}
}

func TestRenderUseIncludesExpand(t *testing.T) {
	info := lookup.TestPkgFullInfo{
		Atom:   "www-client/firefox",
		Expand: []lookup.TestUseExpandGroup{{Name: "l10n", Flags: []lookup.TestUseFlag{{Name: "zh-CN"}, {Name: "en", Def: true}}}},
	}
	out := tgfmt.RenderUse(i18n.LangZH, info, "", "", false, nil)
	if !strings.Contains(out, "L10N") {
		t.Errorf("renderUse should include the L10N use_expand group: %q", out)
	}
	if strings.Contains(out, i18n.Messages.LookupPackages.Use.NoFlags.For(i18n.LangZH)) {
		t.Error("a package with use_expand must not be reported as having no USE flags")
	}
}

func TestRenderUseRichIncludesExpand(t *testing.T) {
	info := lookup.TestPkgFullInfo{
		Atom:   "www-client/firefox",
		Expand: []lookup.TestUseExpandGroup{{Name: "llvm_slot", Flags: []lookup.TestUseFlag{{Name: "20"}, {Name: "21", Desc: "Use LLVM 21.", Def: true}}}},
	}
	out := tgfmt.RenderUseRich(i18n.LangZH, info, "", "https://packages.gentoo.org/packages/www-client/firefox", false, nil)
	if !strings.Contains(out, "<details>") || !strings.Contains(out, "LLVM_SLOT") {
		t.Errorf("renderUseRich should put USE_EXPAND in a <details> block, got %q", out)
	}
	if !strings.Contains(out, "+21") || !strings.Contains(out, "Use LLVM 21.") {
		t.Errorf("rich USE_EXPAND should show the default marker + description, got %q", out)
	}
}

func TestRenderUseLookupMiss(t *testing.T) {
	for _, l := range i18n.Languages() {
		for _, tc := range []struct {
			Name         string
			Availability lookup.TestPkgLookupAvailability
			Want         func(lookup.TestPkgLookupAvailability) string
			NotWant      func(lookup.TestPkgLookupAvailability) string
		}{
			{
				Name:         "answered miss",
				Availability: lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"guru": true}},
				Want: func(_ lookup.TestPkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.NotFound.Render(l, "vim")
				},
				NotWant: func(availability lookup.TestPkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.Unavailable.Render(l, "vim", tgfmt.UnavailableSources(l, availability))
				},
			},
			{
				Name:         "source unavailable",
				Availability: lookup.TestPkgLookupAvailability{Overlays: map[string]bool{"guru": true}},
				Want: func(availability lookup.TestPkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.Unavailable.Render(l, "vim", tgfmt.UnavailableSources(l, availability))
				},
				NotWant: func(_ lookup.TestPkgLookupAvailability) string {
					return i18n.Messages.LookupPackages.Use.NotFound.Render(l, "vim")
				},
			},
		} {
			t.Run(l.String()+"/"+tc.Name, func(t *testing.T) {
				got := tgfmt.RenderUseLookupMiss(l, "vim", tc.Availability)
				want := tc.Want(tc.Availability)
				if got != want {
					t.Errorf("renderUseLookupMiss() = %q, want %q", got, want)
				}
				notWant := tc.NotWant(tc.Availability)
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
			Name         string
			Availability lookup.TestPkgLookupAvailability
			WantNote     bool
		}{
			{
				Name:         "all answered",
				Availability: lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"guru": true}},
			},
			{
				Name:         "overlay failed",
				Availability: lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"guru": false}},
				WantNote:     true,
			},
		} {
			t.Run(l.String()+"/"+tc.Name, func(t *testing.T) {
				plain, rich := tgfmt.AppendUseAvailabilityNote(l, "plain result", "<p>rich result</p>", tc.Availability)
				note := i18n.Messages.LookupPackages.Source.PartialResults.Render(l, tgfmt.UnavailableSources(l, tc.Availability))
				for label, got := range map[string]string{"plain": plain, "rich": rich} {
					hasNote := strings.Contains(got, note)
					if hasNote != tc.WantNote {
						t.Errorf("%s output %q contains partial note=%v, want %v", label, got, hasNote, tc.WantNote)
					}
				}
			})
		}
	}
}

func TestUseRenderingLimitsLocalFlagsToTwelve(t *testing.T) {
	flags := make([]lookup.TestUseFlag, 13)
	for i := range flags {
		flags[i] = lookup.TestUseFlag{Name: "flag-" + string(rune('a'+i))}
	}

	got := tgfmt.RenderUse(i18n.LangEN, lookup.TestPkgFullInfo{Atom: "app-editors/example", Local: flags}, "", "", false, nil)
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
		Name, Description, Want, Unwanted string
	}{
		{
			Name:        "removes URLs",
			Description: "Enables diagnostics http://localhost/path",
			Want:        "Enables diagnostics",
			Unwanted:    "localhost",
		},
		{
			Name:        "keeps only the first sentence",
			Description: "Enables compact replies. This second sentence must not be shown",
			Want:        "Enables compact replies",
			Unwanted:    "second sentence",
		},
		{
			Name:        "caps a long rune sequence",
			Description: longDescription,
			Want:        strings.Repeat("界", 64) + "…",
			Unwanted:    longDescription,
		},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			got := tgfmt.RenderUse(i18n.LangEN, lookup.TestPkgFullInfo{
				Atom:  "app-editors/example",
				Local: []lookup.TestUseFlag{{Name: "example", Desc: tt.Description}},
			}, "", "", false, nil)
			if !strings.Contains(got, tt.Want) {
				t.Errorf("/use omitted expected compact description %q: %q", tt.Want, got)
			}
			if strings.Contains(got, tt.Unwanted) {
				t.Errorf("/use kept %q in a local flag description; it can turn a compact reply into a wall of text", tt.Unwanted)
			}
		})
	}
}
