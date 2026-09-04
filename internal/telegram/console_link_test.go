package telegram

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

// The link carries a one-time credential in its path, so plain HTTP over a
// network hands that credential to the network. The loopback interface is not a
// network: the request never leaves the machine, and reaching it from anywhere
// else means a port forward, whose transport is the tunnel's. Refusing loopback
// too meant the one deployment the design document describes for an operator
// with no domain -- bound to localhost, opened over a forward -- could not
// activate at all.
func TestConsoleBaseURLRequiresHTTPSOffTheLoopback(t *testing.T) {
	for _, rawURL := range []string{
		"http://console.example.test",
		"http://10.0.0.4:8080",
		"ftp://console.example.test",
		"https:///missing-host",
		"console.example.test",
	} {
		if _, err := consoleBaseURL(rawURL); err == nil {
			t.Fatalf("consoleBaseURL(%q) accepted a credential-carrying URL over an open network", rawURL)
		}
	}
	for _, rawURL := range []string{
		"https://console.example.test",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"http://localhost:8080",
	} {
		if _, err := consoleBaseURL(rawURL); err != nil {
			t.Fatalf("consoleBaseURL(%q): %v", rawURL, err)
		}
	}
}

// A one-time credential pasted as message text sits in the chat: selectable,
// forwardable, and shown in every preview of that conversation. It goes behind
// a button, except where a button cannot reach it.
func TestConsoleLinkTravelsInAButtonOffTheLoopback(t *testing.T) {
	base, err := consoleBaseURL("https://console.example.test")
	if err != nil {
		t.Fatal(err)
	}
	remote := consoleLinkMessage(7, base, "issued-token", i18n.LangEN)
	if remote.ReplyMarkup == nil {
		t.Fatal("the console link was sent without a button, so the token is chat text")
	}
	markup, ok := remote.ReplyMarkup.(*telego.InlineKeyboardMarkup)
	if !ok || len(markup.InlineKeyboard) != 1 || len(markup.InlineKeyboard[0]) != 1 {
		t.Fatalf("the console link carries an unexpected keyboard: %#v", remote.ReplyMarkup)
	}
	if !strings.Contains(markup.InlineKeyboard[0][0].URL, "issued-token") {
		t.Fatalf("the button does not open the issued link: %q", markup.InlineKeyboard[0][0].URL)
	}
	if strings.Contains(remote.Text, "issued-token") {
		t.Fatalf("the token is in the message text as well as the button: %q", remote.Text)
	}

	// A loopback console cannot be opened from a phone, and Telegram will not
	// make a button for an address it cannot reach, so the address is the text.
	local, err := consoleBaseURL("http://127.0.0.1:8123")
	if err != nil {
		t.Fatal(err)
	}
	loopback := consoleLinkMessage(7, local, "issued-token", i18n.LangEN)
	if loopback.ReplyMarkup != nil {
		t.Fatal("a loopback address was offered as a button Telegram cannot open")
	}
	if !strings.Contains(loopback.Text, "issued-token") {
		t.Fatalf("the loopback message does not carry the address: %q", loopback.Text)
	}
}
