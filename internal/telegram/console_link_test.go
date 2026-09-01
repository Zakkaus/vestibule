package telegram

import "testing"

func TestConsoleBaseURLRequiresHTTPS(t *testing.T) {
	for _, rawURL := range []string{
		"http://console.example.test",
		"http://127.0.0.1:8080",
		"http://[::1]:8080",
		"ftp://console.example.test",
		"https:///missing-host",
	} {
		if _, err := consoleBaseURL(rawURL); err == nil {
			t.Fatalf("consoleBaseURL(%q) accepted an unsafe URL", rawURL)
		}
	}
	if _, err := consoleBaseURL("https://console.example.test"); err != nil {
		t.Fatalf("consoleBaseURL(https): %v", err)
	}
}
