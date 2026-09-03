package settings

import "testing"

type scriptedSettingsRepository struct {
	actual  uint64
	written bool
}

func (r scriptedSettingsRepository) LoadSettings() ([]Record, error) {
	return nil, nil
}

func (r scriptedSettingsRepository) SeedSettings([]Record) error {
	return nil
}

func (r scriptedSettingsRepository) CompareAndSwapSettings(
	int64,
	uint64,
	GroupOverrides,
) (uint64, bool, error) {
	return r.actual, r.written, nil
}

func TestSettingsRepositoryConflictDoesNotReportLostWriteAsSaved(t *testing.T) {
	refused, err := NewStore("", testSettingsBaseline(), scriptedSettingsRepository{actual: 7})
	requireNoError(t, err)
	before := requireSettingsView(t, refused, testGroupA)
	next := before.Overrides()
	next.Enabled = ptr(false)
	_, err = refused.Update(before.ID(), before.Revision(), next)
	requireErrorIs(t, err, ErrSettingsConflict, "database refusal must not be reported as a saved setting")
	after := requireSettingsView(t, refused, testGroupA)
	requireEqual(t, after.Enabled(), before.Enabled(), "database refusal must keep the visible setting")
	requireEqual(t, after.Revision(), before.Revision(), "database refusal must keep the visible revision")

	accepted, err := NewStore("", testSettingsBaseline(), scriptedSettingsRepository{actual: 1, written: true})
	requireNoError(t, err)
	before = requireSettingsView(t, accepted, testGroupA)
	next = before.Overrides()
	next.Enabled = ptr(false)
	result, err := accepted.Update(before.ID(), before.Revision(), next)
	requireNoError(t, err)
	requireEqual(t, result.Revision, uint64(1), "valid database write revision")
}

func TestSettingsRepositoryWriteReportsDatabaseRevision(t *testing.T) {
	const databaseRevision = uint64(7)
	settings, err := NewStore("", testSettingsBaseline(), scriptedSettingsRepository{
		actual: databaseRevision, written: true,
	})
	requireNoError(t, err)
	before := requireSettingsView(t, settings, testGroupA)
	next := before.Overrides()
	next.Enabled = ptr(false)

	result, err := settings.Update(before.ID(), before.Revision(), next)
	requireNoError(t, err)
	requireEqual(t, result.Revision, databaseRevision,
		"successful repository write must report the database revision")
}

func updateWithoutPanic(
	t *testing.T,
	settings *Store,
	groupID int64,
	revision uint64,
	next GroupOverrides,
) (result CommitResult, err error) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("updating an unknown group panicked instead of refusing it: %v", recovered)
		}
	}()
	return settings.Update(groupID, revision, next)
}

func TestSettingsUpdateRefusesUnknownGroupsWithoutPanicking(t *testing.T) {
	settings, err := NewStore("", testSettingsBaseline(), nil)
	requireNoError(t, err)

	_, err = updateWithoutPanic(t, settings, -1009000000501, 0, GroupOverrides{Enabled: ptr(false)})
	requireErrorIs(t, err, ErrUnknownGroup, "unknown group update")

	known := requireSettingsView(t, settings, testGroupA)
	next := known.Overrides()
	next.Enabled = ptr(false)
	_, err = settings.Update(known.ID(), known.Revision(), next)
	requireNoError(t, err)
}
