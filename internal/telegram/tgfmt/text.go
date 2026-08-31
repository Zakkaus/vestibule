package tgfmt

import (
	"strings"
	"unicode/utf16"
)

// MessageLimit is Telegram's maximum text length in UTF-16 code units.
const MessageLimit = 4096

// TextUnits measures Telegram text length in UTF-16 code units.
func TextUnits(text string) int {
	units := 0
	for _, r := range text {
		units += utf16.RuneLen(r)
	}
	return units
}

// CapText truncates text by UTF-16 units and appends an ellipsis when cut.
func CapText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if TextUnits(text) <= limit {
		return text
	}
	target := limit - 1
	var builder strings.Builder
	for _, r := range text {
		units := utf16.RuneLen(r)
		if target < units {
			break
		}
		builder.WriteRune(r)
		target -= units
	}
	builder.WriteRune('…')
	return builder.String()
}
