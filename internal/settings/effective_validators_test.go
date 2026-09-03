package settings

import (
	"path/filepath"
	"strings"
	"testing"
)

// Thirteen validators run when a group's settings are rebuilt, each refusing an override that
// would leave the group in a state nobody can work with. Nine of them fail two or three tests
// when made to accept everything. These four failed none.
func TestEffectiveValidatorsRefuseUnusableOverrides(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override func(*GroupOverrides)
		want     string
	}{
		{
			// A required channel the applicant cannot reach: a numeric id, no invite URL, and
			// a display that is not an @handle. Every applicant is told to join something they
			// cannot find, and every one of them times out and is declined.
			name: "a required channel with no way to reach it",
			override: func(o *GroupOverrides) {
				id := int64(-1009000001100)
				display := "Some Channel"
				invite := ""
				// The invite URL has to be cleared too: the baseline carries one, and with it
				// the channel is reachable and accepting the override is correct.
				o.RequiredChannelID = &id
				o.ChannelDisplay = &display
				o.ChannelInviteURL = &invite
			},
			want: "reachable display or invite URL",
		},
		{
			name: "a lookup cache that never holds anything",
			override: func(o *GroupOverrides) {
				ttl := 0
				o.LookupTTLSeconds = &ttl
			},
			want: "lookup_ttl_seconds must be positive",
		},
		{
			name: "a chat allowed by an id that is not a chat",
			override: func(o *GroupOverrides) {
				o.ChannelWhitelist = &[]int64{0}
			},
			want: "channel_whitelist",
		},
		{
			name: "fallback questions supplied while the built-in set is in use",
			override: func(o *GroupOverrides) {
				builtin := true
				o.FallbackBuiltin = &builtin
				o.FallbackQuestions = &[]ShortQuestion{{Q: "anything", Answers: []string{"yes"}}}
			},
			want: "fallback_questions cannot be overridden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"),
				testSettingsBaseline(), nil)
			if err != nil {
				t.Fatal(err)
			}
			group, _ := store.Settings(testGroupA)
			next := group.Overrides()
			tc.override(&next)

			_, err = store.Update(testGroupA, group.Revision(), next)
			if err == nil {
				t.Fatalf("the store accepted %s; a group left in that state cannot be worked "+
					"with and nothing else refuses it", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("refusal = %q, want it to name %q so an operator can see which value "+
					"was refused", err, tc.want)
			}
		})
	}
}
