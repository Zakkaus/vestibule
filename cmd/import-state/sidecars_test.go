package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSidecarsRequireExplicitAcceptanceWithoutTouchingDestination(t *testing.T) {
	directory := legacyState(t)
	names := []string{"settings.json", "antispam.json", "group-titles.json", "feed--1009000000711.json"}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("{truncated sidecar"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	destination := t.TempDir()
	db := filepath.Join(destination, "new.db")
	backup := filepath.Join(destination, "backup")
	args := []string{"--state-dir", directory, "--database-uri", "file:" + db, "--backup-dir", backup, "--pending", "drop"}
	denied := importState(t, nil, args...)
	if denied.err == nil {
		t.Fatal("import silently omitted legacy sidecars")
	}
	for _, fragment := range append(names, "--accept-unimported", "archive the whole legacy state and config tree", "by hand") {
		if !strings.Contains(denied.stderr, fragment) {
			t.Errorf("refusal omits %q: %s", fragment, denied.stderr)
		}
	}
	for _, path := range []string{db, backup} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("refused import touched %s: %v", path, err)
		}
	}
	accepted := importState(t, nil, append(args, "--accept-unimported")...)
	if accepted.err != nil {
		t.Fatalf("accepted import: %v\n%s", accepted.err, accepted.stderr)
	}
	for _, name := range names {
		if !strings.Contains(accepted.stdout, name) {
			t.Errorf("report omits unimported %s", name)
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil || string(data) != "{truncated sidecar" {
			t.Errorf("unimported file %s changed: %q/%v", name, data, err)
		}
		if _, err := os.Stat(filepath.Join(backup, name)); !os.IsNotExist(err) {
			t.Errorf("five-file backup claims to include %s", name)
		}
	}
}
