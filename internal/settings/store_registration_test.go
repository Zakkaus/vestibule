package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSettingsRejectsStaleGroupRevision(t *testing.T) {
	settings, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstView, _ := settings.Settings(testGroupA)
	secondView, _ := settings.Settings(testGroupA)
	first := firstView.Overrides()
	first.Enabled = ptr(false)
	if _, err := settings.Update(testGroupA, firstView.Revision(), first); err != nil {
		t.Fatalf("first writer: %v", err)
	}
	second := secondView.Overrides()
	second.NameSpoiler = ptr(false)
	_, err = settings.Update(testGroupA, secondView.Revision(), second)
	if !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("second writer error = %v, want ErrSettingsConflict", err)
	}
	group, _ := settings.Settings(testGroupA)
	if group.Enabled().Value || !group.NameSpoiler().Value || group.Revision() != 1 {
		t.Fatalf("published state after conflict = enabled:%v spoiler:%v revision:%d", group.Enabled().Value, group.NameSpoiler().Value, group.Revision())
	}
	t.Logf("second writer refused: %v", err)
}

func TestSettingsWriteFailurePublishesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "settings.json")
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	group, _ := settings.Settings(testGroupA)
	next := group.Overrides()
	next.Enabled = ptr(false)
	if _, err := settings.Update(testGroupA, group.Revision(), next); err == nil {
		t.Fatal("commit unexpectedly succeeded")
	}
	group, _ = settings.Settings(testGroupA)
	if !group.Enabled().Value || group.Revision() != 0 {
		t.Fatalf("failed write published enabled:%v revision:%d", group.Enabled().Value, group.Revision())
	}
}

type failingSettingsRepository struct {
	err error
}

func (r failingSettingsRepository) LoadSettings() ([]Record, error) {
	return nil, nil
}

func (r failingSettingsRepository) SeedSettings([]Record) error {
	return nil
}

func (r failingSettingsRepository) CompareAndSwapSettings(
	chatID int64,
	expectedRevision uint64,
	next GroupOverrides,
) (uint64, bool, error) {
	return expectedRevision, false, r.err
}

func TestUpdateWriteFailureKeepsSnapshot(t *testing.T) {
	writeErr := errors.New("database write interrupted")
	settings, err := NewStore("", testSettingsBaseline(), failingSettingsRepository{err: writeErr})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := settings.Settings(testGroupA)
	next := before.Overrides()
	next.Enabled = ptr(false)

	if _, err = settings.Update(testGroupA, before.Revision(), next); !errors.Is(err, writeErr) {
		t.Fatalf("Update error = %v, want %v", err, writeErr)
	}
	after, _ := settings.Settings(testGroupA)
	if !after.Enabled().Value || after.Revision() != before.Revision() {
		t.Fatalf("failed repository write published enabled:%v revision:%d",
			after.Enabled().Value, after.Revision())
	}
	status := settings.Persistence()
	if !errors.Is(status.LastError, writeErr) {
		t.Fatalf("persistence error = %v, want %v", status.LastError, writeErr)
	}
	t.Logf("repository write refused: %v; snapshot revision=%d enabled=%v",
		status.LastError, after.Revision(), after.Enabled().Value)
}

func TestSettingsUnknownVersionPreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	before := []byte(`{"version":99,"groups":{"-1009000000001":{"revision":7,"enabled":false}},"future":"keep"}`)
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status := settings.Persistence(); status.Writable || status.Durable || status.LastError == nil {
		t.Fatalf("future-version persistence status = %+v", status)
	}
	group, _ := settings.Settings(testGroupA)
	next := group.Overrides()
	next.Enabled = ptr(false)
	if _, err := settings.Update(testGroupA, group.Revision(), next); !errors.Is(err, ErrSettingsUnavailable) {
		t.Fatalf("future-version commit error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("future-version file changed\nwant %s\n got %s", before, after)
	}
}

func newOwnerClaimStore(t *testing.T) (*Store, string, time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	return settings, path, time.Unix(2_000_000_000, 0)
}

func TestSettingsOwnerClaimExpires(t *testing.T) {
	settings, _, now := newOwnerClaimStore(t)
	claim, created, err := settings.EnsureOwnerClaim(now, time.Minute)
	requireNoError(t, err)
	requireEqual(t, created, true, "first owner claim created")
	requireEqual(t, claim == "", false, "first owner claim empty")
	err = settings.ClaimOwner(42, claim, now.Add(time.Minute))
	requireErrorIs(t, err, ErrOwnerClaimInvalid, "expired owner claim error")
}

