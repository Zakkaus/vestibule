package tgfmt

import (
	"fmt"
	"html"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/mymmrac/telego"
)

func RenderBug(l i18n.Lang, id, link string, info lookup.BugInfo) string {
	var b strings.Builder
	b.WriteString(i18n.Messages.LookupContent.Bug.Heading.Render(l, link, id, html.EscapeString(info.Summary)))
	status := i18n.TranslateBugValue(l, info.Status)
	if info.Resolution != "" {
		status += i18n.Messages.LookupContent.Bug.Details.ResolutionSeparator.For(l) + i18n.TranslateBugValue(l, info.Resolution)
	}
	b.WriteString(i18n.Messages.LookupContent.Bug.Details.Status.Render(l, html.EscapeString(status)))
	if info.Severity != "" {
		b.WriteString(i18n.Messages.LookupContent.Bug.Details.Severity.Render(l, html.EscapeString(i18n.TranslateBugValue(l, info.Severity))))
	}
	if info.Product != "" {
		comp := info.Product
		if info.Component != "" {
			comp += " › " + info.Component
		}
		b.WriteByte('\n')
		b.WriteString(i18n.Messages.LookupContent.Bug.Details.ProductComponent.Render(l, html.EscapeString(comp)))
	}
	return b.String()
}

func RenderWiki(l i18n.Lang, q string, results []lookup.WikiResult) string {
	var b strings.Builder
	b.WriteString(i18n.Messages.LookupContent.Wiki.Heading.Render(l, html.EscapeString(q)))
	found := false
	srcOK := make([]bool, len(results))
	for i, result := range results {
		srcOK[i] = result.Available
		if len(result.Pages) == 0 {
			continue
		}
		found = true
		fmt.Fprintf(&b, "\n\n<b>%s Wiki</b>", html.EscapeString(result.Name))
		for _, page := range result.Pages {
			fmt.Fprintf(&b, "\n • <a href=\"%s\">%s</a>", html.EscapeString(page.URL), html.EscapeString(page.Title))
		}
	}
	b.WriteString(WikiResultNotice(l, found, srcOK))
	return b.String()
}

func RenderBBS(l i18n.Lang, q string, hits []lookup.ForumTopic, available bool) (string, [][]telego.InlineKeyboardButton) {
	var b strings.Builder
	b.WriteString(i18n.Messages.LookupContent.BBS.Heading.Render(l, html.EscapeString(q)))
	switch {
	case len(hits) > 0:
		b.WriteString(i18n.Messages.LookupContent.BBS.ArchCNHeading.For(l))
		for _, h := range hits {
			fmt.Fprintf(&b, "\n • <a href=\"%s\">%s</a>", html.EscapeString(h.Url), html.EscapeString(h.Title))
		}
	case !available:
		b.WriteString(i18n.Messages.LookupContent.BBS.ArchCNUnavailable.For(l))
	default:
		b.WriteString(i18n.Messages.LookupContent.BBS.ArchCNNoMatches.For(l))
	}
	b.WriteString(i18n.Messages.LookupContent.BBS.OtherForums.For(l))
	qBtn := q
	if r := []rune(qBtn); len(r) > 200 {
		qBtn = string(r[:200])
	}
	var rows [][]telego.InlineKeyboardButton
	for i := 0; i < len(lookup.ForumLinks); i += 2 {
		var row []telego.InlineKeyboardButton
		for j := i; j < i+2 && j < len(lookup.ForumLinks); j++ {
			row = append(row, telego.InlineKeyboardButton{Text: lookup.ForumLinks[j].Label.For(l), URL: lookup.DdgSiteSearch(lookup.ForumLinks[j].Site, qBtn)})
		}
		rows = append(rows, row)
	}
	return b.String(), rows
}
