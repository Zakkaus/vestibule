package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func requestFor(path, accept string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Language", "en")
	return request
}

// A person who mistyped an address, or opened a one-time link twice, was handed
// a JSON body. What they need is what to do instead.
func TestAnUnknownAddressAnswersABrowserWithAPage(t *testing.T) {
	server := New(Config{BotUsername: "example_bot"})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, requestFor("/mistyped", "text/html,application/xhtml+xml"))

	if response.Code != http.StatusNotFound {
		t.Fatalf("an unknown address answered %d, want 404", response.Code)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("a browser navigation was answered with %q", got)
	}
	body := response.Body.String()
	for name, want := range map[string]string{
		"title":         i18n.Messages.Bot.ErrorPage.NotFoundTitle.For(i18n.LangEN),
		"what to do":    i18n.Messages.Bot.ErrorPage.StepsLabel.For(i18n.LangEN),
		"the bot":       "@example_bot",
		"the way back":  i18n.Messages.Bot.ErrorPage.Action.For(i18n.LangEN),
		"its own style": "data-page-card",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("the error page omitted the %s (%q)", name, want)
		}
	}
	if strings.Contains(body, "\"error\"") {
		t.Fatal("the error page still carries the JSON body it replaced")
	}
}

// Probes, fetches and scripts are not helped by a page, and a console that
// answers them with one breaks every client that reads the error code.
func TestAnUnknownAddressStillAnswersAClientWithJSON(t *testing.T) {
	server := New(Config{})
	for name, accept := range map[string]string{
		"a fetch":              "application/json",
		"a probe":              "*/*",
		"no stated preference": "",
	} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, requestFor("/mistyped", accept))
		if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("%s was answered with %q instead of JSON", name, got)
		}
		if !strings.Contains(response.Body.String(), "not_found") {
			t.Fatalf("%s lost the error code clients read: %s", name, response.Body.String())
		}
	}
}

// An instance nobody has claimed has no bot, so the page cannot tell anyone to
// open one -- the same distinction the entry screen makes.
func TestTheErrorPageNamesNoBotOnAnUnclaimedInstance(t *testing.T) {
	page := (&Server{}).errorPageFor(i18n.LangEN, http.StatusNotFound)
	for _, step := range page.Steps {
		if strings.Contains(step, "@") {
			t.Fatalf("an unclaimed instance named a bot in %q", step)
		}
	}
	if len(page.Steps) == 0 {
		t.Fatal("an unclaimed instance was left with no next step at all")
	}
}
