package panel

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestPanelLanguageCallbacksPersistJapaneseAndRussian(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		tag   string
		label i18n.Text
	}{
		{name: "Japanese", value: "j", tag: "ja", label: i18n.Messages.Panel.Settings.Field.LanguageJA},
		{name: "Russian", value: "r", tag: "ru", label: i18n.Messages.Panel.Settings.Field.LanguageRU},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := t.TempDir() + "/settings.json"
			panel, store, caller, bot := newSettingsPanelTest(t, path)
			session := addPanelSession(t, panel, store, panelTestGroupA, "rt")
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, "lg", test.value)

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var state struct {
				Groups map[string]struct {
					Lang *string `json:"lang"`
				} `json:"groups"`
			}
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatal(err)
			}
			persisted := state.Groups[strconv.FormatInt(panelTestGroupA, 10)].Lang
			if persisted == nil || *persisted != test.tag {
				t.Fatalf("language override after %s panel selection = %v, want persisted %s", test.name, persisted, test.tag)
			}
			group, ok := store.Settings(panelTestGroupA)
			if !ok {
				t.Fatal("panel group disappeared after language selection")
			}
			if got := group.Lang(); got.Value != test.tag || got.Source != settings.SourceChatOverride {
				t.Fatalf("effective %s language after panel selection = %+v", test.name, got)
			}
			if got, want := caller.lastEditText, expectedRuntimeScreen(panel, group, i18n.FromStored(test.tag)); got != want {
				t.Fatalf("%s runtime screen = %q, want catalogue rendering %q", test.name, got, want)
			}
			rendered := test.label.For(i18n.FromStored(test.tag))
			if !strings.Contains(caller.lastEditText, rendered) {
				t.Fatalf("%s runtime screen %q omitted catalogue label %q", test.name, caller.lastEditText, rendered)
			}
		})
	}
}

func expectedRuntimeActions(groupID int64) []string {
	return []string{
		action(groupID, "en", "_"),
		action(groupID, "gt", "_"), action(groupID, "lx", "_"),
		action(groupID, "df", "g"), action(groupID, "df", "d"), action(groupID, "df", "b"),
		action(groupID, "vm", "k"), action(groupID, "vm", "q"), action(groupID, "vm", "m"),
		action(groupID, "ns", "_"), action(groupID, "bd", "_"), action(groupID, "ld", "_"),
		action(groupID, "lt", "_"), action(groupID, "lg", "z"), action(groupID, "lg", "h"),
		action(groupID, "lg", "e"), action(groupID, "lg", "j"), action(groupID, "lg", "r"),
		action(groupID, "go", "gh"),
	}
}
