package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The screen that names the bot is the screen a visitor reaches with no
// session, so this answer cannot require one. It is also the screen that used
// to name a handle compiled into the frontend, which is why the value has to
// come from the running instance.
func TestInstanceNamesThisInstancesBotWithoutASession(t *testing.T) {
	for name, testCase := range map[string]struct {
		configured string
		want       string
	}{
		"claimed":                  {configured: "example_bot", want: "example_bot"},
		"claimed with a sigil":     {configured: "@example_bot", want: "example_bot"},
		"unclaimed has no bot yet": {configured: "", want: ""},
	} {
		server := New(Config{BotUsername: testCase.configured})
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/instance", nil))

		if response.Code != http.StatusOK {
			t.Fatalf("%s answered %d without a session, want 200", name, response.Code)
		}
		var body instanceResponse
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s returned undecodable JSON: %v", name, err)
		}
		if body.BotUsername != testCase.want {
			t.Fatalf("%s reported %q, want %q", name, body.BotUsername, testCase.want)
		}
	}
}
