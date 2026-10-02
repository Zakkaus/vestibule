package tgfmt

import (
	"fmt"
	"html"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

func ReleaseText(l i18n.Lang, release lookup.ReleaseInfo) string {
	label := release.Label
	if release.SupportEnded {
		label += i18n.Messages.LookupDistros.Release.StandardSupportEnded.For(l)
	}
	return label
}

func RenderDistros(l i18n.Lang, name string, result lookup.DistroResult) (string, string) {
	esc := html.EscapeString
	proj, alts, exact := result.Project, result.Alternatives, result.Exact
	head := i18n.Messages.LookupDistros.Pkgs.Heading.Render(l, esc(lookup.RepologyVersionsURL(proj)), esc(proj))
	if !exact {
		head += i18n.Messages.LookupDistros.Pkgs.ClosestMatch.Render(l, esc(name))
	}
	var plain, rich strings.Builder
	plain.WriteString(i18n.Messages.LookupDistros.Pkgs.PlainHeading.Render(l, head))
	rich.WriteString("<h3>" + head + "</h3><ul>")
	for _, ln := range result.Rows {
		famLink := fmt.Sprintf("<a href=\"%s\">%s</a>", esc(ln.URL), esc(ln.Label))
		rel := ""
		if label := ReleaseText(l, ln.Release); label != "" {
			rel = i18n.Messages.LookupDistros.Pkgs.ReleaseRole.Render(l, esc(label))
		}
		plain.WriteString(i18n.Messages.LookupDistros.Pkgs.PlainRow.Render(l, famLink, esc(lookup.DisplayVer(ln.Version)), rel))
		rich.WriteString(i18n.Messages.LookupDistros.Pkgs.RichRow.Render(l, famLink, esc(lookup.DisplayVer(ln.Version)), rel))
	}
	rich.WriteString("</ul>")
	if len(alts) > 0 {
		var al strings.Builder
		for i, a := range alts {
			if i > 0 {
				al.WriteString(" · ")
			}
			fmt.Fprintf(&al, "<a href=\"%s\">%s</a>", esc(lookup.RepologyVersionsURL(a)), esc(a))
		}
		plain.WriteString(i18n.Messages.LookupDistros.Pkgs.Alternatives.Render(l, al.String()))
		rich.WriteString(i18n.Messages.LookupDistros.Pkgs.RichAlternatives.Render(l, len(alts), al.String()))
	}
	return rich.String(), plain.String()
}
