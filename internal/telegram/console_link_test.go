package telegram

import "testing"

func TestConsoleBaseURLAllowsHTTPSAndLoopbackHTTP(t *testing.T) {
	for _, rawURL := range []string{
		"http://console.example.test",
		"ftp://console.example.test",
		"https:///missing-host",
	} {
		if _, err := consoleBaseURL(rawURL); err == nil {
			t.Errorf("consoleBaseURL(%q) accepted an unsafe URL", rawURL)
		}
	}
	for _, rawURL := range []string{
		"https://console.example.test",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
	} {
		if _, err := consoleBaseURL(rawURL); err != nil {
			t.Errorf("consoleBaseURL(%q): %v", rawURL, err)
		}
	}
}
