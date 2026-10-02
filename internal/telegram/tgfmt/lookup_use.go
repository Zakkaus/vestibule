package tgfmt

import (
	"fmt"
	"html"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

func RenderUse(l i18n.Lang, info lookup.PkgFullInfo, srcLabel, pkgURL string, overlay bool, alsoIn []lookup.OverlayReference) string {
	esc := html.EscapeString
	messages := i18n.Messages.LookupPackages.Use
	var b strings.Builder
	label := ""
	if srcLabel != "" { // only overlay packages get a source label; official tree is implied
		label = messages.SourceLabel.Render(l, esc(srcLabel))
	}
	if pkgURL != "" {
		fmt.Fprintf(&b, "🧩 <a href=\"%s\"><b>%s</b></a>%s", esc(pkgURL), esc(info.Atom), label)
	} else {
		fmt.Fprintf(&b, "🧩 <b>%s</b>%s", esc(info.Atom), label)
	}
	if info.Description != "" {
		fmt.Fprintf(&b, "\n%s", esc(info.Description))
	}
	if info.Homepage != "" {
		fmt.Fprintf(&b, "\n🏠 %s", esc(info.Homepage))
	}
	switch {
	case info.Stable != "" && info.Latest != "" && info.Latest != info.Stable:
		fmt.Fprintf(&b, "\n%s", messages.VersionStableLatest.Render(l, esc(info.Stable), esc(info.Latest)))
	case info.Stable != "":
		fmt.Fprintf(&b, "\n%s", messages.VersionStable.Render(l, esc(info.Stable)))
	case info.Latest != "":
		fmt.Fprintf(&b, "\n%s", messages.VersionLatest.Render(l, esc(info.Latest)))
	}
	writeLocalFlags(&b, l, info.Local)
	writeGlobalFlags(&b, l, info.Global)
	WriteExpandFlags(&b, l, info.Expand)
	if len(info.Local) == 0 && len(info.Global) == 0 && len(info.Expand) == 0 {
		fmt.Fprintf(&b, "\n%s", messages.NoFlags.For(l))
	}
	if len(alsoIn) > 0 {
		fmt.Fprintf(&b, "\n<i>%s</i>", messages.AlsoInOverlay.Render(l, overlayRefs(l, alsoIn, info.Atom)))
	}
	if overlay {
		fmt.Fprintf(&b, "\n\n<i>%s</i>", messages.OverlayLegend.For(l))
	} else {
		fmt.Fprintf(&b, "\n\n<i>%s</i>", messages.OfficialLegend.For(l))
	}
	return b.String()
}

// Rich output keeps full flag descriptions in collapsible sections.
func RenderUseRich(l i18n.Lang, info lookup.PkgFullInfo, srcLabel, pkgURL string, overlay bool, alsoIn []lookup.OverlayReference) string {
	messages := i18n.Messages.LookupPackages.Use
	var b strings.Builder
	writeUseRichHeader(&b, l, info, srcLabel, pkgURL)
	writeFlagsRich(&b, l, messages.LocalFlags.For(l), info.Local, false)
	writeFlagsRich(&b, l, messages.GlobalFlags.For(l), info.Global, true)
	writeExpandFlagsRich(&b, l, info.Expand)
	if len(info.Local) == 0 && len(info.Global) == 0 && len(info.Expand) == 0 {
		fmt.Fprintf(&b, "<p>%s</p>", messages.NoFlags.For(l))
	}
	if len(alsoIn) > 0 {
		fmt.Fprintf(&b, "<p>%s</p>", messages.AlsoInOverlay.Render(l, overlayRefs(l, alsoIn, info.Atom)))
	}
	if overlay {
		fmt.Fprintf(&b, "<footer><i>%s</i></footer>", messages.OverlayLegend.For(l))
	} else {
		fmt.Fprintf(&b, "<footer><i>%s</i></footer>", messages.OfficialLegend.For(l))
	}
	return b.String()
}

func writeUseRichHeader(b *strings.Builder, l i18n.Lang, info lookup.PkgFullInfo, srcLabel, pkgURL string) {
	esc := html.EscapeString
	messages := i18n.Messages.LookupPackages.Use
	label := ""
	if srcLabel != "" {
		label = messages.SourceLabel.Render(l, esc(srcLabel))
	}
	if pkgURL != "" {
		fmt.Fprintf(b, "<h3>🧩 <a href=\"%s\">%s</a>%s</h3>", esc(pkgURL), esc(info.Atom), label)
	} else {
		fmt.Fprintf(b, "<h3>🧩 %s%s</h3>", esc(info.Atom), label)
	}
	// One paragraph with <br> avoids large inter-paragraph gaps.
	var hdr []string
	if info.Description != "" {
		hdr = append(hdr, esc(info.Description))
	}
	if info.Homepage != "" {
		hdr = append(hdr, fmt.Sprintf("🏠 <a href=\"%s\">%s</a>", esc(info.Homepage), esc(messages.Homepage.For(l))))
	}
	switch {
	case info.Stable != "" && info.Latest != "" && info.Latest != info.Stable:
		hdr = append(hdr, messages.VersionStableLatest.Render(l, esc(info.Stable), esc(info.Latest)))
	case info.Stable != "":
		hdr = append(hdr, messages.VersionStable.Render(l, esc(info.Stable)))
	case info.Latest != "":
		hdr = append(hdr, messages.VersionLatest.Render(l, esc(info.Latest)))
	}
	if len(hdr) > 0 {
		fmt.Fprintf(b, "<p>%s</p>", strings.Join(hdr, "<br>"))
	}
}

// Rich messages require block structure; newlines are whitespace.
func writeFlagsRich(b *strings.Builder, l i18n.Lang, title string, flags []lookup.UseFlag, collapse bool) {
	if len(flags) == 0 {
		return
	}
	count := i18n.Messages.LookupPackages.Use.Count.Render(l, len(flags))
	if collapse {
		fmt.Fprintf(b, "<details><summary><b>%s</b>%s</summary><ul>", title, count)
	} else {
		fmt.Fprintf(b, "<p><b>%s</b>%s</p><ul>", title, count)
	}
	for _, f := range flags {
		if f.Desc != "" {
			fmt.Fprintf(b, "<li>%s — %s</li>", useLink(f), html.EscapeString(f.Desc))
		} else {
			fmt.Fprintf(b, "<li>%s</li>", useLink(f))
		}
	}
	b.WriteString("</ul>")
	if collapse {
		b.WriteString("</details>")
	}
}

// Each large USE_EXPAND group gets its own collapsible section.
func writeExpandFlagsRich(b *strings.Builder, l i18n.Lang, groups []lookup.UseExpandGroup) {
	for _, g := range groups {
		if len(g.Flags) == 0 {
			continue
		}
		fmt.Fprintf(
			b,
			"<details><summary><b>%s</b>%s</summary><ul>",
			html.EscapeString(strings.ToUpper(g.Name)),
			i18n.Messages.LookupPackages.Use.Count.Render(l, len(g.Flags)),
		)
		for _, f := range g.Flags {
			if f.Desc != "" {
				fmt.Fprintf(b, "<li>%s%s — %s</li>", flagMark(f), html.EscapeString(f.Name), html.EscapeString(f.Desc))
			} else {
				fmt.Fprintf(b, "<li>%s%s</li>", flagMark(f), html.EscapeString(f.Name))
			}
		}
		b.WriteString("</ul></details>")
	}
}
