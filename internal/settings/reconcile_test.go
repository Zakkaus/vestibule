package settings

import (
	"path/filepath"
	"testing"
)

const testRuntimeGroup int64 = -1009000000009

func registerRuntimeTestGroup(t *testing.T, store *Store) {
	t.Helper()
	registration := store.Registrations()
	registration.OwnerID = 42
	registration.RegisteredGroups = []RegisteredGroup{{
		ID: testRuntimeGroup, RegisteredBy: 42, Title: "Runtime",
	}}
	_, err := store.CommitRegistrations(registration.Revision, registration)
	requireNoError(t, err)
}

func disableTestGroup(t *testing.T, store *Store, groupID int64) {
	t.Helper()
	group := requireSettingsView(t, store, groupID)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	_, err := store.Update(groupID, group.Revision(), overrides)
	requireNoError(t, err)
}

func baselineWithoutGroup(t *testing.T, groupID int64) SettingsBaseline {
	t.Helper()
	baseline := testSettingsBaseline()
	groups := make([]GroupBaseline, 0, len(baseline.Groups)-1)
	found := false
	for _, group := range baseline.Groups {
		if group.ID == groupID {
			found = true
			continue
		}
		groups = append(groups, group)
	}
	if !found {
		t.Fatalf("baseline does not contain group %d", groupID)
	}
	baseline.Groups = groups
	return baseline
}

func TestConfiguredGroupPromotionKeepsRuntimeDecisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	first, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	registerRuntimeTestGroup(t, first)
	disableTestGroup(t, first, testRuntimeGroup)

	baseline := testSettingsBaseline()
	promoted := cloneGroupBaseline(baseline.Factory)
	promoted.ID = testRuntimeGroup
	baseline.Groups = append(baseline.Groups, promoted)
	second, err := NewStore(path, baseline, nil)
	requireNoError(t, err)

	group := requireSettingsView(t, second, testRuntimeGroup)
	requireEqual(t, group.Enabled(), Setting[bool]{Value: false, Source: SourceChatOverride},
		"promoted group's runtime override")
	registration := second.Registrations()
	requireEqual(t, registration.OwnerID, int64(42), "owner after configured-group promotion")
	requireEqual(t, len(registration.RegisteredGroups), 0, "runtime registrations after promotion")
	if second.Persistence().Writable {
		t.Error("promotion reconciliation remained writable without operator acknowledgement")
	}
}

func TestConfiguredGroupRetirementPreservesOverrideForReaddition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	first, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	registerRuntimeTestGroup(t, first)
	disableTestGroup(t, first, testGroupB)

	retired, err := NewStore(path, baselineWithoutGroup(t, testGroupB), nil)
	requireNoError(t, err)
	if _, ok := retired.Settings(testGroupB); ok {
		t.Fatal("retired configured group remained active without runtime registration")
	}
	if !retired.Persistence().Writable {
		t.Error("retiring a configured group made unrelated settings read-only")
	}
	registration := retired.Registrations()
	requireEqual(t, registration.OwnerID, int64(42), "owner after configured-group retirement")
	if len(registration.RegisteredGroups) != 1 || registration.RegisteredGroups[0].ID != testRuntimeGroup {
		t.Errorf("runtime registrations after retirement = %#v, want group %d", registration.RegisteredGroups, testRuntimeGroup)
	}

	restored, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	group := requireSettingsView(t, restored, testGroupB)
	requireEqual(t, group.Enabled(), Setting[bool]{Value: false, Source: SourceChatOverride},
		"re-added group's retained runtime override")
}
