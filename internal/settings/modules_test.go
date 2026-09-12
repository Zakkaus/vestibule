package settings

import (
	"strings"
	"testing"
)

func TestOptionalModuleConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		present bool
		modules []string
		want    map[string]bool
	}{
		{
			name: "absent disables every optional module",
			want: map[string]bool{ModuleGentoo: false, ModuleLinux: false},
		},
		{
			name:    "empty disables every optional module",
			present: true,
			modules: []string{},
			want:    map[string]bool{ModuleGentoo: false, ModuleLinux: false},
		},
		{
			name:    "explicit selection enables only gentoo",
			present: true,
			modules: []string{ModuleGentoo},
			want:    map[string]bool{ModuleGentoo: true, ModuleLinux: false},
		},
		{
			name:    "explicit selection enables only linux",
			present: true,
			modules: []string{ModuleLinux},
			want:    map[string]bool{ModuleGentoo: false, ModuleLinux: true},
		},
		{
			name:    "explicit selection enables both",
			present: true,
			modules: []string{ModuleGentoo, ModuleLinux},
			want:    map[string]bool{ModuleGentoo: true, ModuleLinux: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{}
			if tc.present {
				config["modules"] = tc.modules
			}
			loaded, err := LoadConfig(writeConfig(t, config))
			if err != nil {
				t.Fatal(err)
			}
			for module, want := range tc.want {
				if got := loaded.ModuleEnabled(module); got != want {
					t.Errorf("ModuleEnabled(%q) = %v, want %v", module, got, want)
				}
			}
		})
	}
}

func TestLoadConfigRejectsInvalidOrDuplicateModules(t *testing.T) {
	for _, tc := range []struct {
		name    string
		modules []string
	}{
		{name: "unknown", modules: []string{"missing"}},
		{name: "duplicate", modules: []string{ModuleGentoo, ModuleGentoo}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfig(writeConfig(t, map[string]any{"modules": tc.modules}))
			if err == nil || !strings.Contains(err.Error(), "modules") {
				t.Fatalf("LoadConfig(%#v) error = %v, want modules validation error", tc.modules, err)
			}
		})
	}
}

func TestLoadConfigRejectsLegacyDisabledModulesWithMigrationGuidance(t *testing.T) {
	_, err := LoadConfig(writeConfig(t, map[string]any{
		"disabled_modules": []string{ModuleGentoo},
	}))
	if err == nil || !strings.Contains(err.Error(), "disabled_modules") ||
		!strings.Contains(strings.ToLower(err.Error()), "modules") {
		t.Fatalf("legacy disabled_modules error = %v, want migration guidance to modules", err)
	}
}
