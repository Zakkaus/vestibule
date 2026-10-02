package rules

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func assertSignalScore(t *testing.T, input SignalInput, weights SignalWeights, total int, hits []SignalHit) {
	t.Helper()
	got := ScoreSignals(input, weights)
	if got.Total != total || !reflect.DeepEqual(got.Hits, hits) {
		t.Fatalf("ScoreSignals(%+v) = %+v, want total %d and hits %+v", input, got, total, hits)
	}
}

func TestSignalShapes(t *testing.T) {
	for _, test := range []struct {
		name  string
		input SignalInput
		total int
		hits  []SignalHit
	}{
		{name: "empty"},
		{name: "hidden inside word", input: SignalInput{Text: "sa\u200bmple"}, total: 2,
			hits: []SignalHit{{SignalHiddenCharacters, 1, 2}}},
		{name: "private invite", input: SignalInput{Entities: []SignalEntity{{EntityURL, "https://t.me/+sample"}}}, total: 4,
			hits: []SignalHit{{SignalPrivateInviteLinks, 1, 3}, {SignalLinks, 1, 1}}},
		{name: "mentions", input: SignalInput{Entities: []SignalEntity{{Kind: EntityMention}, {Kind: EntityTextMention}}}, total: 2,
			hits: []SignalHit{{SignalMentions, 2, 2}}},
		{name: "ordinary links", input: SignalInput{Joined: 48 * time.Hour, KnownJoin: true,
			Entities: []SignalEntity{{EntityURL, "https://example.org"}, {EntityTextLink, "https://example.net"}}}, total: 2,
			hits: []SignalHit{{SignalLinks, 2, 2}}},
		{name: "new member link", input: SignalInput{Joined: time.Hour, KnownJoin: true,
			Entities: []SignalEntity{{EntityTextLink, "https://example.org"}}}, total: 5,
			hits: []SignalHit{{SignalLinks, 1, 1}, {SignalNewMemberWithLink, 1, 4}}},
		{name: "no full text scan", input: SignalInput{Text: "https://t.me/+sample @sample_one", KnownJoin: true}},
		{name: "mention URL is not a link", input: SignalInput{Entities: []SignalEntity{{EntityMention, "https://t.me/+sample"}}}, total: 1,
			hits: []SignalHit{{SignalMentions, 1, 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertSignalScore(t, test.input, DefaultSignalWeights, test.total, test.hits)
		})
	}
}

func TestEmojiAndHiddenCharacters(t *testing.T) {
	for _, test := range []struct {
		name  string
		text  string
		count int
	}{
		{name: "family", text: "👨\u200d👩\u200d👧"},
		{name: "heart", text: "❤\ufe0f"},
		{name: "text presentation", text: "\u2764\ufe0e"},
		{name: "keycaps", text: "1\ufe0f⃣ #\ufe0f⃣ *\ufe0f⃣"},
		{name: "England", text: "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f"},
		{name: "qualified joiners", text: "🏳\ufe0f\u200d🌈 👩🏽\u200d💻 ❤\ufe0f\u200d🔥"},
		{name: "tagged flag before joiner", text: "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f\u200d\U0001f480"},
		{name: "ordinary text and emoji", text: "Hello 👨\u200d👩\u200d👧 ❤\ufe0f 1\ufe0f⃣"},
		{name: "pictographic outside common blocks", text: "©\ufe0f ↔\ufe0f Ⓜ\ufe0f ㊗\ufe0f"},
		{name: "joiner in word", text: "sa\u200dmple", count: 1},
		{name: "trailing joiner", text: "👨\u200d", count: 1},
		{name: "leading joiner", text: "\u200d👨", count: 1},
		{name: "non pictographic joiner neighbor", text: "👨\u200da", count: 1},
		{name: "hidden near emoji", text: "👨\u200b\u200d👩", count: 2},
		{name: "selector in word", text: "sa\ufe0fmple", count: 1},
		{name: "standalone selectors", text: "\ufe0e\ufe0f", count: 2},
		{name: "repeated selector", text: "❤\ufe0f\ufe0f", count: 1},
		{name: "regional indicators not pictographic", text: "🇬\ufe0f", count: 1},
		{name: "skin tone not pictographic", text: "🏽\ufe0f", count: 1},
		{name: "pictographic range gaps", text: "\u25ff\ufe0f \U0001f394\ufe0f \U0001f4fe\ufe0f", count: 3},
		{name: "supplemental selectors", text: "\U000e0100\U000e01ef", count: 2},
		{name: "all drop ranges", text: "\u200b\u200c\u200e\u200f\u2060\u2064\ufeff\ufe00\ufe0d", count: 9},
		{name: "normalization not shape", text: "Ｆｕｌｌ　ｗｉｄｔｈ 验*证-码"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var hits []SignalHit
			if test.count > 0 {
				hits = []SignalHit{{SignalHiddenCharacters, test.count, 2 * test.count}}
			}
			assertSignalScore(t, SignalInput{Text: test.text}, DefaultSignalWeights, 2*test.count, hits)
		})
	}
}