func TestSettingsOwnerClaimReplacementIsDistinct(t *testing.T) {
	settings, _, now := newOwnerClaimStore(t)
	first, _, err := settings.EnsureOwnerClaim(now, time.Minute)
	requireNoError(t, err)
	replacement, created, err := settings.EnsureOwnerClaim(now.Add(time.Minute), time.Minute)
	requireNoError(t, err)
	requireEqual(t, created, true, "replacement owner claim created")
	requireEqual(t, replacement == "", false, "replacement owner claim empty")
	requireEqual(t, replacement == first, false, "replacement owner claim equality")
}

func claimTestOwner(t *testing.T, settings *Store, now time.Time) string {
	t.Helper()
	_, _, err := settings.EnsureOwnerClaim(now, time.Minute)
	requireNoError(t, err)
	replacement, _, err := settings.EnsureOwnerClaim(now.Add(time.Minute), time.Minute)
	requireNoError(t, err)
	requireNoError(t, settings.ClaimOwner(42, replacement, now.Add(time.Minute)))
	return replacement
}

func TestSettingsOwnerClaimCannotBeReused(t *testing.T) {
	settings, _, now := newOwnerClaimStore(t)
	claim := claimTestOwner(t, settings, now)
	err := settings.ClaimOwner(43, claim, now.Add(time.Minute))
	requireErrorIs(t, err, ErrOwnerClaimInvalid, "used owner claim error")
}

func TestSettingsEnrollmentNonceRequiresOwner(t *testing.T) {
	settings, _, now := newOwnerClaimStore(t)
	claimTestOwner(t, settings, now)
	_, err := settings.IssueEnrollmentNonce(43, now, time.Minute)
	requireErrorIs(t, err, ErrRegistrationOwnerOnly, "non-owner enrollment error")
}

func TestSettingsOwnerEnrollmentNoncePersists(t *testing.T) {
	settings, path, now := newOwnerClaimStore(t)
	claimTestOwner(t, settings, now)
	issued, err := settings.IssueEnrollmentNonce(42, now, time.Minute)
	requireNoError(t, err)
	requireEqual(t, issued.Nonce == "", false, "owner enrollment nonce empty")
	requireEqual(t, issued.IssuedBy, int64(42), "owner enrollment nonce issuer")
	reloaded, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	state := reloaded.Registrations()
	requireEqual(t, state.OwnerID, int64(42), "registration owner after reload")
	requireEqual(t, state.OwnerClaimNonce, "", "registration owner claim after reload")
	requireEqual(t, len(state.EnrollmentNonces), 1, "registration enrollment nonces after reload")
}

func newRegistrationFixture(t *testing.T) (*Store, string, CommitResult) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	registration := settings.Registrations()
	registration.OwnerID = 42
	registration.RegisteredGroups = []RegisteredGroup{{ID: -1009000000003, RegisteredBy: 42, Title: "Runtime"}}
	registration.EnrollmentNonces = []EnrollmentNonce{{Nonce: "enroll", IssuedBy: 42, ExpiresAt: 1000}}
	registration.PendingRegistrations = []PendingRegistration{{GroupID: -1009000000004, RegisteredBy: 42, Title: "Pending", ExpiresAt: 2000}}
	registration.UnknownGroupLeaves = []UnknownGroupLeave{{GroupID: -1009000000005, Title: "Cleanup", ExpiresAt: 3000}}
	committed, err := settings.CommitRegistrations(registration.Revision, registration)
	requireNoError(t, err)
	return settings, path, committed
}

func updateRegistrationRuntimeGroup(t *testing.T, settings *Store) {
	t.Helper()
	runtimeGroup := requireSettingsView(t, settings, -1009000000003)
	overrides := runtimeGroup.Overrides()
	overrides.TimeoutSeconds = ptr(900)
	_, err := settings.Update(runtimeGroup.ID(), runtimeGroup.Revision(), overrides)
	requireNoError(t, err)
}

