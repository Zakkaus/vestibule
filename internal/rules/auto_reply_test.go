package rules

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAutoReplyMatchingModes(t *testing.T) {
	for _, test := range []struct {
		mode, trigger string
		alternatives  []string
		yes, no       string
	}{
		{mode: "", trigger: "matrix", yes: "Is MATRIX available?", no: "bridge"},
		{mode: "equals", trigger: "rules", yes: " ＲＵＬＥＳ ", no: "show rules"},
		{mode: "one_of", trigger: "bridge", alternatives: []string{"matrix"}, yes: "MATRIX", no: "matrix bridge"},
		{mode: "hashtag", trigger: "matrix", yes: "See #ＭＡＴＲＩＸ!", no: "#matrixbridge"},
		{mode: "hashtag", trigger: "matrix", yes: "#matrix", no: "#matrix_bridge"},
		{mode: "hashtag", trigger: "中文", yes: "#中文 怎么加入", no: "#中文标签"},
		{mode: "hashtag", trigger: "中文_标签", yes: "#中文_标签", no: "#中文标签"},
		{mode: "hashtag", trigger: "中文标签", yes: "#中文标签", no: "#中文_标签"},
		{mode: "regex", trigger: `issue #[0-9]+$`, yes: "ISSUE #123", no: "issue #abc"},
	} {
		t.Run(test.mode+test.no, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"match_mode": test.mode, "trigger": test.trigger, "alternatives": test.alternatives, "reply": "<literal reply>"})
			rule, err := DecodeAutoReply(data)
			if err != nil {
				t.Fatal(err)
			}
			if !rule.Matches(test.yes) || rule.Matches(test.no) {
				t.Fatalf("yes=%q no=%q: match boundary broken", test.yes, test.no)
			}
			if rule.Reply != "<literal reply>" || rule.Cooldown != 10*time.Minute {
				t.Fatalf("reply=%q cooldown=%s", rule.Reply, rule.Cooldown)
			}
		})
	}
}

func TestAutoReplyHashtagLeftBoundary(t *testing.T) {
	for _, test := range []struct {
		trigger, text string
		want          bool
	}{
		{"matrix", "foo#matrix", false},
		{"rules", "https://example.org/docs#rules", false},
		{"matrix", "#matrix", true},
		{"matrix", "hi #matrix!", true},
		{"中文", "（#中文）", true},
		{"中文", "#中文 怎么加入", true},
		{"tag", "a_#tag", false},
		{"matrix", "foo#matrix #matrix", true},
		{"matrix", "#other foo#matrix", false},
		{"matrix", "#other#matrix", false},
		{"tag", "7#tag", false},
		{"tag", "字#tag", false},
		{"tag", "a\u0301#tag", false},
	} {
		t.Run(test.text, func(t *testing.T) {
			data, err := json.Marshal(map[string]string{"match_mode": "hashtag", "trigger": test.trigger, "reply": "ok"})
			if err != nil {
				t.Fatal(err)
			}
			rule, err := DecodeAutoReply(data)
			if err != nil {
				t.Fatal(err)
			}
			if got := rule.Matches(test.text); got != test.want {
				t.Fatalf("trigger %q text %q: got %v, want %v", test.trigger, test.text, got, test.want)
			}
		})
	}
}

func TestAutoReplyInvalidDefinitions(t *testing.T) {
	for _, test := range []struct{ data, field string }{
		{`{"reply":"ok"}`, "trigger"},
		{`{"trigger":"matrix"}`, "reply"},
		{`{"trigger":"matrix","reply":"ok","match_mode":"script"}`, "match_mode"},
		{`{"trigger":"matrix","reply":"ok","cooldown_seconds":0}`, "cooldown_seconds"},
		{`{"trigger":"matrix","reply":"ok","cooldown_seconds":9223372036854775807}`, "cooldown_seconds"},
		{`{"trigger":"[","reply":"ok","match_mode":"regex"}`, "trigger"},
		{`{"trigger":"two words","reply":"ok","match_mode":"hashtag"}`, "trigger"},
		{`{"trigger":"中文 标签","reply":"ok","match_mode":"hashtag"}`, "trigger"},
		{`{"trigger":"matrix","reply":"ok","alternatives":["bridge"]}`, "alternatives"},
		{`{"trigger":"matrix","reply":"ok","match_mode":"one_of","alternatives":[""]}`, "alternatives"},
		{`{"trigger":"matrix","reply":"ok","execute":"ban"}`, "definition"},
		{`["matrix","ok"]`, "definition"},
		{`{`, "definition"},
	} {
		_, err := DecodeAutoReply(json.RawMessage(test.data))
		field, ok := err.(*DefinitionError)
		if !ok || field.Field != test.field {
			t.Fatalf("DecodeAutoReply(%s)=%v, want field %s", test.data, err, test.field)
		}
	}
	data, _ := json.Marshal(map[string]any{"trigger": strings.Repeat("x", 513), "reply": "ok", "match_mode": "regex"})
	if _, err := DecodeAutoReply(data); err == nil {
		t.Fatal("oversized regex accepted")
	}
}
