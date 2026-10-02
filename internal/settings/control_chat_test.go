package settings

import (
	"errors"
	"path/filepath"
	"strconv"
	"testing"
)

const controlTestChat int64 = -1009000000610

func TestControlChatDefaultsAndReverseLookup(t *testing.T) {
	for _, chat := range []int64{0, controlTestChat} {
		t.Run(strconv.FormatInt(chat, 10), func(t *testing.T) {
			baseline := testSettingsBaseline()
			baseline.Groups[0].ControlChatID = factoryBaseline().ControlChatID
			if chat != 0 {
				baseline.Groups[0].ControlChatID = userFileValue(chat)
			}
			store, err := NewStore("", baseline, nil, nil)
			requireNoError(t, err)
			group := requireSettingsView(t, store, testGroupA)
			requireEqual(t, group.ControlChatID().Value, chat, "control chat")
			resolved, ok := store.ControlGroup(controlTestChat)
			requireEqual(t, ok, chat != 0, "reverse lookup")
			if ok {
				requireEqual(t, resolved.ID(), testGroupA, "protected group")
				requireEqual(t, store.IsKnownChat(chat), true, "auto-leave exemption")
				requireEqual(t, store.IsGroup(chat), false, "no protected-group privileges")
			}
			_, zero := store.ControlGroup(0)
			requireEqual(t, zero, false, "disabled control chat never resolves")
		})
	}
}

func TestControlChatWriteRejectionsAreAtomic(t *testing.T) {
	cases := []struct {
		name     string
		chat     int64
		check    func(int64, int64) error
		occupied bool
		conflict bool
	}{
		{name: "self", chat: testGroupA},
		{name: "occupied", chat: controlTestChat, occupied: true, conflict: true},
		{name: "bot absent", chat: controlTestChat, check: func(int64, int64) error { return errors.New("left") }},
		{name: "membership unavailable", chat: controlTestChat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline := testSettingsBaseline()
			if tc.occupied {
				baseline.Groups[1].ControlChatID = userFileValue(controlTestChat)
			}
			store, err := NewStore("", baseline, nil, nil)
			requireNoError(t, err)
			store.SetControlChatMembership(tc.check)
			before := requireSettingsView(t, store, testGroupA)
			next := before.Overrides()
			next.ControlChatID = &tc.chat
			_, err = store.Update(testGroupA, before.Revision(), next, 7)
			if tc.conflict {
				var conflict *ControlChatConflictError
				if !errors.As(err, &conflict) || conflict.OtherGroupID != testGroupB {
					t.Fatalf("conflict = %v, want other group %d", err, testGroupB)
				}
			} else {
				requireErrorIs(t, err, ErrControlChatInvalid, "rejected assignment")
			}
			after := requireSettingsView(t, store, testGroupA)
			requireEqual(t, after.Revision(), before.Revision(), "failed write revision")
			requireEqual(t, after.ControlChatID(), before.ControlChatID(), "failed write value")
		})
	}
}

func TestControlChatAssignmentPersistsAndClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(path, testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	store.SetControlChatMembership(func(chat, actor int64) error {
		requireEqual(t, chat, controlTestChat, "live membership target")
		return nil
	})
	group := requireSettingsView(t, store, testGroupA)
	next := group.Overrides()
	next.ControlChatID = ptr(controlTestChat)
	_, err = store.Update(group.ID(), group.Revision(), next, 7)
	requireNoError(t, err)
	reloaded, err := NewStore(path, testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	group = requireSettingsView(t, reloaded, testGroupA)
	requireEqual(t, group.ControlChatID(), Setting[int64]{Value: controlTestChat, Source: SourceChatOverride}, "persisted assignment")
	next = group.Overrides()
	next.ControlChatID = ptr(int64(0))
	_, err = reloaded.Update(group.ID(), group.Revision(), next, 7)
	requireNoError(t, err)
	_, found := reloaded.ControlGroup(controlTestChat)
	requireEqual(t, found, false, "cleared reverse lookup")
}

func TestControlChatConfigImport(t *testing.T) {
	for _, tc := range []struct {
		name        string
		chats, want []int64
	}{
		{name: "off", chats: []int64{0, 0}, want: []int64{0, 0}},
		{name: "assigned", chats: []int64{controlTestChat, 0}, want: []int64{controlTestChat, 0}},
		{name: "self", chats: []int64{testGroupA, 0}, want: []int64{0, 0}},
		{name: "duplicate", chats: []int64{controlTestChat, controlTestChat}, want: []int64{controlTestChat, 0}},
		{name: "private ID", chats: []int64{42, 0}, want: []int64{0, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfig(t, map[string]any{"groups": []map[string]any{
				{"id": testGroupA, "control_chat_id": tc.chats[0]},
				{"id": testGroupB, "control_chat_id": tc.chats[1]},
			}})
			cfg, err := LoadConfig(path)
			requireNoError(t, err)
			baseline, err := LoadBaseline(path, cfg)
			requireNoError(t, err)
			store, err := NewStore("", baseline, nil, nil)
			requireNoError(t, err)
			for i, id := range []int64{testGroupA, testGroupB} {
				group := requireSettingsView(t, store, id)
				requireEqual(t, group.ControlChatID(), Setting[int64]{Value: tc.want[i], Source: SourceUserFile}, "file assignment")
			}
		})
	}
}
