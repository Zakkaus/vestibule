package tgfmt

import (
	"fmt"
	"html"
	"sort"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

func UnavailableSources(l i18n.Lang, a lookup.PkgLookupAvailability) string {
	messages := i18n.Messages.LookupPackages.Source
	sources := make([]string, 0, len(a.Overlays)+1)
	if !a.Official {
		sources = append(sources, messages.GentooOfficialTree.For(l))
	}
	unavailableOverlays := make([]string, 0, len(a.Overlays))
	for name, ok := range a.Overlays {
		if !ok {
			unavailableOverlays = append(unavailableOverlays, name)
		}
	}
	sort.Strings(unavailableOverlays)
	for _, name := range unavailableOverlays {
		sources = append(sources, messages.Overlay.Render(l, html.EscapeString(name)))
	}
	return strings.Join(sources, messages.ListSeparator.For(l))
}

func RenderPkg(l i18n.Lang, q string, result lookup.PackageResult) string {
	mainRes, vm, ovRes, availability := result.Official, result.Versions, result.Overlays, result.Availability
	esc := html.EscapeString
	messages := i18n.Messages.LookupPackages.Pkg
	var b strings.Builder
	fmt.Fprintf(&b, "🔎 %s", messages.ResultsHeading.Render(l, "<b>"+esc(q)+"</b>"))
	found := false
	if len(mainRes) > 0 {
		found = true
		fmt.Fprintf(&b, "\n\n📦 <b>%s</b>", messages.OfficialHeading.For(l))
		for _, a := range mainRes {
			ver := ""
			if vm[a][0] != "" {
				ver = " — " + esc(vm[a][0]) // amd64-stable: no symbol
			} else if vm[a][1] != "" {
				ver = " — ~" + esc(vm[a][1]) // no amd64-stable version; latest need not have ~amd64
			}
			fmt.Fprintf(&b, "\n • <a href=\"%s\">%s</a>%s",
				esc("https://packages.gentoo.org/packages/"+a), esc(a), ver)
		}
	}
	for _, o := range ovRes {
		hits := o.Hits
		if len(hits) == 0 {
			continue
		}
		found = true
		fmt.Fprintf(&b, "\n\n🧩 <b>%s</b>", esc(o.Name))
		for _, a := range hits {
			ver := ""
			if vv := a.Version; vv != "" {
				ver = " — ~" + esc(vv) // the marker identifies an overlay version, not keyword status
			}
			fmt.Fprintf(&b, "\n • <a href=\"%s\">%s</a>%s",
				esc(a.URL), esc(a.Atom), ver)
		}
	}
	if !found {
		if availability.AnyUnavailable() {
			fmt.Fprintf(&b, "\n\n%s", messages.Unavailable.Render(l, UnavailableSources(l, availability)))
		} else {
			fmt.Fprintf(&b, "\n\n%s", messages.NotFound.For(l))
		}
	} else {
		fmt.Fprintf(&b, "\n\n<i>%s</i>", messages.KeywordLegend.For(l))
		if availability.AnyUnavailable() {
			fmt.Fprintf(&b, "\n<i>%s</i>", i18n.Messages.LookupPackages.Source.PartialResults.Render(l, UnavailableSources(l, availability)))
		}
	}
	return b.String()
}

// Rich messages require block tags because newlines are ignored.
func RenderPkgRich(l i18n.Lang, q string, result lookup.PackageResult) string {
	mainRes, vm, ovRes, availability := result.Official, result.Versions, result.Overlays, result.Availability
	esc := html.EscapeString
	messages := i18n.Messages.LookupPackages.Pkg
	var b strings.Builder
	fmt.Fprintf(&b, "<h3>🔎 %s</h3>", messages.ResultsHeading.Render(l, esc(q)))
	found := false
	if len(mainRes) > 0 {
		found = true
		fmt.Fprintf(&b, "<h4>📦 %s</h4><ul>", messages.OfficialHeading.For(l))
		for _, a := range mainRes {
			ver := ""
			if vm[a][0] != "" {
				ver = " — " + esc(vm[a][0])
			} else if vm[a][1] != "" {
				ver = " — ~" + esc(vm[a][1])
			}
			fmt.Fprintf(&b, "<li><a href=\"%s\">%s</a>%s</li>",
				esc("https://packages.gentoo.org/packages/"+a), esc(a), ver)
		}
		b.WriteString("</ul>")
	}
	for _, o := range ovRes {
		hits := o.Hits
		if len(hits) == 0 {
			continue
		}
		found = true
		fmt.Fprintf(
			&b,
			"<details><summary>🧩 <b>%s</b>%s</summary><ul>",
			esc(o.Name),
			messages.OverlayCount.Render(l, len(hits)),
		)
		for _, a := range hits {
			ver := ""
			if vv := a.Version; vv != "" {
				ver = " — ~" + esc(vv)
			}
			fmt.Fprintf(&b, "<li><a href=\"%s\">%s</a>%s</li>",
				esc(a.URL), esc(a.Atom), ver)
		}
		b.WriteString("</ul></details>")
	}
	if !found {
		if availability.AnyUnavailable() {
			fmt.Fprintf(&b, "<p>%s</p>", messages.Unavailable.Render(l, UnavailableSources(l, availability)))
		} else {
			fmt.Fprintf(&b, "<p>%s</p>", messages.NotFound.For(l))
		}
	} else {
		fmt.Fprintf(&b, "<footer><i>%s</i></footer>", messages.KeywordLegend.For(l))
		if availability.AnyUnavailable() {
			fmt.Fprintf(&b, "<footer><i>%s</i></footer>", i18n.Messages.LookupPackages.Source.PartialResults.Render(l, UnavailableSources(l, availability)))
		}
	}
	return b.String()
}

// Keep compact output to one short, URL-free sentence.
func shortDesc(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "http"); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	if i := strings.IndexAny(s, ".。"); i > 8 {
		s = s[:i]
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) > 64 {
		return strings.TrimSpace(string(r[:64])) + "…"
	}
	return string(r)
}

