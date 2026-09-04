package api

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

const maxSetupBody = 4 << 10

// ErrSetupUnavailable indicates that this process no longer admits an instance claim.
var ErrSetupUnavailable = errors.New("setup is unavailable")

// ErrSetupTokenRejected reports that Telegram answered and refused the token.
// It is separate from ErrSetupTelegramUnreachable because the two send the
// reader to different places: one token has to be pasted again, the other has
// to be fixed on the machine, and a single message for both sends half of them
// to the wrong one.
var ErrSetupTokenRejected = errors.New("telegram rejected the bot token")

// ErrSetupTelegramUnreachable reports that the token was never judged because
// Telegram did not answer.
var ErrSetupTelegramUnreachable = errors.New("telegram could not be reached")

// SetupService activates an unclaimed instance after the caller proves possession of its link.
type SetupService interface {
	SetupAvailable(token string) bool
	Claim(context.Context, string) (SetupResult, error)
}

// SetupResult contains the non-credential next step after a successful instance claim.
type SetupResult struct {
	Claimed     bool
	BotUsername string
	BindingURL  string
}

// setupStyle is kept in its own file rather than in the template literal below
// so that the design checks which scan the application stylesheets can scan
// this one too. A stylesheet living inside a Go string is invisible to every
// check the repository owns.
//
//go:embed setup.css
var setupStyle string

// setupStyleHash lets the page keep a Content-Security-Policy that admits this
// one stylesheet and nothing else. 'unsafe-inline' would admit any style an
// injection managed to reach the page.
var setupStyleHash = styleHash(setupStyle)

func styleHash(style string) string {
	sum := sha256.Sum256([]byte(style))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

type setupStep struct {
	Index int
	State string
	Name  string
	Note  string
}

type setupPage struct {
	Language      string
	Style         template.CSS
	Eyebrow       string
	Title         string
	Description   string
	StepsLabel    string
	Steps         []setupStep
	TokenLabel    string
	TokenHint     string
	Submit        string
	Error         string
	Claimed       string
	BotNameLabel  string
	BotName       string
	Binding       string
	BindingURL    string
	BindingAction string
	AfterBinding  string
}

var setupPageTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>{{.Style}}</style>
</head>
<body>
<main data-setup-main>
<article data-setup-card>
<header data-setup-head>
<p data-setup-eyebrow>{{.Eyebrow}}</p>
<h1>{{.Title}}</h1>
<p data-setup-lede>{{.Description}}</p>
</header>
<ol data-setup-steps aria-label="{{.StepsLabel}}">
{{range .Steps}}<li data-setup-step data-state="{{.State}}">
<span data-setup-step-mark aria-hidden="true">{{if eq .State "done"}}&#x2713;{{else}}{{.Index}}{{end}}</span>
<span data-setup-step-body><span data-setup-step-name>{{.Name}}</span><span data-setup-step-note>{{.Note}}</span></span>
</li>
{{end}}</ol>
{{if .Error}}<p data-setup-alert id="setup-error" role="alert">{{.Error}}</p>{{end}}
{{if .Claimed}}
<section data-setup-panel>
<p data-setup-ok>{{.Claimed}}</p>
{{if .BotName}}<p data-setup-bot>{{.BotNameLabel}} <b>{{.BotName}}</b></p>{{end}}
{{if .BindingURL}}<p data-setup-note>{{.Binding}}</p><a data-setup-action href="{{.BindingURL}}">{{.BindingAction}}</a><p data-setup-note>{{.AfterBinding}}</p>{{end}}
</section>
{{else}}
<form method="post" data-setup-form>
<label for="bot-token">{{.TokenLabel}}</label>
<input id="bot-token" name="bot_token" type="password" autocomplete="off" spellcheck="false" required{{if .Error}} aria-invalid="true" aria-describedby="setup-error"{{end}}>
<p data-setup-hint>{{.TokenHint}}</p>
<button type="submit" data-setup-action>{{.Submit}}</button>
</form>
{{end}}
</article>
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
		renderSetup(writer, request, http.StatusUnprocessableEntity, claimFailureText(setupLanguage(request), err), SetupResult{})
		return
	}
	if s.setupClaimed != nil {
		s.setupClaimed()
	}
	renderSetup(writer, request, http.StatusOK, "", result)
}

// claimFailureText names the cause the reader can act on. The default is the
// instance's own fault rather than the token's, because everything that is not
// one of the two Telegram answers happened after the token was accepted, and
// telling those readers to check their token sends them to re-paste a string
// that was never the problem.
func claimFailureText(language i18n.Lang, err error) string {
	switch {
	case errors.Is(err, ErrSetupTokenRejected):
		return i18n.Messages.Bot.Setup.TokenRejected.For(language)
	case errors.Is(err, ErrSetupTelegramUnreachable):
		return i18n.Messages.Bot.Setup.TelegramUnreachable.For(language)
	default:
		return i18n.Messages.Bot.Setup.InstanceFault.For(language)
	}
}

func renderSetup(writer http.ResponseWriter, request *http.Request, statusCode int, failure string, result SetupResult) {
	language := setupLanguage(request)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; form-action 'self'; style-src "+setupStyleHash)
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.WriteHeader(statusCode)
	_ = setupPageTemplate.Execute(writer, setupPageFor(language, failure, result))
}

func setupPageFor(language i18n.Lang, failure string, result SetupResult) setupPage {
	setup := i18n.Messages.Bot.Setup
	return setupPage{
		Language: language.String(),
		// #nosec G203 -- setupStyle is the embedded contents of setup.css, fixed at
		// build time and reachable by no request. The conversion exists because a
		// stylesheet is not HTML text, not because anything untrusted passes here.
		Style:         template.CSS(setupStyle),
		Eyebrow:       setup.Eyebrow.For(language),
		Title:         setup.Title.For(language),
		Description:   setup.Description.For(language),
		StepsLabel:    setup.StepsLabel.For(language),
		Steps:         setupSteps(language, result.Claimed),
		TokenLabel:    setup.TokenLabel.For(language),
		TokenHint:     setup.TokenHint.For(language),
		Submit:        setup.Submit.For(language),
		Error:         failure,
		Claimed:       claimedSetupText(language, result),
		BotNameLabel:  setup.BotNameLabel.For(language),
		BotName:       botHandle(result.BotUsername),
		Binding:       setup.Binding.For(language),
		BindingURL:    result.BindingURL,
		BindingAction: setup.BindingAction.For(language),
		AfterBinding:  setup.AfterBinding.For(language),
	}
}

// setupSteps renders the four deployment steps with the one in progress marked.
// The list is shown before the token field rather than after it so that nobody
// is asked for a credential by a screen that has not yet said what it is doing
// with it.
func setupSteps(language i18n.Lang, claimed bool) []setupStep {
	setup := i18n.Messages.Bot.Setup
	current := 2
	if claimed {
		current = 3
	}
	names := []i18n.Text{setup.StepConsole, setup.StepToken, setup.StepBinding, setup.StepGroup}
	notes := []i18n.Text{setup.StepConsoleNote, setup.StepTokenNote, setup.StepBindingNote, setup.StepGroupNote}
	steps := make([]setupStep, 0, len(names))
	for index := range names {
		steps = append(steps, setupStep{
			Index: index + 1,
			State: stepState(index+1, current),
			Name:  names[index].For(language),
			Note:  notes[index].For(language),
		})
	}
	return steps
}

func stepState(step, current int) string {
	switch {
	case step < current:
		return "done"
	case step == current:
		return "current"
	default:
		return "pending"
	}
}

func botHandle(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	return "@" + strings.TrimPrefix(username, "@")
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
