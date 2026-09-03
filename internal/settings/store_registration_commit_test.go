package settings

import (
	"path/filepath"
	"testing"
)

func TestSettingsRegistrationCommitRefusesStaleRevisions(t *testing.T) {
	settings, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), testSettingsBaseline(), nil)
	requireNoError(t, err)

	initial := settings.Registrations()
	valid := cloneRegistrationState(initial)
	valid.OwnerID = 42
	committed, err := settings.CommitRegistrations(initial.Revision, valid)
	requireNoError(t, err)
	requireEqual(t, committed.Revision, uint64(1), "valid registration commit revision")

	staleSnapshot := cloneRegistrationState(initial)
	staleSnapshot.OwnerID = 43
	_, err = settings.CommitRegistrations(initial.Revision, staleSnapshot)
	requireErrorIs(t, err, ErrSettingsConflict, "stale registration snapshot must not overwrite the owner")

	current := settings.Registrations()
	staleRequest := cloneRegistrationState(current)
	staleRequest.Revision = current.Revision + 1
	staleRequest.OwnerID = 43
	_, err = settings.CommitRegistrations(current.Revision, staleRequest)
	requireErrorIs(t, err, ErrSettingsConflict, "caller revision mismatch must not overwrite the owner")
}

func TestSettingsUnchangedRegistrationCommitDoesNotAdvanceRevision(t *testing.T) {
	settings, _, _ := newRegistrationFixture(t)
	current := settings.Registrations()

	result, err := settings.CommitRegistrations(current.Revision, cloneRegistrationState(current))
	requireNoError(t, err)
	requireEqual(t, result.Revision, current.Revision, "unchanged registration commit revision")
	requireDeepEqual(t, settings.Registrations(), current, "unchanged registration commit state")
}

func TestSettingsDeregisteredGroupsDiscardTheirOverrides(t *testing.T) {
	settings, _, _ := newRegistrationFixture(t)
	updateRegistrationRuntimeGroup(t, settings)

	withoutRuntime := settings.Registrations()
	withoutRuntime.RegisteredGroups = nil
	_, err := settings.CommitRegistrations(withoutRuntime.Revision, withoutRuntime)
	requireNoError(t, err)
	if _, ok := settings.Settings(-1009000000003); ok {
		t.Fatal("deregistered group stayed effective")
	}

	reRegistered := settings.Registrations()
	reRegistered.RegisteredGroups = []RegisteredGroup{{
		ID: -1009000000003, RegisteredBy: 42, Title: "Replacement runtime group",
	}}
	_, err = settings.CommitRegistrations(reRegistered.Revision, reRegistered)
	requireNoError(t, err)
	group := requireSettingsView(t, settings, -1009000000003)
	requireDeepEqual(t, group.Overrides(), GroupOverrides{},
		"re-registered group retained the previous administrators' settings")
	requireEqual(t, group.TimeoutSeconds(), Setting[int]{Value: 240, Source: SourceUserFile},
		"re-registered group retained the previous timeout override")
}