func flagMark(f lookup.UseFlag) string {
	if f.Def {
		return "+"
	}
	return ""
}

func useLink(f lookup.UseFlag) string {
	u := "https://packages.gentoo.org/useflags/" + f.Name
	return flagMark(f) + fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(u), html.EscapeString(f.Name))
}

func writeLocalFlags(b *strings.Builder, l i18n.Lang, flags []lookup.UseFlag) {
	if len(flags) == 0 {
		return
	}
	messages := i18n.Messages.LookupPackages.Use
	fmt.Fprintf(b, "\n<b>%s</b>%s", messages.LocalFlags.For(l), messages.Count.Render(l, len(flags)))
	for i, f := range flags {
		if i >= 12 && len(flags) > 12 {
			fmt.Fprintf(b, "\n%s", messages.TruncatedCount.Render(l, len(flags)))
			break
		}
		if d := shortDesc(f.Desc); d != "" {
			fmt.Fprintf(b, "\n • %s — %s", useLink(f), html.EscapeString(d))
		} else {
			fmt.Fprintf(b, "\n • %s", useLink(f))
		}
	}
}

func writeGlobalFlags(b *strings.Builder, l i18n.Lang, flags []lookup.UseFlag) {
	if len(flags) == 0 {
		return
	}
	links := make([]string, 0, len(flags))
	for _, f := range flags {
		links = append(links, useLink(f))
	}
	messages := i18n.Messages.LookupPackages.Use
	fmt.Fprintf(
		b,
		"\n<b>%s</b>%s%s%s",
		messages.GlobalFlags.For(l),
		messages.Count.Render(l, len(flags)),
		messages.ValueSeparator.For(l),
		strings.Join(links, " "),
	)
}

// Compact output truncates values; the rich view retains full descriptions.
func WriteExpandFlags(b *strings.Builder, l i18n.Lang, groups []lookup.UseExpandGroup) {
	messages := i18n.Messages.LookupPackages.Use
	for _, g := range groups {
		if len(g.Flags) == 0 {
			continue
		}
		names := make([]string, 0, lookup.ExpandCap)
		for i, f := range g.Flags {
			if i >= lookup.ExpandCap {
				break
			}
			names = append(names, flagMark(f)+html.EscapeString(f.Name))
		}
		more := ""
		if len(g.Flags) > lookup.ExpandCap {
			more = messages.TruncatedCount.Render(l, len(g.Flags))
		}
		fmt.Fprintf(
			b,
			"\n<b>%s</b>%s%s%s%s",
			html.EscapeString(strings.ToUpper(g.Name)),
			messages.Count.Render(l, len(g.Flags)),
			messages.ValueSeparator.For(l),
			strings.Join(names, " "),
			more,
		)
	}
}

// overlayRefs renders linked overlay names for the cross-source footer.
func overlayRefs(l i18n.Lang, alsoIn []lookup.OverlayReference, _ string) string {
	refs := make([]string, 0, len(alsoIn))
	for _, overlay := range alsoIn {
		ref := html.EscapeString(overlay.Name)
		if overlay.URL != "" {
			ref = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(overlay.URL), html.EscapeString(overlay.Name))
		}
		refs = append(refs, ref)
	}
	return strings.Join(refs, i18n.Messages.LookupPackages.Source.ListSeparator.For(l))
}

func RenderUseLookupMiss(l i18n.Lang, q string, availability lookup.PkgLookupAvailability) string {
	messages := i18n.Messages.LookupPackages.Use
	if availability.AnyUnavailable() {
		return messages.Unavailable.Render(l, q, UnavailableSources(l, availability))
	}
	return messages.NotFound.Render(l, q)
}

func AppendUseAvailabilityNote(l i18n.Lang, plain, rich string, availability lookup.PkgLookupAvailability) (string, string) {
	if !availability.AnyUnavailable() {
		return plain, rich
	}
	note := i18n.Messages.LookupPackages.Source.PartialResults.Render(l, UnavailableSources(l, availability))
	plain += "\n\n<i>" + note + "</i>"
	if rich != "" {
		rich += "<footer><i>" + note + "</i></footer>"
	}
	return plain, rich
}

// OnUse handles package metadata and USE flag lookups.
// renderUseMultipleMatches lists candidate atoms with the canonical query command.
func RenderUseMultipleMatches(l i18n.Lang, atoms []string, availability lookup.PkgLookupAvailability) string {
	sort.Strings(atoms)
	var b strings.Builder
	b.WriteString(i18n.Messages.LookupPackages.Use.MultipleMatches.For(l))
	for _, a := range atoms {
		fmt.Fprintf(&b, "\n • /guse %s", a)
	}
	if availability.AnyUnavailable() {
		fmt.Fprintf(&b, "\n%s", i18n.Messages.LookupPackages.Use.PartialMatches.Render(l, UnavailableSources(l, availability)))
	}
	return b.String()
}
