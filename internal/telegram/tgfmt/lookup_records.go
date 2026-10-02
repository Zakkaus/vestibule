package tgfmt

import (
	"html"
	"net/url"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

func RenderCVE(l i18n.Lang, id string, record lookup.CVERecord) string {
	cve := &i18n.Messages.LookupDistros.CVE
	lines := []string{cve.Heading.Render(l, "https://nvd.nist.gov/vuln/detail/"+id, html.EscapeString(id))}
	if record.Severity != "" {
		lines = append(lines, cve.Severity.Render(l, html.EscapeString(record.Severity), html.EscapeString(record.Score)))
	}
	if record.Published != "" {
		lines = append(lines, cve.Published.Render(l, html.EscapeString(record.Published), html.EscapeString(record.Status)))
	}
	if record.Description != "" {
		lines = append(lines, "", html.EscapeString(record.Description))
	}
	return strings.Join(lines, "\n")
}

func RenderMan(l i18n.Lang, page lookup.ManPage) string {
	man := &i18n.Messages.LookupDistros.Man
	lines := []string{man.Heading.Render(l, page.Url, html.EscapeString(page.Title))}
	if page.Synopsis != "" {
		lines = append(lines, "", man.Synopsis.Render(l, html.EscapeString(page.Synopsis)))
	}
	return strings.Join(lines, "\n")
}

func RenderKernel(l i18n.Lang, releases []lookup.KernelRelease) string {
	kernel := &i18n.Messages.LookupDistros.Kernel
	lines := []string{kernel.Heading.For(l)}
	for _, r := range releases {
		note := r.Released.ISODate
		if r.IsEOL {
			note = kernel.EOL.For(l)
		}
		lines = append(lines, kernel.Row.Render(l, html.EscapeString(r.Moniker), html.EscapeString(r.Version), html.EscapeString(note)))
	}
	lines = append(lines, "", kernel.Footer.For(l))
	return strings.Join(lines, "\n")
}

func RenderRepology(l i18n.Lang, proj string, entries []lookup.RepologyEntry, others int) string {
	repology := &i18n.Messages.LookupDistros.Repology
	link := "https://repology.org/project/" + url.PathEscape(proj) + "/versions"
	lines := []string{repology.Heading.Render(l, link, html.EscapeString(proj))}
	for _, entry := range entries {
		lines = append(lines, repology.Row.Render(l, html.EscapeString(entry.Label), html.EscapeString(entry.Version)))
	}
	if others > 0 {
		lines = append(lines, "", repology.More.Render(l, others))
	}
	return strings.Join(lines, "\n")
}
