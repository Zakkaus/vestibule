package settings

import (
	"slices"
	"testing"
)

func TestTrustedGroupPrecedenceDistinguishesOmittedAndEmpty(t *testing.T) {
	const groupID = int64(-1009000000401)
	global := []int64{-1009000000402}
	tests := []struct {
		name       string
		groupValue []int64
		want       []int64
	}{
		{name: "omitted inherits top-level list", want: global},
		{name: "explicit empty disables top-level list", groupValue: []int64{}, want: []int64{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{
				TrustedMemberGroupIDs: global,
				Groups: []GroupConfig{{
					ID:                    groupID,
					TrustedMemberGroupIDs: test.groupValue,
				}},
			}
			if got := cfg.TrustedGroups(groupID); !slices.Equal(got, test.want) {
				t.Errorf("TrustedGroups(%d) = %v, want %v: an empty group list must disable rather than inherit", groupID, got, test.want)
			}
		})
	}
}

func TestPerGroupQuestionPrecedenceDistinguishesOmittedAndEmpty(t *testing.T) {
	const groupID = int64(-1009000000403)
	global := []Question{{Q: "Global question", Options: []string{"yes", "no"}, Answer: 0}}
	tests := []struct {
		name       string
		groupValue []Question
		wantCount  int
	}{
		{name: "omitted inherits top-level bank", wantCount: 1},
		{name: "explicit empty disables top-level bank", groupValue: []Question{}, wantCount: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{
				Questions: global,
				Groups:    []GroupConfig{{ID: groupID, Questions: test.groupValue}},
			}
			baseline := settingsBaselineFromConfig(&cfg, configPresence{"questions": true})
			store, err := NewStore("", baseline, nil)
			if err != nil {
				t.Fatal(err)
			}
			group, ok := store.Settings(groupID)
			if !ok {
				t.Fatalf("configured group %d is missing", groupID)
			}
			questions := group.Questions()
			if got := len(questions.Value); got != test.wantCount {
				t.Errorf("question count = %d, want %d: an empty group bank must disable rather than inherit", got, test.wantCount)
			}
			if questions.Source != SourceUserFile {
				t.Errorf("question source = %v, want %v: file-managed precedence was lost", questions.Source, SourceUserFile)
			}
		})
	}
}
