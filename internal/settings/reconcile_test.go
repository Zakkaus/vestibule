package settings

import (
	"path/filepath"
	"testing"
)

const testRuntimeGroup int64 = -1009000000009

// Promoting a runtime-registered group into the user file, or retiring a configured group, is
// ordinary maintenance. Neither may silently discard stored overrides or registration metadata.
func TestConfigDriftKeepsRuntimeDecisions(t *testing.T) {
	cases := []struct {
		name         string
		mutate       func(base *SettingsBaseline)
		wantWritable bool
	}{
		{
			name: "registered group promoted into config",
			mutate: func(base *SettingsBaseline) {
				promoted := base.Factory
				promoted.ID = testRuntimeGroup
				base.Groups = append(base.Groups, promoted)
			},
		},
		{
			name: "configured group retired",
			mutate: func(base *SettingsBaseline) {
				base.Groups = base.Groups[1:]
			},
			wantWritable: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			first, err := NewStore(path, testSettingsBaseline(), nil)
			if err != nil {
				t.Fatal(err)
			}
			registration := first.Registrations()
			registration.OwnerID = 42
			registration.RegisteredGroups = []RegisteredGroup{{ID: testRuntimeGroup, RegisteredBy: 42, Title: "Runtime"}}
			if _, err := first.CommitRegistrations(registration.Revision, registration); err != nil {
				t.Fatal(err)
			}
			target := testGroupB
			group, _ := first.Settings(target)
			overrides := group.Overrides()
			disabled := false
			overrides.Enabled = &disabled
			if _, err := first.Update(target, group.Revision(), overrides); err != nil {
				t.Fatal(err)
			}

			base := testSettingsBaseline()
			tc.mutate(&base)
			second, err := NewStore(path, base, nil)
			if err != nil {
				t.Fatal(err)
			}
			survived, ok := second.Settings(target)
			if !ok {
				t.Fatal("the configured group must survive reconciliation")
			}
			if survived.Enabled().Value || survived.Enabled().Source != SourceChatOverride {
				t.Errorf("Enabled = %#v, want the administrator's runtime override to survive", survived.Enabled())
			}
			if got := second.Registrations().OwnerID; got != 42 {
				t.Errorf("OwnerID = %d, want 42: reconciliation must not forget who owns the bot", got)
			}
			if !second.IsGroup(testRuntimeGroup) {
				t.Error("the runtime group is still guarded, whether it is registered or configured")
			}
			if got := second.Persistence().Writable; got != tc.wantWritable {
				t.Errorf("writable = %v, want %v", got, tc.wantWritable)
			}
		})
	}
}
