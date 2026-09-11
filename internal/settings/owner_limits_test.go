package settings

import (
	"os"
	"path/filepath"
	"testing"
)

const ownerLimitsRuntimeGroup int64 = -1009000000003

func ownerLimitsTestBaseline() SettingsBaseline {
	baseline := testSettingsBaseline()
	baseline.Factory.BanSeconds = BaselineValue[int]{Value: 30, Source: SourceFactory}
	baseline.Factory.FallbackQuestions = BaselineValue[[]ShortQuestion]{Value: []ShortQuestion{{Q: "Factory fallback?", Answers: []string{"yes"}}}, Source: SourceFactory}
	baseline.Factory.TimeoutSeconds = BaselineValue[int]{Value: 240, Source: SourceUserFile}
	baseline.Groups[0].BanSeconds = BaselineValue[int]{Value: 30, Source: SourceUserFile}
	baseline.Groups[0].FallbackBuiltin = BaselineValue[bool]{Value: true, Source: SourceFactory}
	baseline.Groups[0].TimeoutSeconds = BaselineValue[int]{Value: 240, Source: SourceUserFile}
	baseline.Groups[1].BanSeconds = BaselineValue[int]{Value: 30, Source: SourceUserFile}
	baseline.Groups[1].FallbackBuiltin = BaselineValue[bool]{Value: false, Source: SourceUserFile}
	baseline.Groups[1].FallbackQuestions = BaselineValue[[]ShortQuestion]{Value: []ShortQuestion{
		{Q: "File fallback one?", Answers: []string{"yes"}},
		{Q: "File fallback two?", Answers: []string{"yes"}},
	}, Source: SourceUserFile}
	baseline.Groups[1].TimeoutSeconds = BaselineValue[int]{Value: 120, Source: SourceUserFile}
	return baseline
}

func ownerLimitsDurableStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	return store
}

func ownerLimitsRegisterRuntime(t *testing.T, store *Store) GroupView {
	t.Helper()
	registration := store.Registrations()
	registration.RegisteredGroups = []RegisteredGroup{{ID: ownerLimitsRuntimeGroup, RegisteredBy: 42}}
	_, err := store.CommitRegistrations(registration.Revision, registration)
	requireNoError(t, err)
	return requireSettingsView(t, store, ownerLimitsRuntimeGroup)
}

func ownerLimitsRequireViolation(t *testing.T, state OwnerLimitsState, chatID int64, field string, value, limit int64) {
	t.Helper()
	for _, violation := range state.Violations {
		if violation.ChatID == chatID && violation.Field == field && violation.Value == value && violation.Limit == limit {
			return
		}
	}
	t.Fatalf("missing violation chat=%d field=%s value=%d limit=%d in %+v", chatID, field, value, limit, state.Violations)
}

func TestOwnerLimitsValidateAllFieldsAtUpperBound(t *testing.T) {
	maxima := map[string]int64{
		"timeout_seconds": 1800, "ban_seconds": 31622400, "mute_seconds": 31622400,
		"lookup_ttl_seconds": 86400, "verify_retry_seconds": 31622400, "verify_max_fails": 2147483647,
		"warn_limit": 2147483647, "private_query_per_min": 2147483647, "questions": 2147483647,
		"fallback_questions": 2147483647, "channel_whitelist": 2147483647,
		"trusted_member_group_ids": 2147483647, "known_chat_ids": 2147483647,
	}
	store := ownerLimitsDurableStore(t)
	state := store.OwnerLimits()
	for field, maximum := range maxima {
		value := maximum
		updated, err := store.UpdateOwnerLimits(state.Revision, LimitChanges{field: &value})
		requireNoError(t, err)
		got, _ := updated.Limits.Value(field)
		requireEqual(t, got, maximum, field+" at upper bound")

		over := maximum + 1
		_, err = store.UpdateOwnerLimits(updated.Revision, LimitChanges{field: &over})
		requireErrorIs(t, err, ErrOwnerLimitsInvalid, field+" above upper bound")
		requireDeepEqual(t, store.OwnerLimits(), updated, field+" invalid update state")
		state = updated
	}
}

