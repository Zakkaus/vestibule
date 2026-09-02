package settings

import "testing"

func TestObserveOnlyConfigIsTypedAndDefaultsOff(t *testing.T) {
	defaults, err := LoadConfig(writeConfig(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if defaults.ObserveOnly {
		t.Fatal("observe-only mode is enabled by default")
	}
	enabled, err := LoadConfig(writeConfig(t, map[string]any{"observe_only": true}))
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.ObserveOnly {
		t.Fatal("observe_only=true did not enable the mode")
	}
	if _, err = LoadConfig(writeConfig(t, map[string]any{"observe_only": "true"})); err == nil {
		t.Fatal("string observe_only value was accepted")
	}
}
