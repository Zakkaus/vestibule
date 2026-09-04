package telegram

import "testing"

func TestConsoleBaseURLAllowsHTTPSAndLoopbackHTTP(t *testing.T) {
	for _, rawURL := range []string{
		"https://console.example.test",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	} {
		if _, err := consoleBaseURL(rawURL); err != nil {
			t.Fatalf("consoleBaseURL(%q): %v", rawURL, err)
		}
	}
	for _, rawURL := range []string{
		"http://console.example.test",
		"ftp://console.example.test",
		"https:///missing-host",
	} {
		if _, err := consoleBaseURL(rawURL); err == nil {
			t.Fatalf("consoleBaseURL(%q) accepted an unsafe URL", rawURL)
		}
	}
}
