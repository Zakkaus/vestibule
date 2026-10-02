package rules

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// SignalEntityKind is the closed set of entity shapes used for scoring.
type SignalEntityKind uint8

const (
	EntityURL SignalEntityKind = iota + 1
	EntityTextLink
	EntityMention
	EntityTextMention
)

// SignalEntity contains a platform-parsed entity; URL is the target of either link kind.
type SignalEntity struct {
	Kind SignalEntityKind
	URL  string
}

// SignalInput keeps original text and elapsed membership time, not a platform message.
type SignalInput struct {
	Text      string
	Entities  []SignalEntity
	Joined    time.Duration
	KnownJoin bool
}

// SignalKind is the closed set of structural signals, in hit reporting order.
type SignalKind uint8

const (
	SignalHiddenCharacters SignalKind = iota + 1
	SignalPrivateInviteLinks
	SignalMentions
	SignalLinks
	SignalNewMemberWithLink
)

// SignalWeights assigns points per occurrence; the membership signal occurs at most once.
type SignalWeights struct {
	HiddenCharacters   int
	PrivateInviteLinks int
	Mentions           int
	Links              int
	NewMemberWithLink  int
}

// DefaultSignalWeights gives an ordinary link 1 point, below an illustrative threshold of 10;
// two private invites with one hidden character from a new member give 14, clearly above it.
var DefaultSignalWeights = SignalWeights{
	HiddenCharacters: 2, PrivateInviteLinks: 3, Mentions: 1, Links: 1, NewMemberWithLink: 4,
}

// SignalHit reports the occurrence count and its weighted contribution, including zero weights.
type SignalHit struct {
	Signal SignalKind
	Count  int
	Points int
}

// SignalScore reports the sum and one hit per present signal, in SignalKind order.
type SignalScore struct {
	Total int
	Hits  []SignalHit
}

// ScoreSignals reads only supplied entities for links and mentions and never changes its input.
func ScoreSignals(input SignalInput, weights SignalWeights) SignalScore {
	var counts [5]int
	counts[SignalHiddenCharacters-1] = hiddenCharacterCount(input.Text)
	for _, entity := range input.Entities {
		switch entity.Kind {
		case EntityURL, EntityTextLink:
			counts[SignalLinks-1]++
			if privateInviteURL(entity.URL) {
				counts[SignalPrivateInviteLinks-1]++
			}
		case EntityMention, EntityTextMention:
			counts[SignalMentions-1]++
		}
	}
	if input.KnownJoin && input.Joined < 24*time.Hour && counts[SignalLinks-1] > 0 {
		counts[SignalNewMemberWithLink-1] = 1
	}
	// Indexed by SignalKind-1, matching counts.
	values := [5]int{weights.HiddenCharacters, weights.PrivateInviteLinks, weights.Mentions, weights.Links, weights.NewMemberWithLink}
	present := 0
	for _, count := range counts {
		if count > 0 {
			present++
		}
	}
	var result SignalScore
	if present > 0 {
		result.Hits = make([]SignalHit, 0, present)
	}
	for i, count := range counts {
		if count > 0 {
			points := count * values[i]
			result.Total += points
			result.Hits = append(result.Hits, SignalHit{Signal: SignalKind(i + 1), Count: count, Points: points})
		}
	}
	return result
}

func hiddenCharacterCount(text string) int {
	count := 0
	var previous rune
	for offset, r := range text {
		if invisible(r) && !emojiInvisible(text, offset, r, previous) {
			count++
		}
		previous = r
	}
	// Normalize does not drop tag characters, so flag tags are already excluded by invisible.
	return count
}

func emojiInvisible(text string, offset int, r, previous rune) bool {
	switch r {
	case '\ufe0e', '\ufe0f':
		return unicode.Is(extendedPictographic, previous) || keycapBase(previous)
	case '\u200d':
		// Presentation selectors, skin tones and flag tags can precede a joiner in qualified emoji.
		before := offset
		for before > 0 {
			left, size := utf8.DecodeLastRuneInString(text[:before])
			before -= size
			if left == '\ufe0e' || left == '\ufe0f' || (left >= '\U0001f3fb' && left <= '\U0001f3ff') || (left >= '\U000e0020' && left <= '\U000e007f') {
				continue
			}
			right, _ := utf8.DecodeRuneInString(text[offset+utf8.RuneLen(r):])
			return unicode.Is(extendedPictographic, left) && unicode.Is(extendedPictographic, right)
		}
	}
	return false
}

func keycapBase(r rune) bool {
	return (r >= '0' && r <= '9') || r == '#' || r == '*'
}

