package app

import "testing"

func TestDefaultConsoleServerAddressIsLoopbackOnly(t *testing.T) {
	if got := consoleAddress(""); got != "127.0.0.1:8080" {
		t.Fatalf("unset CONSOLE_ADDR exposes the operator and setup console outside loopback: got %q", got)
	}
}
