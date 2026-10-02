package settings

import (
	"bytes"
	"errors"
	"log"
	"path/filepath"
	"strings"
	"testing"

	statefile "github.com/Zakkaus/vestibule/internal/store"
)

func TestControlChatRound2UnrelatedWritesAfterBotLeaves(t *testing.T) {
	baseline := testSettingsBaseline()
	baseline.Groups[0].ControlChatID = userFileValue(controlTestChat)
	store, err := NewStore("", baseline, nil, nil)
	requireNoError(t, err)
	store.SetControlChatMembership(func(int64, int64) error { return errors.New("bot left") })
	group := requireSettingsView(t, store, testGroupA)
	next := group.Overrides()
	next.MuteSeconds = new(900)
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	requireNoError(t, err)
	group = requireSettingsView(t, store, testGroupA)
	requireEqual(t, group.MuteSeconds().Value, 900, "unrelated write after departure")
	next = group.Overrides()
	next.ControlChatID = new(controlTestChat - 1)
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	requireErrorIs(t, err, ErrControlChatInvalid, "different assignment requires membership")
	requireEqual(t, requireSettingsView(t, store, testGroupA).Revision(), group.Revision(), "rejected revision")
	next.ControlChatID = new(int64(0))
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	requireNoError(t, err)
	group = requireSettingsView(t, store, testGroupA)
	next = group.Overrides()
	next.ControlChatID = nil
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	requireErrorIs(t, err, ErrControlChatInvalid, "restoring a nonzero file assignment checks membership")
}

func TestControlChatRound2RevalidatesClaimWithoutWriterLock(t *testing.T) {
	store, err := NewStore("", testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	store.SetControlChatMembership(func(chatID, actorID int64) error {
		if !store.writer.TryLock() {
			return errors.New("membership lookup holds the global writer lock")
		}
		store.writer.Unlock()
		if actorID == 8 {
			return nil
		}
		other := requireSettingsView(t, store, testGroupB)
		next := other.Overrides()
		next.ControlChatID = new(chatID)
		_, err := store.Update(other.ID(), other.Revision(), next, 8)
		return err
	})
	group := requireSettingsView(t, store, testGroupA)
	next := group.Overrides()
	next.ControlChatID = new(controlTestChat)
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	var conflict *ControlChatConflictError
	if !errors.As(err, &conflict) || conflict.OtherGroupID != testGroupB {
		t.Fatalf("assignment raced another claimant: %v", err)
	}
	requireEqual(t, requireSettingsView(t, store, testGroupA).ControlChatID().Value, int64(0), "losing claim")
	requireEqual(t, requireSettingsView(t, store, testGroupB).ControlChatID().Value, controlTestChat, "winning claim")
}

func TestControlChatRound2StartupPreservesAllProtection(t *testing.T) {
	const registeredID int64 = -1009000000703
	for _, tc := range []struct {
		name                 string
		fileChat, storedChat int64
	}{
		{"file conflicts with stored claim", controlTestChat, 0},
		{"file self", testGroupB, 0},
		{"file private ID", 42, 0},
		{"stored self", 0, registeredID},
		{"stored duplicate", 0, controlTestChat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline := testSettingsBaseline()
			baseline.Groups[1].ControlChatID = userFileValue(tc.fileChat)
			path := filepath.Join(t.TempDir(), "settings.json")
			state := settingsFile{Version: SettingsSchemaVersion, OwnerID: 42,
				RegisteredGroups: []RegisteredGroup{{ID: registeredID, RegisteredBy: 42}},
				Groups: map[int64]groupRecord{
					testGroupA:   {Revision: 7, GroupOverrides: GroupOverrides{ControlChatID: new(controlTestChat), Enabled: new(false)}},
					registeredID: {Revision: 9, GroupOverrides: GroupOverrides{ControlChatID: new(tc.storedChat), MuteSeconds: new(900)}},
				}}
			requireNoError(t, statefile.Write(path, state))
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(previous) })
			store, err := NewStore(path, baseline, nil, nil)
			requireNoError(t, err)
			if store.snapshot.Load() == nil || !store.IsGroup(testGroupA) || !store.IsGroup(testGroupB) || !store.IsGroup(registeredID) {
				t.Fatal("invalid control assignment removed protected groups")
			}
			a := requireSettingsView(t, store, testGroupA)
			runtime := requireSettingsView(t, store, registeredID)
			requireEqual(t, a.ControlChatID().Value, controlTestChat, "earlier stored claim")
			requireEqual(t, a.Enabled().Value, false, "stored verification setting")
			requireEqual(t, a.Revision(), uint64(7), "stored revision")
			requireEqual(t, runtime.Revision(), uint64(9), "registered revision")
			requireEqual(t, runtime.MuteSeconds().Value, 900, "registered moderation setting")
			requireEqual(t, runtime.ControlChatID().Value, int64(0), "later stored claim dropped")
			requireEqual(t, requireSettingsView(t, store, testGroupB).ControlChatID().Value, int64(0), "invalid file claim dropped")
			requireEqual(t, strings.Count(logs.String(), "dropped control chat"), 1, "one assignment log")
			if !store.Persistence().Writable || store.Persistence().LastError != nil {
				t.Fatalf("control conflict poisoned startup: %+v", store.Persistence())
			}
		})
	}
}

func TestControlChatRound2StaleApprovalsCannotCommit(t *testing.T) {
	for _, replaceChecker := range []bool{false, true} {
		t.Run(map[bool]string{false: "group revision", true: "checker replaced"}[replaceChecker], func(t *testing.T) {
			store, err := NewStore("", testSettingsBaseline(), nil, nil)
			requireNoError(t, err)
			store.SetControlChatMembership(func(int64, int64) error {
				if !store.writer.TryLock() {
					return errors.New("membership lookup holds the writer lock")
				}
				store.writer.Unlock()
				if replaceChecker {
					store.SetControlChatMembership(nil)
					return nil
				}
				group := requireSettingsView(t, store, testGroupA)
				next := group.Overrides()
				next.MuteSeconds = new(900)
				_, err := store.Update(group.ID(), group.Revision(), next, 7)
				return err
			})
			group := requireSettingsView(t, store, testGroupA)
			next := group.Overrides()
			next.ControlChatID = new(controlTestChat)
			_, err = store.Update(group.ID(), group.Revision(), next, 7)
			want := ErrSettingsConflict
			if replaceChecker {
				want = ErrControlChatInvalid
			}
			requireErrorIs(t, err, want, "stale assignment approval")
			requireEqual(t, requireSettingsView(t, store, testGroupA).ControlChatID().Value, int64(0), "stale approval did not assign")
		})
	}
}

func TestControlChatRound2MissingActorCannotAssign(t *testing.T) {
	store, err := NewStore("", testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	store.SetControlChatMembership(func(int64, int64) error { return nil })
	group := requireSettingsView(t, store, testGroupA)
	next := group.Overrides()
	next.ControlChatID = new(controlTestChat)
	_, err = store.Update(group.ID(), group.Revision(), next, 0)
	requireErrorIs(t, err, ErrControlChatInvalid, "assignment without an actor")
	requireEqual(t, requireSettingsView(t, store, testGroupA).Revision(), group.Revision(), "unauthorized assignment revision")
}