func TestSettingsRuntimeOnlyCommitIsNotDurable(t *testing.T) {
	runtimeOnly, err := NewStore("", testSettingsBaseline(), nil)
	requireNoError(t, err)
	group := requireSettingsView(t, runtimeOnly, testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	result, err := runtimeOnly.Update(testGroupA, group.Revision(), overrides)
	requireNoError(t, err)
	requireEqual(t, result.Durable, false, "runtime-only commit durability")
}

func TestSettingsRuntimeOnlyRegistrationsRequireDurability(t *testing.T) {
	runtimeOnly, err := NewStore("", testSettingsBaseline(), nil)
	requireNoError(t, err)
	_, err = runtimeOnly.CommitRegistrations(0, runtimeOnly.Registrations())
	requireErrorIs(t, err, ErrSettingsNotDurable, "runtime-only registration error")
}

func TestSettingsRegistrationCommitCreatesRuntimeGroup(t *testing.T) {
	settings, _, committed := newRegistrationFixture(t)
	requireEqual(t, committed.Revision, uint64(1), "registration commit revision")
	requireEqual(t, committed.Durable, true, "registration commit durability")
	requireEqual(t, settings.IsGroup(-1009000000003), true, "registration runtime group")
}

func TestSettingsChatTitleReturnsRegisteredRuntimeMetadata(t *testing.T) {
	settings, _, _ := newRegistrationFixture(t)
	requireEqual(t, settings.ChatTitle(-1009000000003), "Runtime", "registered group title")
	requireEqual(t, settings.ChatTitle(testGroupA), "", "configured group title")
}

func TestSettingsRegistrationRoundTripPreservesMetadataAndOverrides(t *testing.T) {
	settings, path, _ := newRegistrationFixture(t)
	updateRegistrationRuntimeGroup(t, settings)
	reloaded, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	requireDeepEqual(t, reloaded.ChatIDs(), []int64{testGroupA, testGroupB, -1009000000003}, "effective groups after reload")
	metadata := reloaded.Registrations()
	requireEqual(t, metadata.Revision, uint64(1), "registration metadata revision")
	requireEqual(t, metadata.OwnerID, int64(42), "registration metadata owner")
	requireEqual(t, metadata.OwnerClaimNonce, "", "registration metadata owner claim")
	requireEqual(t, len(metadata.EnrollmentNonces), 1, "registration metadata enrollment nonces")
	requireEqual(t, len(metadata.PendingRegistrations), 1, "registration metadata pending registrations")
	requireEqual(t, len(metadata.UnknownGroupLeaves), 1, "registration metadata unknown leaves")
	runtimeGroup := requireSettingsView(t, reloaded, -1009000000003)
	requireEqual(t, runtimeGroup.TimeoutSeconds(), Setting[int]{Value: 900, Source: SourceChatOverride}, "runtime group timeout after reload")
}

func TestSettingsRegistrationRejectsPendingAndUnknownSameGroup(t *testing.T) {
	settings, path, _ := newRegistrationFixture(t)
	updateRegistrationRuntimeGroup(t, settings)
	reloaded, err := NewStore(path, testSettingsBaseline(), nil)
	requireNoError(t, err)
	metadata := reloaded.Registrations()
	invalid := metadata
	invalid.UnknownGroupLeaves = []UnknownGroupLeave{{
		GroupID: metadata.PendingRegistrations[0].GroupID, ExpiresAt: 3000,
	}}
	_, err = reloaded.CommitRegistrations(metadata.Revision, invalid)
	requireEqual(t, err == nil, false, "overlapping registration cleanup accepted")
}

func TestSettingsEveryCommitPreservesRegistrationMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	registration := settings.Registrations()
	registration.OwnerID = 99
	if _, err := settings.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	group, _ := settings.Settings(testGroupA)
	overrides := group.Overrides()
	overrides.NameSpoiler = ptr(false)
	overrides.RichMessages = ptr(true)
	if _, err := settings.Update(testGroupA, group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	var state settingsFile
	decodeFile(t, path, &state)
	if state.OwnerID != 99 || state.OwnerClaimNonce != "" {
		t.Fatalf("metadata lost after setters: %#v", state)
	}
	record := state.Groups[testGroupA]
	if record.NameSpoiler == nil || *record.NameSpoiler || record.RichMessages == nil || !*record.RichMessages {
		t.Fatalf("chat override lost after registration commit: %#v", record)
	}
}

func TestSettingsUnreadableExistingPathDisablesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, err := NewStore(path, testSettingsBaseline(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status := settings.Persistence(); status.Writable || status.LastError == nil {
		t.Fatalf("unreadable path status = %+v", status)
	}
	group, _ := settings.Settings(testGroupA)
	overrides := group.Overrides()
	overrides.Enabled = ptr(false)
	if _, err := settings.Update(testGroupA, group.Revision(), overrides); !errors.Is(err, ErrSettingsUnavailable) {
		t.Fatalf("commit error = %v, want ErrSettingsUnavailable", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("unreadable state path was overwritten")
	}
}
