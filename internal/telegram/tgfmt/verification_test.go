package tgfmt

import (
	"strings"
	"testing"
)

func TestJoinerLabel(t *testing.T) {
	const evil = `繁星帮<&>"` // an advert-style name with HTML metacharacters
	on := JoinerLabel(42, evil, true)
	if !strings.HasPrefix(on, "<tg-spoiler>") || !strings.HasSuffix(on, "</tg-spoiler>") {
		t.Errorf("spoiler-on should wrap the name in one <tg-spoiler> entity, got %q", on)
	}
	if strings.Contains(on, "<a ") || strings.Contains(on, "tg://user") {
		t.Errorf("spoiler-on must NOT emit a nested mention link (parse-safety), got %q", on)
	}
	if strings.Contains(on, "<&>") || strings.Contains(on, "\"") {
		t.Errorf("spoiler-on must HTML-escape the name, got %q", on)
	}
	off := JoinerLabel(42, evil, false)
	if !strings.Contains(off, `href="tg://user?id=42"`) {
		t.Errorf("spoiler-off should render a clickable mention, got %q", off)
	}
	if strings.Contains(off, "<&>") {
		t.Errorf("spoiler-off must HTML-escape the name, got %q", off)
	}
}