func TestPrivateInviteEntityTargets(t *testing.T) {
	for _, test := range []struct {
		target  string
		private bool
	}{
		{"https://t.me/+sample", true},
		{"https://t.me/+15551234567", false},
		{"https://telegram.dog/+sampleA", true},
		{"tg://join?invite=sampleA", true},
		{"tg:join?invite=sampleA", true},
		{"tg://resolve?domain=sample", false},
		{"tg://join?other=1", false},
		{"http://telegram.me/joinchat/sample?x=1", true},
		{"t.me/+sample", true},
		{"//telegram.me/joinchat/sample", true},
		{"https://T.ME:443/+sample#fragment", true},
		{"https://user@t.me/joinchat/sample", true},
		{"https://t.me/public_channel", false},
		{"https://t.me/joinchat", false},
		{"https://t.me/JoinChat/sample", false},
		{"https://t.me/other/+sample", false},
		{"https://t.me.evil.example/+sample", false},
		{"https://sub.t.me/+sample", false},
		{"https://t.me@evil.example/+sample", false},
		{"https://example.org/?next=https://t.me/+sample", false},
		{"https://t.me?next=/+sample", false},
		{"https://t.me#/+sample", false},
		{"https://t.me", false},
		{"", false},
	} {
		for _, kind := range []SignalEntityKind{EntityURL, EntityTextLink} {
			t.Run(test.target+"/"+string(rune('0'+kind)), func(t *testing.T) {
				total := 1
				hits := []SignalHit{{SignalLinks, 1, 1}}
				if test.private {
					total = 4
					hits = []SignalHit{{SignalPrivateInviteLinks, 1, 3}, {SignalLinks, 1, 1}}
				}
				assertSignalScore(t, SignalInput{Entities: []SignalEntity{{kind, test.target}}}, DefaultSignalWeights, total, hits)
			})
		}
	}
}

func TestNewMemberLinkBoundary(t *testing.T) {
	for _, test := range []struct {
		name   string
		joined time.Duration
		known  bool
		links  int
		new    bool
	}{
		{"at join", 0, true, 1, true},
		{"just under day", 24*time.Hour - time.Nanosecond, true, 1, true},
		{"exact day", 24 * time.Hour, true, 1, false},
		{"over day", 48 * time.Hour, true, 1, false},
		{"unknown zero", 0, false, 1, false},
		{"unknown recent", time.Hour, false, 2, false},
		{"unknown negative", -time.Hour, false, 1, false},
		{"new without link", time.Hour, true, 0, false},
		{"multiple links only one join hit", time.Hour, true, 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := SignalInput{Joined: test.joined, KnownJoin: test.known}
			for range test.links {
				input.Entities = append(input.Entities, SignalEntity{EntityURL, "https://example.org"})
			}
			total := test.links
			var hits []SignalHit
			if test.links > 0 {
				hits = append(hits, SignalHit{SignalLinks, test.links, test.links})
			}
			if test.new {
				total += 4
				hits = append(hits, SignalHit{SignalNewMemberWithLink, 1, 4})
			}
			assertSignalScore(t, input, DefaultSignalWeights, total, hits)
		})
	}
}

