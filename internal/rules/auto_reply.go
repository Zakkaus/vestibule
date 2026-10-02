package rules

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const AutoReplyCollection = "auto_reply"

// DefinitionError identifies an invalid field without exposing administrator-supplied text.
type DefinitionError struct{ Field, Code string }

func (e *DefinitionError) Error() string { return e.Field + ": " + e.Code }

// AutoReply is a decoded rule; text is normalized only for matching, never for delivery.
type AutoReply struct {
	Reply     string
	Cooldown  time.Duration
	condition Condition
}

type autoReplyDefinition struct {
	Trigger         string   `json:"trigger"`
	MatchMode       string   `json:"match_mode"`
	Alternatives    []string `json:"alternatives"`
	Reply           string   `json:"reply"`
	CooldownSeconds *int64   `json:"cooldown_seconds"`
}

// DecodeAutoReply validates and compiles the stored definition once per definition change.
func DecodeAutoReply(data json.RawMessage) (AutoReply, error) {
	var definition autoReplyDefinition
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if !json.Valid(data) || decoder.Decode(&definition) != nil {
		return AutoReply{}, &DefinitionError{Field: "definition", Code: "invalid_definition"}
	}
	var trigger string
	if definition.MatchMode == "hashtag" {
		trigger = normalizeHashtag(definition.Trigger)
	} else {
		trigger = Normalize(definition.Trigger)
	}
	if trigger == "" || utf8.RuneCountInString(definition.Trigger) > 512 {
		return AutoReply{}, &DefinitionError{Field: "trigger", Code: "invalid_trigger"}
	}
	if strings.TrimSpace(definition.Reply) == "" || utf8.RuneCountInString(definition.Reply) > 4096 {
		return AutoReply{}, &DefinitionError{Field: "reply", Code: "invalid_reply"}
	}
	seconds := int64(600)
	if definition.CooldownSeconds != nil {
		seconds = *definition.CooldownSeconds
	}
	if seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
		return AutoReply{}, &DefinitionError{Field: "cooldown_seconds", Code: "invalid_cooldown"}
	}
	condition, err := autoReplyCondition(definition)
	if err != nil {
		return AutoReply{}, err
	}
	return AutoReply{Reply: definition.Reply, Cooldown: time.Duration(seconds) * time.Second, condition: condition}, nil
}

func autoReplyCondition(d autoReplyDefinition) (Condition, error) {
	if len(d.Alternatives) > 0 && d.MatchMode != "one_of" {
		return nil, &DefinitionError{Field: "alternatives", Code: "invalid_alternatives"}
	}
	switch d.MatchMode {
	case "", "contains":
		return Contains{Value: d.Trigger}, nil
	case "equals":
		return OneOf{Values: []string{d.Trigger}}, nil
	case "one_of":
		return autoReplyAlternatives(d)
	case "hashtag":
		tag := strings.TrimPrefix(normalizeHashtag(d.Trigger), "#")
		if tag == "" || strings.IndexFunc(tag, func(r rune) bool { return !hashtagRune(r) }) >= 0 {
			return nil, &DefinitionError{Field: "trigger", Code: "invalid_hashtag"}
		}
		return completeHashtag(tag), nil
	case "regex":
		// Go's RE2 engine is linear-time; the expression is bounded above to 512 characters.
		expression, err := regexp.Compile(d.Trigger)
		if err != nil {
			return nil, &DefinitionError{Field: "trigger", Code: "invalid_regex"}
		}
		return replyRegex{expression}, nil
	default:
		return nil, &DefinitionError{Field: "match_mode", Code: "invalid_match_mode"}
	}
}

func autoReplyAlternatives(d autoReplyDefinition) (Condition, error) {
	values := make([]string, 1, len(d.Alternatives)+1)
	values[0] = d.Trigger
	for _, value := range d.Alternatives {
		if Normalize(value) == "" || utf8.RuneCountInString(value) > 512 {
			return nil, &DefinitionError{Field: "alternatives", Code: "invalid_alternatives"}
		}
		values = append(values, value)
	}
	return OneOf{Values: values}, nil
}

// Matches shares the condition engine's acceptance semantics.
func (r AutoReply) Matches(text string) bool {
	return r.condition != nil && (Rule{Accept: []Condition{r.condition}}).Evaluate(text) == Accepted
}

type replyRegex struct{ expression *regexp.Regexp }

func (r replyRegex) matches(text string) bool { return r.expression.MatchString(Normalize(text)) }

type completeHashtag string

func hashtagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_'
}

func normalizeHashtag(text string) string {
	return strings.ToLower(strings.TrimSpace(dropInvisible(foldFullWidth(text))))
}

func (tag completeHashtag) matches(text string) bool {
	text = normalizeHashtag(text)
	var previous rune
	for index, r := range text {
		left := previous
		previous = r
		if r != '#' || (index > 0 && hashtagRune(left)) {
			continue
		}
		body := text[index+1:]
		end := strings.IndexFunc(body, func(r rune) bool { return !hashtagRune(r) })
		if end < 0 {
			end = len(body)
		}
		if body[:end] == string(tag) {
			return true
		}
	}
	return false
}
