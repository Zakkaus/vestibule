package api

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

type errorPage struct {
	Language    string
	Style       template.CSS
	Eyebrow     string
	Title       string
	Description string
	StepsLabel  string
	Steps       []string
	Action      string
	ActionURL   string
}

var errorPageTemplate = template.Must(template.New("error").Parse(`<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="color-scheme" content="light dark">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>{{.Style}}</style>
</head>
<body>
<main data-page-main>
<article data-page-card>
<header data-page-head>
<p data-page-eyebrow>{{.Eyebrow}}</p>
<h1>{{.Title}}</h1>
<p data-page-lede>{{.Description}}</p>
</header>
{{if .Steps}}<section data-error-steps aria-label="{{.StepsLabel}}">
<p data-page-note>{{.StepsLabel}}</p>
<ul>{{range .Steps}}<li>{{.}}</li>{{end}}</ul>
</section>{{end}}
{{if .ActionURL}}<a data-page-action data-page-onward href="{{.ActionURL}}">{{.Action}}</a>{{end}}
</article>
</main>
</body>
</html>`))

// writeNotFound answers in the form the caller asked for. The JSON body an API
// client needs reads, to a person who mistyped an address or opened a one-time
// link twice, as a blank page with a word on it -- and that address is often the
// link they were told to open, so "not found" is the moment they most need to be
// told what to do instead. Anything that did not ask for HTML still gets JSON:
// probes, fetches and scripts are not helped by a page.
func (s *Server) writeNotFound(writer http.ResponseWriter, request *http.Request) {
	if !prefersHTML(request) {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	s.writePageError(writer, request, http.StatusNotFound)
}

// prefersHTML is true for a browser navigation and false for everything else.
// A navigation names text/html explicitly; fetch, curl and probes send */* or
// application/json, and a page would be noise in their logs.
func prefersHTML(request *http.Request) bool {
	return strings.Contains(request.Header.Get("Accept"), "text/html")
}

// writePageError renders the page itself.
func (s *Server) writePageError(writer http.ResponseWriter, request *http.Request, statusCode int) {
	language := setupLanguage(request)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; form-action 'none'; style-src "+pageStyleHash)
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.WriteHeader(statusCode)
	_ = errorPageTemplate.Execute(writer, s.errorPageFor(language, statusCode))
}

func (s *Server) errorPageFor(language i18n.Lang, statusCode int) errorPage {
	catalog := i18n.Messages.Bot.ErrorPage
	page := errorPage{
		Language:   language.String(),
		Style:      template.CSS(pageStyle),
		Eyebrow:    catalog.Eyebrow.For(language),
		StepsLabel: catalog.StepsLabel.For(language),
		Action:     catalog.Action.For(language),
		ActionURL:  "/",
	}
	if statusCode == http.StatusNotFound {
		page.Title = catalog.NotFoundTitle.For(language)
		page.Description = catalog.NotFoundDescription.For(language)
		page.Steps = s.errorSteps(language)
		return page
	}
	page.Title = catalog.RefusedTitle.For(language)
	page.Description = catalog.RefusedDescription.For(language)
	return page
}

// errorSteps names the bot only when there is one. On an instance nobody has
// claimed there is no bot to open, and the way in is the link the install script
// printed -- the same distinction the entry screen makes.
func (s *Server) errorSteps(language i18n.Lang) []string {
	catalog := i18n.Messages.Bot.ErrorPage
	handle := strings.TrimSpace(s.botUsername)
	if handle == "" {
		return []string{catalog.StepUnclaimed.For(language)}
	}
	return []string{
		catalog.StepManager.Render(language, "@"+strings.TrimPrefix(handle, "@")),
		catalog.StepOperator.For(language),
	}
}