func TestIndependentSignalWeights(t *testing.T) {
	input := SignalInput{Text: "sa\u200bmple", KnownJoin: true, Entities: []SignalEntity{
		{EntityURL, "https://t.me/+sampleA"}, {EntityTextLink, "https://telegram.me/joinchat/sampleB"}, {Kind: EntityMention},
	}}
	for _, test := range []struct {
		name    string
		weights SignalWeights
		total   int
		points  [5]int
	}{
		{"hidden alone", SignalWeights{HiddenCharacters: 7}, 7, [5]int{7}},
		{"invites alone", SignalWeights{PrivateInviteLinks: 7}, 14, [5]int{0, 14}},
		{"mentions alone", SignalWeights{Mentions: 7}, 7, [5]int{0, 0, 7}},
		{"links alone", SignalWeights{Links: 7}, 14, [5]int{0, 0, 0, 14}},
		{"join alone", SignalWeights{NewMemberWithLink: 7}, 7, [5]int{0, 0, 0, 0, 7}},
		{"zero weights", SignalWeights{}, 0, [5]int{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			hits := []SignalHit{
				{SignalHiddenCharacters, 1, test.points[0]}, {SignalPrivateInviteLinks, 2, test.points[1]},
				{SignalMentions, 1, test.points[2]}, {SignalLinks, 2, test.points[3]}, {SignalNewMemberWithLink, 1, test.points[4]},
			}
			assertSignalScore(t, input, test.weights, test.total, hits)
		})
	}
}

func TestStructuralSignalSamples(t *testing.T) {
	invites := []SignalEntity{{EntityURL, "https://t.me/+sampleA"}, {EntityTextLink, "https://telegram.me/joinchat/sampleB"}}
	for _, test := range []struct {
		file     string
		entities []SignalEntity
		known    bool
		total    int
		hits     []SignalHit
	}{
		{"emoji-sequences.txt", nil, false, 0, nil},
		{"private-invites.txt", invites, false, 8, []SignalHit{{SignalPrivateInviteLinks, 2, 6}, {SignalLinks, 2, 2}}},
		{"mentions.txt", []SignalEntity{{Kind: EntityMention}, {Kind: EntityMention}, {Kind: EntityTextMention}}, false, 3,
			[]SignalHit{{SignalMentions, 3, 3}}},
		{"combined-signals.txt", append(append([]SignalEntity(nil), invites...), SignalEntity{Kind: EntityMention}, SignalEntity{Kind: EntityTextMention}), true, 18,
			[]SignalHit{{SignalHiddenCharacters, 2, 4}, {SignalPrivateInviteLinks, 2, 6}, {SignalMentions, 2, 2}, {SignalLinks, 2, 2}, {SignalNewMemberWithLink, 1, 4}}},
	} {
		t.Run(test.file, func(t *testing.T) {
			text, err := os.ReadFile("../../testdata/spam/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			input := SignalInput{Text: string(text), Entities: test.entities, Joined: time.Hour, KnownJoin: test.known}
			assertSignalScore(t, input, DefaultSignalWeights, test.total, test.hits)
		})
	}
	ordinary := ScoreSignals(SignalInput{KnownJoin: true, Joined: 48 * time.Hour,
		Entities: []SignalEntity{{EntityURL, "https://example.org"}}}, DefaultSignalWeights)
	if ordinary.Total >= 10 {
		t.Fatalf("ordinary link score %d reaches the illustrative threshold of 10", ordinary.Total)
	}
	text, err := os.ReadFile("../../testdata/spam/combined-signals.txt")
	if err != nil {
		t.Fatal(err)
	}
	spam := ScoreSignals(SignalInput{Text: string(text), Entities: invites, KnownJoin: true}, DefaultSignalWeights)
	if spam.Total <= 10 {
		t.Fatalf("new member invite sample score %d does not exceed the illustrative threshold of 10", spam.Total)
	}
}
