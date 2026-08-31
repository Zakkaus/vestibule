package rules

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Verdict distinguishes an accepted value, an explicit reject condition, and no match.
type Verdict uint8

const (
	NoMatch Verdict = iota
	Accepted
	Rejected
)

// Condition is sealed so rule data can use only condition types implemented by this package.
type Condition interface {
	matches(string) bool
}

// Rule accepts only when every Accept condition matches and no Reject condition matches.
type Rule struct {
	Accept []Condition
	Reject []Condition
}

// Evaluate applies the rule to one original input string.
func (r Rule) Evaluate(text string) Verdict {
	for _, condition := range r.Reject {
		if condition.matches(text) {
			return Rejected
		}
	}
	if len(r.Accept) == 0 {
		return NoMatch
	}
	for _, condition := range r.Accept {
		if !condition.matches(text) {
			return NoMatch
		}
	}
	return Accepted
}

// OneOf matches a complete normalized value from Values.
type OneOf struct {
	Values []string
}

func (c OneOf) matches(text string) bool { return c.Matches(text) }

// Matches compares text after the shared text normalization pipeline.
func (c OneOf) Matches(text string) bool {
	return c.matchesNormalized(Normalize(text), Normalize)
}

// MatchesAnswer preserves the answer-specific URL and punctuation cleanup used by fallback questions.
func (c OneOf) MatchesAnswer(text string) bool {
	return c.matchesNormalized(normalizeAnswer(text), normalizeAnswer)
}

func (c OneOf) matchesNormalized(text string, normalize func(string) string) bool {
	if text == "" {
		return false
	}
	for _, value := range c.Values {
		if text == normalize(value) {
			return true
		}
	}
	return false
}

// Contains matches Value as a normalized substring. CompactWhitespace ignores all Unicode whitespace.
type Contains struct {
	Value             string
	CompactWhitespace bool
}

func (c Contains) matches(text string) bool { return c.Matches(text) }

// Matches compares text after the shared text normalization pipeline.
func (c Contains) Matches(text string) bool {
	return c.MatchesNormalized(Normalize(text))
}

// MatchesNormalized compares text that already passed Normalize.
func (c Contains) MatchesNormalized(text string) bool {
	value := Normalize(c.Value)
	if c.CompactWhitespace {
		text, value = compactWhitespace(text), compactWhitespace(value)
	}
	return value != "" && strings.Contains(text, value)
}

func compactWhitespace(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
}

// NumberRange recognizes the last decimal value inside the inclusive range.
type NumberRange struct {
	Minimum int
	Maximum int
}

func (r NumberRange) matches(text string) bool {
	_, ok := r.Last(text)
	return ok
}

// Last returns the last decimal value in the inclusive range.
func (r NumberRange) Last(text string) (int, bool) {
	if r.Minimum > r.Maximum {
		return 0, false
	}
	normalized := Normalize(text)
	last, found := 0, false
	for _, match := range numberPattern.FindAllStringIndex(normalized, -1) {
		value, err := strconv.Atoi(normalized[match[0]:match[1]])
		if err == nil && value >= r.Minimum && value <= r.Maximum {
			last, found = value, true
		}
	}
	return last, found

}

// Version identifies the major and minor components used to bound a kernel release.
type Version struct {
	Major int
	Minor int
}

// VersionInterval is one inclusive major/minor interval within a VersionRange.
type VersionInterval struct {
	Minimum Version
	Maximum Version
}

// VersionRange recognizes release strings and known kernel command-output shapes.
type VersionRange struct {
	Intervals []VersionInterval
}

func (r VersionRange) matches(text string) bool { return r.Matches(text) }

// Matches accepts a kernel release alone or embedded in known command-output context.
func (r VersionRange) Matches(text string) bool {
	text = stripCommandEcho(Normalize(text))
	if text == "" {
		return false
	}
	if match := kernelReleaseRe.FindStringSubmatch(text); match != nil {
		return r.accepts(match[1])
	}
	for _, expression := range kernelMultiVersionOutputs {
		if match := expression.FindStringSubmatch(text); match != nil {
			return r.accepts(match[1])
		}
	}
	matches := kernelReleaseTokenRe.FindAllStringIndex(text, -1)
	if len(matches) != 1 {
		return false
	}
	if match := wslKernelOutputRe.FindStringSubmatch(text); match != nil {
		return r.accepts(match[1])
	}
	match := matches[0]
	release := text[match[0]:match[1]]
	version := kernelReleaseRe.FindStringSubmatch(release)
	if version == nil || !r.accepts(version[1]) {
		return false
	}
	return benignKernelContext(text[:match[0]], text[match[1]:], kernelReleaseDistribution(release))
}

