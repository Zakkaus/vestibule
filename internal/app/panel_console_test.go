package app

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/mymmrac/telego"
)

func TestPanelConsoleStartupWarning(t *testing.T) {
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	for _, test := range []struct {
		name, rawURL string
		warn         bool
	}{
		{"HTTPS", "https://console.example.test/groups", false},
		{"unset", "", false},
		{"loopback HTTP", "http://localhost:8080", true},
		{"remote HTTP", "http://console.example.test", true},
		{"malformed", "https://user:private-value@console.example.test/%zz", true},
		{"relative", "/groups", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			logs.Reset()
			setPanelConsoleURL(&panel.Panel{}, test.rawURL)
			got := logs.String()
			if !test.warn {
				if got != "" {
					t.Fatalf("unexpected console warning: %q", got)
				}
				return
			}
			if strings.Count(got, "\n") != 1 || !strings.Contains(got, "CONSOLE_URL") ||
				!strings.Contains(got, "HTTPS") {
				t.Fatalf("missing single console URL warning: %q", got)
			}
			if strings.Contains(got, test.rawURL) || strings.Contains(got, "private-value") {
				t.Fatalf("console warning disclosed URL data: %q", got)
			}
		})
	}
}

func TestConsoleAuthenticationDoesNotLogPanelURLWarnings(t *testing.T) {
	token := "1:" + strings.Repeat("a", 35)
	bot, err := telego.NewBot(token, telego.WithAPIServer("http://127.0.0.1:1"), telego.WithDiscardLogger())
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	for _, test := range []struct {
		rawURL  string
		allowed bool
	}{
		{"https://console.example.test", true},
		{"http://localhost:8080", true},
		{"https://user:private-value@console.example.test/%zz", false},
		{"/groups", false},
	} {
		t.Run(test.rawURL, func(t *testing.T) {
			logs.Reset()
			_, _, err := newConsoleAuthentication(
				Options{Token: token, ConsoleURL: test.rawURL}, nil, nil, bot, nil,
			)
			if (err == nil) != test.allowed {
				t.Fatalf("console authentication URL acceptance changed: error=%v", err)
			}
			if logs.Len() != 0 {
				t.Fatalf("console authentication emitted a panel URL warning: %q", logs.String())
			}
		})
	}
}
