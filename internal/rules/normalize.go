// Package rules evaluates normalized, side-effect-free text conditions.
package rules

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Normalize canonicalizes text used only for matching; callers keep the original text for display and actions.
func Normalize(in string) string {
	s := foldFullWidth(in)
	s = dropInvisible(s)
	s = foldCJKSeparators(s)
	return strings.ToLower(strings.TrimSpace(s))
}

func foldFullWidth(in string) string {
	for _, r := range in {
		if r == '\u3000' || (r >= '\uff01' && r <= '\uff5e') {
			return strings.Map(func(r rune) rune {
				switch {
				case r == '\u3000':
					return ' '
				case r >= '\uff01' && r <= '\uff5e':
					return r - 0xfee0
				default:
					return r
				}
			}, in)
		}
	}
	return in
}

func dropInvisible(in string) string {
	for _, r := range in {
		if invisible(r) {
			return strings.Map(func(r rune) rune {
				if invisible(r) {
					return -1
				}
				return r
			}, in)
		}
	}
	return in
}

func invisible(r rune) bool {
	switch {
	case r >= '\u200b' && r <= '\u200f':
		return true
	case r >= '\u2060' && r <= '\u2064':
		return true
	case r == '\ufeff':
		return true
	case r >= '\ufe00' && r <= '\ufe0f':
		return true
	case r >= '\U000e0100' && r <= '\U000e01ef':
		return true
	default:
		return false
	}
}

func foldCJKSeparators(in string) string {
	var out strings.Builder
	copyFrom := 0
	var previous rune
	havePrevious := false

	for offset := 0; offset < len(in); {
		r, size := utf8.DecodeRuneInString(in[offset:])
		if !cjkSeparator(r) {
			previous, havePrevious = r, true
			offset += size
			continue
		}

		start := offset
		last := r
		for offset += size; offset < len(in); {
			next, nextSize := utf8.DecodeRuneInString(in[offset:])
			if !cjkSeparator(next) {
				break
			}
			last = next
			offset += nextSize
		}
		if havePrevious && unicode.Is(unicode.Han, previous) && offset < len(in) {
			next, _ := utf8.DecodeRuneInString(in[offset:])
			if unicode.Is(unicode.Han, next) {
				if out.Len() == 0 {
					out.Grow(len(in))
				}
				out.WriteString(in[copyFrom:start])
				copyFrom = offset
				continue
			}
		}
		previous, havePrevious = last, true
	}

	if out.Len() == 0 {
		return in
	}
	out.WriteString(in[copyFrom:])
	return out.String()
}

func cjkSeparator(r rune) bool {
	switch r {
	case '·', '*', '-', '_', '.':
		return true
	default:
		return unicode.IsSpace(r)
	}
}

func normalizeAnswer(in string) string {
	in = Normalize(in)
	in = strings.TrimSpace(strings.TrimFunc(in, unicode.IsPunct))
	in = strings.TrimPrefix(in, "https://")
	in = strings.TrimPrefix(in, "http://")
	in = strings.TrimPrefix(in, "www.")
	return strings.TrimSpace(strings.TrimFunc(in, unicode.IsPunct))
}
