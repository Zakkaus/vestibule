package api

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/verification"
)

// WebVerificationService owns non-consuming resolution, proof settlement and C5 question delivery.
type WebVerificationService interface {
	ResolveWebToken(context.Context, string) (verification.PendingRecord, verification.WebTokenRecord, bool, error)
	AnswerWeb(context.Context, string, verification.WebProof) (verification.WebResult, error)
	DeliverWebFallback(context.Context, string)
	WebModeUnavailable(int64, string) string
}

func consoleOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return "https://" + strings.ToLower(parsed.Host)
}

func (s *Server) verifyRoute(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		s.showVerification(writer, request, "", nil)
	case http.MethodPost:
		s.submitVerification(writer, request)
	default:
		renderVerification(writer, http.StatusMethodNotAllowed, verification.PendingRecord{}, verification.WebTokenRecord{}, "", "", nil)
	}
}

func (s *Server) resolveVerification(request *http.Request) (verification.PendingRecord, verification.WebTokenRecord, bool, error) {
	if s.webVerification == nil || strings.Count(request.URL.Path, "/") != 2 {
		return verification.PendingRecord{}, verification.WebTokenRecord{}, false, nil
	}
	raw := strings.TrimPrefix(request.URL.Path, "/verify/")
	if len(raw) != 32 {
		return verification.PendingRecord{}, verification.WebTokenRecord{}, false, nil
	}
	return s.webVerification.ResolveWebToken(request.Context(), raw)
}

func (s *Server) showVerification(writer http.ResponseWriter, request *http.Request, failure string, channel *verification.WebResult) {
	record, token, valid, err := s.resolveVerification(request)
	if err != nil {
		renderVerification(writer, http.StatusServiceUnavailable, verification.PendingRecord{}, token, "", i18n.Messages.Verification.Web.Unavailable.For(i18n.LangEN), nil)
		return
	}
	if !valid {
		record, token = verification.PendingRecord{}, verification.WebTokenRecord{}
	}
	renderVerification(writer, http.StatusOK, record, token, s.turnstileSiteKey, failure, channel)
}

func (s *Server) admitVerification(writer http.ResponseWriter, request *http.Request) bool {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		writeError(writer, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return false
	}
	origin, site := request.Header.Get("Origin"), request.Header.Get("Sec-Fetch-Site")
	expected := consoleOrigin(s.consoleURL)
	// no-referrer makes an ordinary browser form's Origin opaque; Fetch Metadata still proves its site.
	if expected == "" || site == "cross-site" || (site != "same-origin" && origin != expected) || (origin != "" && origin != "null" && origin != expected) {
		writeError(writer, http.StatusForbidden, "origin_rejected")
		return false
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
	if err := request.ParseForm(); err != nil {
		var oversized *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &oversized) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(writer, status, "invalid_form")
		return false
	}
	return true
}

func (s *Server) submitVerification(writer http.ResponseWriter, request *http.Request) {
	if !s.admitVerification(writer, request) {
		return
	}
	record, token, valid, err := s.resolveVerification(request)
	if err != nil || !valid {
		s.showVerification(writer, request, "", nil)
		return
	}
	proof := verification.WebProof{Token: strings.TrimPrefix(request.URL.Path, "/verify/"), Nonce: request.PostForm.Get("nonce"), CaptchaResponse: request.PostForm.Get("cf-turnstile-response")}
	result, err := s.webVerification.AnswerWeb(request.Context(), token.ChallengeID, proof)
	copy := i18n.Messages.Verification.Web
	language := i18n.FromStored(record.Lang)
	if result.OperatorAlert && s.webOperatorAlert != nil {
		s.webOperatorAlert(request.Context(), record.GroupID)
	}
	if err != nil {
		renderVerification(writer, http.StatusServiceUnavailable, record, token, s.turnstileSiteKey, copy.Unavailable.For(language), nil)
		return
	}
	switch result.Outcome {
	case verification.WebWrong:
		s.showVerification(writer, request, copy.Wrong.For(language), nil)
	case verification.WebChannelRequired:
		s.showVerification(writer, request, copy.ChannelRequired.For(language), &result)
	case verification.WebFallback:
		s.webVerification.DeliverWebFallback(request.Context(), token.ChallengeID)
		renderVerificationNotice(writer, language, copy.Fallback.For(language))
	case verification.WebReceived:
		renderVerificationNotice(writer, language, copy.Received.For(language))
	default:
		renderVerification(writer, http.StatusOK, verification.PendingRecord{}, verification.WebTokenRecord{}, "", "", nil)
	}
}
