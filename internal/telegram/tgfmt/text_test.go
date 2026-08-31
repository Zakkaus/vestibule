package tgfmt

import (
	"testing"
	"unicode/utf8"
)

func TestTextUnits(t *testing.T) {
	text := "a😀界"
	if got := TextUnits(text); got != 4 {
		t.Fatalf("TextUnits(%q) = %d, want 4", text, got)
	}
	capped := CapText("a😀bc", 4)
	if capped != "a😀…" || !utf8.ValidString(capped) || TextUnits(capped) > 4 {
		t.Errorf("CapText() = %q (%d units)", capped, TextUnits(capped))
	}
	if got := CapText("unchanged", MessageLimit); got != "unchanged" {
		t.Errorf("short text changed to %q", got)
	}
}