func privateInviteURL(target string) bool {
	// The net package boundary excludes net/url; inspect entity targets, never free text.
	// Forms follow https://core.telegram.org/api/links#chat-invite-links.
	if scheme, rest, found := strings.Cut(target, ":"); found && strings.EqualFold(scheme, "tg") {
		rest = strings.TrimPrefix(rest, "//")
		action, query, _ := strings.Cut(rest, "?")
		return strings.EqualFold(action, "join") && strings.Contains("&"+query, "&invite=")
	}
	if _, rest, found := strings.Cut(target, "://"); found {
		target = rest
	}
	target = strings.TrimPrefix(target, "//")
	end := strings.IndexAny(target, "/?#")
	if end < 0 || target[end] != '/' {
		return false
	}
	authority, path := target[:end], target[end:]
	if userinfo := strings.LastIndexByte(authority, '@'); userinfo >= 0 {
		authority = authority[userinfo+1:]
	}
	host, _, _ := strings.Cut(authority, ":")
	if !strings.EqualFold(host, "t.me") && !strings.EqualFold(host, "telegram.me") && !strings.EqualFold(host, "telegram.dog") {
		return false
	}
	if hash, ok := strings.CutPrefix(path, "/+"); ok {
		// t.me/+<digits> is a phone-number contact link, not an invite.
		hash, _, _ = strings.Cut(hash, "/")
		return strings.Trim(hash, "0123456789") != ""
	}
	return strings.HasPrefix(path, "/joinchat/")
}

