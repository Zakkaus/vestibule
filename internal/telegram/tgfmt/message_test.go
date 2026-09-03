package tgfmt

import (
	"testing"

	"github.com/mymmrac/telego"
)

func TestHTMLMessagesKeepTheirTargetAndSuppressLinkPreviews(t *testing.T) {
	const (
		chatID int64 = -1009000001701
		text         = `<b>safe</b>`
	)

	message := HTMLMessage(chatID, text)
	if message.ChatID.ID != chatID || message.Text != text || message.ParseMode != telego.ModeHTML {
		t.Fatalf("HTML message = chat %d text %q parse mode %q, want chat %d unchanged text in HTML mode",
			message.ChatID.ID, message.Text, message.ParseMode, chatID)
	}
	if message.LinkPreviewOptions == nil || !message.LinkPreviewOptions.IsDisabled {
		t.Fatalf("HTML message link previews = %+v, want disabled: an automatic website preview would obscure the bot's answer",
			message.LinkPreviewOptions)
	}
}

func TestPlainFallbackKeepsAllVisibleHTMLCharacters(t *testing.T) {
	const (
		rich = `<blockquote expandable><b>&lt;kernel&gt;</b> Tom&#39;s &amp; Jerry</blockquote>`
		want = `<kernel> Tom's & Jerry`
	)

	if got := StripHTML(rich); got != want {
		t.Fatalf("plain fallback = %q, want %q: applicants on older Bot API servers would receive a damaged question", got, want)
	}
}
