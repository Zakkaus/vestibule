package tgfmt

import (
	"fmt"
	"html"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

func BugLookupFailureMessage(l i18n.Lang, id, link string, state lookup.BugLookupState) string {
	if state == lookup.BugLookupNotFound {
		return i18n.Messages.LookupContent.Bug.NotFound.Render(l, id)
	}
	return i18n.Messages.LookupContent.Bug.Unavailable.Render(l, id, link)
}

func RenderNews(l i18n.Lang, arg string, items []lookup.NewsItem, available bool) string {
	q := strings.ToLower(arg)
	var b strings.Builder
	if q == "" {
		b.WriteString(i18n.Messages.LookupContent.News.LatestHeading.For(l))
	} else {
		b.WriteString(i18n.Messages.LookupContent.News.SearchHeading.Render(l, html.EscapeString(arg)))
	}
	n := 0
	for _, it := range items {
		if q != "" && !strings.Contains(strings.ToLower(it.Title), q) && !strings.Contains(strings.ToLower(it.URL), q) {
			continue
		}
		title := html.EscapeString(html.UnescapeString(it.Title))
		fmt.Fprintf(&b, "\n • <a href=\"%s\">%s — %s</a>", html.EscapeString(it.URL), it.Date, title)
		n++
		if n >= 8 {
			break
		}
	}
	if n == 0 {
		if available {
			b.WriteByte('\n')
			b.WriteString(i18n.Messages.LookupContent.News.NoMatches.For(l))
		} else {
			b.WriteByte('\n')
			b.WriteString(i18n.Messages.LookupContent.News.Unavailable.For(l))
		}
	} else if !available {
		b.WriteByte('\n')
		b.WriteString(i18n.Messages.LookupContent.News.Stale.For(l))
	}
	return b.String()
}

func WikiResultNotice(l i18n.Lang, found bool, srcOK []bool) string {
	var missing []string
	for i, ok := range srcOK {
		if !ok {
			missing = append(missing, lookup.WikiSources[i].Name+" Wiki")
		}
	}
	if len(missing) > 0 {
		sources := strings.Join(missing, i18n.Messages.LookupContent.Wiki.SourceJoin.For(l))
		return i18n.Messages.LookupContent.Wiki.SourcesUnavailable.Render(l, sources)
	}
	if !found {
		return i18n.Messages.LookupContent.Wiki.NoMatches.For(l)
	}
	return ""
}