func TestOwnerLimitsDetectEachEffectiveFieldAtCapBoundary(t *testing.T) {
	type limitCase struct {
		field   string
		groupID int64
		equal   int64
		prepare func(*GroupOverrides)
	}
	cases := []limitCase{
		{"timeout_seconds", testGroupA, 240, nil},
		{"ban_seconds", testGroupA, 60, func(next *GroupOverrides) { next.BanSeconds = ptr(60) }},
		{"mute_seconds", testGroupA, 3600, nil},
		{"lookup_ttl_seconds", testGroupA, 180, nil},
		{"verify_retry_seconds", testGroupA, 180, nil},
		{"verify_max_fails", testGroupA, 3, nil},
		{"warn_limit", testGroupA, 3, nil},
		{"private_query_per_min", testGroupA, 3, nil},
		{"questions", testGroupA, 2, func(next *GroupOverrides) {
			next.Questions = &[]Question{{Q: "one", Options: []string{"a", "b"}, Answer: 0}, {Q: "two", Options: []string{"a", "b"}, Answer: 0}}
		}},
		{"fallback_questions", testGroupB, 2, nil},
		{"channel_whitelist", testGroupA, 2, func(next *GroupOverrides) { next.ChannelWhitelist = &[]int64{-1, -2} }},
		{"trusted_member_group_ids", testGroupA, 2, func(next *GroupOverrides) { next.TrustedMemberGroupIDs = &[]int64{-3, -4} }},
		{"known_chat_ids", testGroupA, 2, func(next *GroupOverrides) { next.KnownChatIDs = &[]int64{-5, -6} }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			store := ownerLimitsDurableStore(t)
			group := requireSettingsView(t, store, tc.groupID)
			if tc.prepare != nil {
				next := group.Overrides()
				tc.prepare(&next)
				_, err := store.Update(group.ID(), group.Revision(), next)
				requireNoError(t, err)
			}
			cap := tc.equal
			atLimit, err := store.UpdateOwnerLimits(0, LimitChanges{tc.field: &cap})
			requireNoError(t, err)
			for _, violation := range atLimit.Violations {
				if violation.ChatID == tc.groupID && violation.Field == tc.field {
					t.Fatalf("equal effective value was reported as violation: %+v", violation)
				}
			}
			below := tc.equal - 1
			belowLimit, err := store.UpdateOwnerLimits(atLimit.Revision, LimitChanges{tc.field: &below})
			requireNoError(t, err)
			ownerLimitsRequireViolation(t, belowLimit, tc.groupID, tc.field, tc.equal, below)
		})
	}
}

func TestOwnerLimitsPermanentBanAndDisabledNegativeValues(t *testing.T) {
	baseline := ownerLimitsTestBaseline()
	baseline.Groups[0].BanSeconds = BaselineValue[int]{Value: 0, Source: SourceUserFile}
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	requireNoError(t, err)
	finite := int64(30)
	state, err := store.UpdateOwnerLimits(0, LimitChanges{"ban_seconds": &finite})
	requireNoError(t, err)
	ownerLimitsRequireViolation(t, state, testGroupA, "ban_seconds", 0, finite)

	for _, field := range []string{"verify_retry_seconds", "verify_max_fails"} {
		t.Run(field, func(t *testing.T) {
			store := ownerLimitsDurableStore(t)
			group := requireSettingsView(t, store, testGroupA)
			next := group.Overrides()
			negative := -1
			if field == "verify_retry_seconds" {
				next.VerifyRetrySeconds = &negative
			} else {
				next.VerifyMaxFails = &negative
			}
			_, err := store.Update(group.ID(), group.Revision(), next)
			requireNoError(t, err)
			cap := int64(1)
			state, err := store.UpdateOwnerLimits(0, LimitChanges{field: &cap})
			requireNoError(t, err)
			for _, violation := range state.Violations {
				if violation.ChatID == testGroupA && violation.Field == field {
					t.Fatalf("negative disabled value was limited: %+v", violation)
				}
			}
		})
	}
}

func TestOwnerLimitsSourcesLoweredCapAndTargetedGroupUpdates(t *testing.T) {
	store := ownerLimitsDurableStore(t)
	a := requireSettingsView(t, store, testGroupA)
	b := requireSettingsView(t, store, testGroupB)
	c := ownerLimitsRegisterRuntime(t, store)
	if got := a.FallbackQuestions(); got.Source != SourceFactory || len(got.Value) != 1 {
		t.Fatalf("builtin fallback = %+v, want factory source with one question", got)
	}
	if got := b.FallbackQuestions(); got.Source != SourceUserFile || len(got.Value) != 2 {
		t.Fatalf("file fallback = %+v, want user-file source with two questions", got)
	}
	if got := c.FallbackQuestions(); got.Source != SourceFactory || len(got.Value) != 1 {
		t.Fatalf("registered inherited fallback = %+v, want factory source with one question", got)
	}
	for _, group := range []GroupView{a, b, c} {
		requireDeepEqual(t, group.Overrides(), GroupOverrides{}, "zero override group")
	}

	initialCap := int64(300)
	first, err := store.UpdateOwnerLimits(0, LimitChanges{"timeout_seconds": &initialCap})
	requireNoError(t, err)
	requireEqual(t, len(first.Violations), 0, "initial cap violations")
	loweredCap := int64(200)
	lowered, err := store.UpdateOwnerLimits(first.Revision, LimitChanges{"timeout_seconds": &loweredCap})
	requireNoError(t, err)
	requireDeepEqual(t, lowered.Violations, []LimitViolation{
		{ChatID: ownerLimitsRuntimeGroup, Field: "timeout_seconds", Value: 240, Limit: loweredCap},
		{ChatID: testGroupA, Field: "timeout_seconds", Value: 240, Limit: loweredCap},
	}, "lowered-cap violations")

	b = requireSettingsView(t, store, testGroupB)
	next := b.Overrides()
	next.Enabled = ptr(false)
	_, err = store.Update(testGroupB, b.Revision(), next)
	requireNoError(t, err)

	a = requireSettingsView(t, store, testGroupA)
	next = a.Overrides()
	next.TimeoutSeconds = ptr(180)
	_, err = store.Update(testGroupA, a.Revision(), next)
	requireNoError(t, err)
	a = requireSettingsView(t, store, testGroupA)
	restore := a.Overrides()
	restore.TimeoutSeconds = nil
	beforeRestore := a
	_, err = store.Update(testGroupA, a.Revision(), restore)
	requireErrorIs(t, err, ErrOwnerLimitsExceeded, "null restore of over-limit baseline")
	requireDeepEqual(t, requireSettingsView(t, store, testGroupA), beforeRestore, "failed null restore state")

	c = requireSettingsView(t, store, ownerLimitsRuntimeGroup)
	beforeNoop := c
	_, err = store.Update(c.ID(), c.Revision(), c.Overrides())
	requireErrorIs(t, err, ErrOwnerLimitsExceeded, "over-limit no-op")
	requireDeepEqual(t, requireSettingsView(t, store, ownerLimitsRuntimeGroup), beforeNoop, "failed no-op state")
}