// Unicode 17.0 Extended_Pictographic, with adjacent ranges merged; Go omits this property.
// Source: https://www.unicode.org/Public/17.0.0/ucd/emoji/emoji-data.txt
var extendedPictographic = &unicode.RangeTable{
	R16: []unicode.Range16{
		{0x00a9, 0x00a9, 1}, {0x00ae, 0x00ae, 1},
		{0x203c, 0x203c, 1}, {0x2049, 0x2049, 1},
		{0x2122, 0x2122, 1}, {0x2139, 0x2139, 1},
		{0x2194, 0x2199, 1}, {0x21a9, 0x21aa, 1},
		{0x231a, 0x231b, 1}, {0x2328, 0x2328, 1},
		{0x23cf, 0x23cf, 1}, {0x23e9, 0x23f3, 1}, {0x23f8, 0x23fa, 1},
		{0x24c2, 0x24c2, 1}, {0x25aa, 0x25ab, 1},
		{0x25b6, 0x25b6, 1}, {0x25c0, 0x25c0, 1}, {0x25fb, 0x25fe, 1},
		{0x2600, 0x2604, 1},
		{0x260e, 0x260e, 1}, {0x2611, 0x2611, 1}, {0x2614, 0x2615, 1},
		{0x2618, 0x2618, 1}, {0x261d, 0x261d, 1}, {0x2620, 0x2620, 1},
		{0x2622, 0x2623, 1}, {0x2626, 0x2626, 1}, {0x262a, 0x262a, 1},
		{0x262e, 0x262f, 1}, {0x2638, 0x263a, 1},
		{0x2640, 0x2640, 1}, {0x2642, 0x2642, 1}, {0x2648, 0x2653, 1},
		{0x265f, 0x2660, 1}, {0x2663, 0x2663, 1}, {0x2665, 0x2666, 1},
		{0x2668, 0x2668, 1}, {0x267b, 0x267b, 1}, {0x267e, 0x267f, 1},
		{0x2692, 0x2697, 1}, {0x2699, 0x2699, 1}, {0x269b, 0x269c, 1},
		{0x26a0, 0x26a1, 1}, {0x26a7, 0x26a7, 1}, {0x26aa, 0x26ab, 1},
		{0x26b0, 0x26b1, 1}, {0x26bd, 0x26be, 1}, {0x26c4, 0x26c5, 1},
		{0x26c8, 0x26c8, 1}, {0x26ce, 0x26cf, 1}, {0x26d1, 0x26d1, 1},
		{0x26d3, 0x26d4, 1}, {0x26e9, 0x26ea, 1}, {0x26f0, 0x26f5, 1},
		{0x26f7, 0x26fa, 1}, {0x26fd, 0x26fd, 1},
		{0x2702, 0x2702, 1}, {0x2705, 0x2705, 1}, {0x2708, 0x270d, 1},
		{0x270f, 0x270f, 1}, {0x2712, 0x2712, 1}, {0x2714, 0x2714, 1},
		{0x2716, 0x2716, 1}, {0x271d, 0x271d, 1}, {0x2721, 0x2721, 1},
		{0x2728, 0x2728, 1}, {0x2733, 0x2734, 1}, {0x2744, 0x2744, 1},
		{0x2747, 0x2747, 1}, {0x274c, 0x274c, 1}, {0x274e, 0x274e, 1},
		{0x2753, 0x2755, 1}, {0x2757, 0x2757, 1}, {0x2763, 0x2764, 1},
		{0x2795, 0x2797, 1}, {0x27a1, 0x27a1, 1}, {0x27b0, 0x27b0, 1},
		{0x27bf, 0x27bf, 1}, {0x2934, 0x2935, 1}, {0x2b05, 0x2b07, 1},
		{0x2b1b, 0x2b1c, 1}, {0x2b50, 0x2b50, 1}, {0x2b55, 0x2b55, 1},
		{0x3030, 0x3030, 1}, {0x303d, 0x303d, 1},
		{0x3297, 0x3297, 1}, {0x3299, 0x3299, 1},
	},
	R32: []unicode.Range32{
		{0x1f004, 0x1f004, 1}, {0x1f02c, 0x1f02f, 1},
		{0x1f094, 0x1f09f, 1}, {0x1f0af, 0x1f0b0, 1},
		{0x1f0c0, 0x1f0c0, 1}, {0x1f0cf, 0x1f0d0, 1}, {0x1f0f6, 0x1f0ff, 1},
		{0x1f170, 0x1f171, 1}, {0x1f17e, 0x1f17f, 1}, {0x1f18e, 0x1f18e, 1},
		{0x1f191, 0x1f19a, 1}, {0x1f1ae, 0x1f1e5, 1}, {0x1f201, 0x1f20f, 1},
		{0x1f21a, 0x1f21a, 1}, {0x1f22f, 0x1f22f, 1}, {0x1f232, 0x1f23a, 1},
		{0x1f23c, 0x1f23f, 1}, {0x1f249, 0x1f25f, 1}, {0x1f266, 0x1f321, 1},
		{0x1f324, 0x1f393, 1}, {0x1f396, 0x1f397, 1}, {0x1f399, 0x1f39b, 1},
		{0x1f39e, 0x1f3f0, 1}, {0x1f3f3, 0x1f3f5, 1}, {0x1f3f7, 0x1f3fa, 1},
		{0x1f400, 0x1f4fd, 1}, {0x1f4ff, 0x1f53d, 1}, {0x1f549, 0x1f54e, 1},
		{0x1f550, 0x1f567, 1}, {0x1f56f, 0x1f570, 1},
		{0x1f573, 0x1f57a, 1}, {0x1f587, 0x1f587, 1}, {0x1f58a, 0x1f58d, 1},
		{0x1f590, 0x1f590, 1}, {0x1f595, 0x1f596, 1}, {0x1f5a4, 0x1f5a5, 1},
		{0x1f5a8, 0x1f5a8, 1}, {0x1f5b1, 0x1f5b2, 1}, {0x1f5bc, 0x1f5bc, 1},
		{0x1f5c2, 0x1f5c4, 1}, {0x1f5d1, 0x1f5d3, 1}, {0x1f5dc, 0x1f5de, 1},
		{0x1f5e1, 0x1f5e1, 1}, {0x1f5e3, 0x1f5e3, 1}, {0x1f5e8, 0x1f5e8, 1},
		{0x1f5ef, 0x1f5ef, 1}, {0x1f5f3, 0x1f5f3, 1}, {0x1f5fa, 0x1f64f, 1},
		{0x1f680, 0x1f6c5, 1}, {0x1f6cb, 0x1f6d2, 1}, {0x1f6d5, 0x1f6e5, 1},
		{0x1f6e9, 0x1f6e9, 1}, {0x1f6eb, 0x1f6f0, 1}, {0x1f6f3, 0x1f6ff, 1},
		{0x1f7da, 0x1f7ff, 1}, {0x1f80c, 0x1f80f, 1}, {0x1f848, 0x1f84f, 1},
		{0x1f85a, 0x1f85f, 1}, {0x1f888, 0x1f88f, 1}, {0x1f8ae, 0x1f8af, 1},
		{0x1f8bc, 0x1f8bf, 1}, {0x1f8c2, 0x1f8cf, 1}, {0x1f8d9, 0x1f8ff, 1},
		{0x1f90c, 0x1f93a, 1}, {0x1f93c, 0x1f945, 1}, {0x1f947, 0x1f9ff, 1},
		{0x1fa58, 0x1fa5f, 1}, {0x1fa6e, 0x1faff, 1}, {0x1fc00, 0x1fffd, 1},
	},
}
