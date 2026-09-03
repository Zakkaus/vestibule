package status

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
)

const sampleToken = "8684199281:AAFpLshbWf1GR6DqiZo2S0mxU8J_wfT0cqk"

type testErrorWriter struct{ err error }

func (w testErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestRedactTokenRemovesItFromAnAPIError(t *testing.T) {
	err := fmt.Errorf(`telego: getUpdates: request call: http do request: Post "http://127.0.0.1:8081/bot%s/getUpdates": connection refused`, sampleToken)
	got := RedactToken(err.Error())
	if strings.Contains(got, sampleToken) {
		t.Errorf("the token survived redaction: %q", got)
	}
	if !strings.Contains(got, "/bot<redacted>") || !strings.Contains(got, "connection refused") {
		t.Errorf("redaction lost the rest of the message: %q", got)
	}
}

func TestRedactionConsumesEverySupportedTokenCharacter(t *testing.T) {
	for _, character := range []string{"_", "-"} {
		token := "12345:" + character + strings.Repeat("x", 19)
		input := "Post https://api.telegram.org/bot" + token + "/getMe failed"
		want := "Post https://api.telegram.org/bot<redacted>/getMe failed"
		if got := RedactToken(input); got != want {
			t.Errorf("redaction left credential bytes after %q: got %q, want %q", character, got, want)
		}
	}
}

// The writer is the guard that does not depend on any call site remembering.
func TestRedactingWriterCatchesWhatALogCallForgot(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(RedactingWriter(&buf), "", 0)
	err := errors.New(`Post "http://127.0.0.1:8081/bot` + sampleToken + `/editMessageText": context deadline exceeded`)
	logger.Printf("feed: edit tracked bug 981623 in -1009999900009: %v", err)
	out := buf.String()
	if strings.Contains(out, sampleToken) {
		t.Errorf("the token reached the log: %q", out)
	}
	for _, keep := range []string{"feed: edit tracked bug 981623", "context deadline exceeded", "/bot<redacted>"} {
		if !strings.Contains(out, keep) {
			t.Errorf("log line lost %q: %q", keep, out)
		}
	}
}

func TestRedactingWriterReportsTheCallersLength(t *testing.T) {
	var buf bytes.Buffer
	w := RedactingWriter(&buf)
	p := []byte("/bot" + sampleToken + "/getMe\n")
	n, err := w.Write(p)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(p) {
		t.Errorf("Write reported %d of %d bytes; a short count looks like a partial write to log", n, len(p))
	}
}

func TestRedactingWriterPropagatesDestinationErrors(t *testing.T) {
	wantErr := errors.New("log destination unavailable")
	p := []byte("/bot" + sampleToken + "/getMe\n")
	n, err := RedactingWriter(testErrorWriter{err: wantErr}).Write(p)
	if n != 0 || !errors.Is(err, wantErr) {
		t.Fatalf("destination failure was hidden: Write returned n=%d err=%v", n, err)
	}
}

func TestRedactionLeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{
		"join 8745264859 in group -1009999900001: pending",
		"telego: getUpdates: Post \"http://127.0.0.1:8081/botBOT_TOKEN/getUpdates\": timeout",
		"/bot123",
	} {
		if got := RedactToken(s); got != s {
			t.Errorf("RedactToken(%q) = %q, want it unchanged", s, got)
		}
	}
}
