package api

import (
	_ "embed"
	"encoding/hex"
	"html/template"
	"net/http"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

//go:embed verify_pow.js
var verificationScript string
var verificationScriptHash = styleHash(verificationScript)

type verificationPage struct {
	Language, Title, Description, Mode, Salt, SiteKey                                       string
	Bits                                                                                    int
	ChannelName, ChannelURL                                                                 string
	Style                                                                                   template.CSS
	Script                                                                                  template.JS
	Failure, Notice, Computing, Ready, Progress, Start, Submit, JavaScriptRequired, Privacy string
}

var verificationPageTemplate = template.Must(template.New("verify").Parse(`<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>{{.Title}}</title>
<style>{{.Style}}</style>
</head>
<body>
<main data-page-main><article data-page-card>
<header data-page-head><h1>{{.Title}}</h1><p data-page-lede>{{.Description}}</p></header>
{{if .Failure}}<p data-page-alert role="alert">{{.Failure}}</p>{{end}}
{{if .ChannelName}}<p data-page-note>{{if .ChannelURL}}<a href="{{.ChannelURL}}">{{.ChannelName}}</a>{{else}}{{.ChannelName}}{{end}}</p>{{end}}
{{if .Notice}}<p data-page-note role="status">{{.Notice}}</p>{{else}}
<form method="post" data-setup-form id="verification-form" data-salt="{{.Salt}}" data-bits="{{.Bits}}" data-computing="{{.Computing}}" data-ready="{{.Ready}}">
{{if eq .Mode "pow"}}
<p id="proof-status" data-page-note role="status" aria-live="polite">{{.JavaScriptRequired}}</p>
<progress id="proof-progress" aria-label="{{.Computing}}" hidden></progress>
<p data-page-note>{{.Progress}}: <output id="proof-count">0</output></p>
<input type="hidden" name="nonce" id="proof-nonce">
<button type="button" data-page-action id="proof-start" hidden>{{.Start}}</button>
<button type="submit" data-page-action id="proof-submit" disabled>{{.Submit}}</button>
{{else}}
<div class="cf-turnstile" data-sitekey="{{.SiteKey}}" data-action="verify" data-size="compact"></div>
<p data-page-note>{{.Privacy}}</p>
<button type="submit" data-page-action>{{.Submit}}</button>
<noscript><p data-page-note>{{.JavaScriptRequired}}</p></noscript>
{{end}}
</form>
{{end}}
</article></main>
{{if eq .Mode "pow"}}<script>{{.Script}}</script>{{end}}
{{if eq .Mode "captcha"}}<script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>{{end}}
</body>
</html>`))

func renderVerification(writer http.ResponseWriter, status int, record verification.PendingRecord, token verification.WebTokenRecord, siteKey, failure string, channel *verification.WebResult) {
	language := i18n.FromStored(record.Lang)
	copy := i18n.Messages.Verification.Web
	if record.Mode == "" {
		language = i18n.LangEN
		renderVerificationNoticeStatus(writer, status, language, copy.Settled.For(language), failure)
		return
	}
	page := verificationPageFor(language)
	page.Mode, page.Salt, page.Bits, page.SiteKey, page.Failure = record.Mode, hex.EncodeToString(token.Salt[:]), token.PoWBits, siteKey, failure
	page.Description = copy.PoWIntro.For(language)
	if record.Mode == settings.ModeCaptcha {
		page.Description = copy.CaptchaIntro.For(language)
	}
	if channel != nil {
		page.ChannelName, page.ChannelURL = channel.ChannelName, channel.ChannelURL
		if page.ChannelName == "" {
			page.ChannelName = i18n.Messages.Verification.Channel.FallbackName.For(language)
		}
	}
	writeVerificationPage(writer, status, page)
}

func verificationPageFor(language i18n.Lang) verificationPage {
	copy := i18n.Messages.Verification.Web
	return verificationPage{Language: language.String(), Title: copy.Title.For(language),
		// #nosec G203 -- fixed embedded CSS and JavaScript; no request data enters either source.
		Style: template.CSS(pageStyle), Script: template.JS(verificationScript),
		Computing: copy.Computing.For(language), Ready: copy.Ready.For(language), Progress: copy.Progress.For(language),
		Start: copy.Start.For(language), Submit: copy.Submit.For(language), JavaScriptRequired: copy.JavaScriptRequired.For(language), Privacy: copy.Privacy.For(language)}
}

func renderVerificationNotice(writer http.ResponseWriter, language i18n.Lang, notice string) {
	renderVerificationNoticeStatus(writer, http.StatusOK, language, notice, "")
}

func renderVerificationNoticeStatus(writer http.ResponseWriter, status int, language i18n.Lang, notice, failure string) {
	page := verificationPageFor(language)
	page.Notice, page.Failure = notice, failure
	writeVerificationPage(writer, status, page)
}

func writeVerificationPage(writer http.ResponseWriter, status int, page verificationPage) {
	policy := "default-src 'none'; style-src " + pageStyleHash + "; script-src " + verificationScriptHash
	if page.Mode == settings.ModeCaptcha {
		policy += " https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; connect-src https://challenges.cloudflare.com"
	}
	policy += "; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"
	writer.Header().Set("Content-Security-Policy", policy)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)
	_ = verificationPageTemplate.Execute(writer, page)
}
