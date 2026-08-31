package tgfmt

import (
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// HTMLMessage builds the standard outbound HTML message with link previews disabled.
func HTMLMessage(chatID int64, text string) *telego.SendMessageParams {
	return tu.Message(tu.ID(chatID), text).
		WithParseMode(telego.ModeHTML).
		WithLinkPreviewOptions(&telego.LinkPreviewOptions{IsDisabled: true})
}

func StripHTML(text string) string {
	var builder strings.Builder
	builder.Grow(len(text))
	depth := 0
	for _, r := range text {
		switch {
		case r == '<':
			depth++
		case r == '>' && depth > 0:
			depth--
		case depth == 0:
			builder.WriteRune(r)
		}
	}
	plain := builder.String()
	for _, pair := range [][2]string{{"&lt;", "<"}, {"&gt;", ">"}, {"&quot;", "\""}, {"&#39;", "'"}, {"&amp;", "&"}} {
		plain = strings.ReplaceAll(plain, pair[0], pair[1])
	}
	return plain
}
