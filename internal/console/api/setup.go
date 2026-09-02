package api

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

const maxSetupBody = 4 << 10

// ErrSetupUnavailable indicates that this process no longer admits an instance claim.
var ErrSetupUnavailable = errors.New("setup is unavailable")

// SetupService activates an unclaimed instance after the caller proves possession of its link.
type SetupService interface {
	SetupAvailable(token string) bool
	Claim(context.Context, string) (SetupResult, error)
}

// SetupResult contains the non-credential next step after a successful instance claim.
type SetupResult struct {
	Claimed    bool
	BindingURL string
}
type setupPage struct {
	Language      string
	Title         string
	Description   string
	TokenLabel    string
	Submit        string
	Error         string
	Claimed       string
	Binding       string
	BindingURL    string
	BindingAction string
}

var setupPageTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Description}}</p>
{{if .Error}}<p id="setup-error" role="alert">{{.Error}}</p>{{end}}
{{if .Claimed}}
<p>{{.Claimed}}</p>
{{if .BindingURL}}<p>{{.Binding}}</p><p><a href="{{.BindingURL}}">{{.BindingAction}}</a></p>{{end}}
{{else}}
<form method="post">
<label for="bot-token">{{.TokenLabel}}</label>
<input id="bot-token" name="bot_token" type="password" autocomplete="off" required{{if .Error}} aria-invalid="true" aria-describedby="setup-error"{{end}}>
<button type="submit">{{.Submit}}</button>
</form>
{{end}}
</main>
</body>
</html>`))

func (s *Server) setupRoute(writer http.ResponseWriter, request *http.Request) {
	if strings.Count(request.URL.Path, "/") != 2 {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	token := strings.TrimPrefix(request.URL.Path, "/setup/")
	if token == "" || !s.setup.SetupAvailable(token) {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	switch request.Method {
	case http.MethodGet:
		renderSetup(writer, request, http.StatusOK, "", SetupResult{})
	case http.MethodPost:
		s.submitSetup(writer, request)
	default:
		writeError(writer, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (s *Server) submitSetup(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxSetupBody)
	if err := request.ParseForm(); err != nil {
		renderSetup(writer, request, http.StatusBadRequest, i18n.Messages.Bot.Setup.TokenRequired.For(setupLanguage(request)), SetupResult{})
		return
	}
	botToken := strings.TrimSpace(request.PostForm.Get("bot_token"))
	if botToken == "" {
		renderSetup(writer, request, http.StatusBadRequest, i18n.Messages.Bot.Setup.TokenRequired.For(setupLanguage(request)), SetupResult{})
		return
	}
	result, err := s.setup.Claim(request.Context(), botToken)
	if errors.Is(err, ErrSetupUnavailable) {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		renderSetup(writer, request, http.StatusUnprocessableEntity, i18n.Messages.Bot.Setup.ClaimFailed.For(setupLanguage(request)), SetupResult{})
		return
	}
	if s.setupClaimed != nil {
		s.setupClaimed()
	}
	renderSetup(writer, request, http.StatusOK, "", result)
}

func renderSetup(writer http.ResponseWriter, request *http.Request, statusCode int, failure string, result SetupResult) {
	language := setupLanguage(request)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'self'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.WriteHeader(statusCode)
	_ = setupPageTemplate.Execute(writer, setupPage{
		Language:      language.String(),
		Title:         i18n.Messages.Bot.Setup.Title.For(language),
		Description:   i18n.Messages.Bot.Setup.Description.For(language),
		TokenLabel:    i18n.Messages.Bot.Setup.TokenLabel.For(language),
		Submit:        i18n.Messages.Bot.Setup.Submit.For(language),
		Error:         failure,
		Claimed:       claimedSetupText(language, result),
		Binding:       i18n.Messages.Bot.Setup.Binding.For(language),
		BindingURL:    result.BindingURL,
		BindingAction: i18n.Messages.Bot.Setup.BindingAction.For(language),
	})
}

func claimedSetupText(language i18n.Lang, result SetupResult) string {
	if !result.Claimed {
		return ""
	}
	return i18n.Messages.Bot.Setup.Claimed.For(language)
}

func setupLanguage(request *http.Request) i18n.Lang {
	return i18n.FromRequester(request.Header.Get("Accept-Language"), i18n.LangZH)
}
