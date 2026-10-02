package telegram

import (
	"context"
	"fmt"
	"html"
	"path"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/verification"
)

// SetWebVerification binds token issuance after constructing the shared service, before admission.
func (g *VerificationGateway) SetWebVerification(service *verification.Service, consoleURL string) {
	g.webService, g.consoleURL = service, strings.TrimRight(consoleURL, "/")
}

func (g *VerificationGateway) webMessage(ctx context.Context, message verification.OutgoingMessage) (verification.OutgoingMessage, error) {
	if g.webService == nil || g.consoleURL == "" || message.ChatID <= 0 {
		return message, fmt.Errorf("private web delivery is unavailable")
	}
	raw := message.WebToken
	if raw == "" || message.Resend {
		var err error
		raw, _, err = g.webService.IssueWebToken(ctx, message.ChallengeID)
		if err != nil {
			return message, err
		}
		if raw == "" {
			return message, fmt.Errorf("web challenge is no longer pending")
		}
	}
	base, err := consoleBaseURL(g.consoleURL)
	if err != nil {
		return message, err
	}
	base.Path = "/" + path.Join(base.Path, "verify", raw)
	base.RawPath = ""
	language := i18n.FromStored(message.Language)
	copy := i18n.Messages.Verification.Web
	if message.HTML {
		message.Text = html.EscapeString(copy.DMPrompt.For(language)) + "\n\n" + message.Text
	} else {
		message.Text = copy.DMPrompt.For(language)
	}
	message.DisableLinkPreview = true
	message.Buttons = [][]verification.Button{{{Text: copy.Button.For(language), URL: base.String()}}}
	return message, nil
}