func TestOwnerLimitsSparseMetadataReloadAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(path, ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	timeout, known := int64(300), int64(2)
	before := store.OwnerLimits()
	updated, err := store.UpdateOwnerLimits(before.Revision, LimitChanges{
		"timeout_seconds": &timeout, "known_chat_ids": &known,
	})
	requireNoError(t, err)
	_, err = store.UpdateOwnerLimits(before.Revision, LimitChanges{"warn_limit": ptr(int64(4))})
	requireErrorIs(t, err, ErrSettingsConflict, "stale owner-limits revision")
	requireDeepEqual(t, store.OwnerLimits(), updated, "CAS conflict state")

	reloaded, err := NewStore(path, ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	requireDeepEqual(t, reloaded.OwnerLimits(), updated, "sparse metadata reload")
	requireEqual(t, reloaded.OwnerLimits().Limits.TimeoutSeconds, timeout, "reloaded timeout")
	requireEqual(t, reloaded.OwnerLimits().Limits.KnownChatIDs, known, "reloaded known-chat cap")
	requireEqual(t, reloaded.OwnerLimits().Limits.WarnLimit, int64(0), "unchanged sparse limit")
}

func TestOwnerLimitsFailedPersistencePublishesNothing(t *testing.T) {
	runtime, err := NewStore("", ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	before := runtime.OwnerLimits()
	value := int64(300)
	_, err = runtime.UpdateOwnerLimits(before.Revision, LimitChanges{"timeout_seconds": &value})
	requireErrorIs(t, err, ErrSettingsNotDurable, "runtime-only owner limit update")
	requireDeepEqual(t, runtime.OwnerLimits(), before, "runtime-only snapshot")

	broken, err := NewStore(filepath.Join(t.TempDir(), "missing", "settings.json"), ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	before = broken.OwnerLimits()
	_, err = broken.UpdateOwnerLimits(before.Revision, LimitChanges{"timeout_seconds": &value})
	if err == nil {
		t.Fatal("owner-limit write unexpectedly succeeded with an unavailable parent directory")
	}
	requireDeepEqual(t, broken.OwnerLimits(), before, "failed owner-limit write snapshot")
}

func TestOwnerLimitsInvalidPersistedCapMakesStoreReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	requireNoError(t, os.WriteFile(path, []byte(`{"version":4,"limits":{"timeout_seconds":29},"groups":{}}`), 0o600))
	store, err := NewStore(path, ownerLimitsTestBaseline(), nil)
	requireNoError(t, err)
	if store.Persistence().Writable {
		t.Fatal("invalid persisted cap left settings writable")
	}
	_, err = store.UpdateOwnerLimits(0, LimitChanges{"timeout_seconds": ptr(int64(300))})
	requireErrorIs(t, err, ErrSettingsUnavailable, "owner write with invalid persisted caps")
}

func TestOwnerLimitsViolationsUseNumericChatAndStableFieldOrder(t *testing.T) {
	baseline := ownerLimitsTestBaseline()
	for index := range baseline.Groups {
		baseline.Groups[index].WarnLimit = BaselineValue[int]{Value: 3, Source: SourceUserFile}
	}
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil)
	requireNoError(t, err)
	state, err := store.UpdateOwnerLimits(0, LimitChanges{
		"timeout_seconds": ptr(int64(60)),
		"warn_limit":      ptr(int64(1)),
	})
	requireNoError(t, err)
	requireDeepEqual(t, state.Violations, []LimitViolation{
		{ChatID: testGroupB, Field: "timeout_seconds", Value: 120, Limit: 60},
		{ChatID: testGroupB, Field: "warn_limit", Value: 3, Limit: 1},
		{ChatID: testGroupA, Field: "timeout_seconds", Value: 240, Limit: 60},
		{ChatID: testGroupA, Field: "warn_limit", Value: 3, Limit: 1},
	}, "numeric chat order followed by stable field order")
}