func (r VersionRange) accepts(text string) bool {
	parts := strings.Split(text, ".")
	if len(parts) > 4 {
		return false
	}
	for _, part := range parts[1:] {
		if len(part) > 4 {
			return false
		}
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	version := Version{Major: major, Minor: minor}
	for _, interval := range r.Intervals {
		if versionAtLeast(version, interval.Minimum) && versionAtMost(version, interval.Maximum) {
			return true
		}
	}
	return false
}

func versionAtLeast(left, right Version) bool {
	return left.Major > right.Major || left.Major == right.Major && left.Minor >= right.Minor
}

func versionAtMost(left, right Version) bool {
	return left.Major < right.Major || left.Major == right.Major && left.Minor <= right.Minor
}

const kernelReleasePattern = `[vV]?(\d{1,3}(?:\.\d{1,6}){1,3})(?:[-+_][0-9A-Za-z][0-9A-Za-z._+-]*)?`

var (
	kernelReleaseRe           = regexp.MustCompile(`^` + kernelReleasePattern + `$`)
	kernelReleaseTokenRe      = regexp.MustCompile(kernelReleasePattern)
	kernelContextWordRe       = regexp.MustCompile(`[-#]?[0-9A-Za-z](?:[0-9A-Za-z_./+-]*[0-9A-Za-z])?`)
	kernelHostnameRe          = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)
	kernelDateNumberRe        = regexp.MustCompile(`^(?:[0-9]{1,2}|[0-9]{4})$`)
	wslKernelOutputRe         = regexp.MustCompile(`(?i)^Windows\s+WSL[0-9]*(?:\s+kernel\s+|\s*,\s*)` + kernelReleasePattern + `$`)
	kernelMultiVersionOutputs = [...]*regexp.Regexp{
		regexp.MustCompile(`^linux version ` + kernelReleasePattern + `\s+\(.+$`),
		regexp.MustCompile(`(?i)^(?:uname\s+-a\s*:?\s*)?Linux\s+\S+\s+` + kernelReleasePattern + `\s+#\d+\b`),
	}
	kernelCommandEchoRe = regexp.MustCompile(`(?i)^\s*(?:[^$#>]*[$#>]\s*)?(?:sudo\s+)?(?:uname\b|cat\s+/proc/version\b|hostnamectl\b).*$`)
	numberPattern       = regexp.MustCompile(`[0-9]+`)
	agentReplyPattern   = regexp.MustCompile(`(?i)^model=[0-9a-z][0-9a-z.:_/+-]*$`)
)

var benignKernelContextWords = map[string]struct{}{
	"#1": {}, "-a": {}, "-r": {}, "-sr": {},
	"linux": {}, "uname": {}, "gnu/linux": {},
	"smp": {}, "preempt": {}, "preempt_dynamic": {},
	"x86_64": {}, "amd64": {}, "aarch64": {}, "arm64": {}, "i686": {},
	"armv7l": {}, "armv8l": {}, "riscv64": {}, "ppc64le": {}, "s390x": {},
	"kernel": {}, "version": {}, "my": {}, "is": {}, "it": {}, "the": {},
	"on": {}, "running": {}, "now": {}, "currently": {}, "here": {}, "use": {}, "using": {},
	"i": {}, "am": {},
	"mon": {}, "tue": {}, "wed": {}, "thu": {}, "fri": {}, "sat": {}, "sun": {},
	"jan": {}, "feb": {}, "mar": {}, "apr": {}, "may": {}, "jun": {},
	"jul": {}, "aug": {}, "sep": {}, "oct": {}, "nov": {}, "dec": {},
	"utc": {}, "gmt": {},
}

func stripCommandEcho(text string) string {
	if !strings.ContainsAny(text, "\n\r") {
		return text
	}
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	kept := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) == "" || kernelCommandEchoRe.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return text
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func kernelReleaseDistribution(release string) string {
	for _, suffix := range []string{"-gentoo", "_gentoo"} {
		if index := strings.Index(release, suffix); index >= 0 {
			end := index + len(suffix)
			if end == len(release) || strings.ContainsRune("._+-", rune(release[end])) {
				return "gentoo"
			}
		}
	}
	return ""
}

func benignKernelContext(before, after, distribution string) bool {
	beforeWords := kernelContextWordRe.FindAllString(before, -1)
	unameShape := len(beforeWords) > 0 && strings.EqualFold(beforeWords[0], "linux")
	for index, word := range beforeWords {
		word = strings.ToLower(word)
		if _, ok := benignKernelContextWords[word]; ok {
			continue
		}
		if word == distribution {
			continue
		}
		if index == 1 && unameShape && kernelHostnameRe.MatchString(word) {
			continue
		}
		return false
	}
	if unameShape && kernelUnameTailRe.MatchString(after) {
		return true
	}
	for _, word := range kernelContextWordRe.FindAllString(after, -1) {
		word = strings.ToLower(word)
		if _, ok := benignKernelContextWords[word]; ok {
			continue
		}
		if word == distribution {
			continue
		}
		if unameShape && kernelDateNumberRe.MatchString(word) {
			continue
		}
		return false
	}
	return true
}

var kernelUnameTailRe = regexp.MustCompile(`^\s*#\d+`)

// AgentToken derives the nonce-bound token printed in the automated-agent instruction.
func AgentToken(nonce string) string {
	if nonce == "" {
		return "AGENT-STOP"
	}
	return "AGENT-" + strings.ToUpper(nonce)
}

// AgentReply recognizes only the exact nonce-bound automated-agent response shape.
func AgentReply(text, nonce string) bool {
	text = strings.TrimSpace(text)
	token := AgentToken(nonce)
	if len(text) <= len(token) || !strings.EqualFold(text[:len(token)], token) || text[len(token)] != ' ' {
		return false
	}
	return agentReplyPattern.MatchString(strings.TrimSpace(text[len(token)+1:]))
}

// MinuteProof accepts the current minute with one minute of clock slack and real timezone offsets.
func MinuteProof(text string, now time.Time) bool {
	claimed, ok := (NumberRange{Minimum: 0, Maximum: 59}).Last(text)
	if !ok {
		return false
	}
	for _, shift := range [...]int{0, 30, 45} {
		difference := ((claimed-now.Minute()-shift)%60%60 + 60) % 60
		if difference <= 1 || difference >= 59 {
			return true
		}
	}
	return false
}
