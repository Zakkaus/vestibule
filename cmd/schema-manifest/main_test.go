package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/migrations"
)

func TestCheckingAnUnreadableManifestNamesItsPath(t *testing.T) {
	unreadable := filepath.Join(t.TempDir(), "missing-schema-manifest")
	err := run([]string{"-check", unreadable}, io.Discard)
	if err == nil {
		t.Fatal("-check accepted a manifest it could not read; it would tell an operator to regenerate a healthy manifest")
	}
	if !strings.Contains(err.Error(), "read schema manifest "+unreadable) {
		t.Fatalf("-check reported an unreadable manifest as %q; want a read refusal naming %s", err, unreadable)
	}

	manifest, err := migrations.CurrentSchemaManifest()
	if err != nil {
		t.Fatal(err)
	}
	readable := filepath.Join(t.TempDir(), "schema-manifest")
	if err := os.WriteFile(readable, []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-check", readable}, io.Discard); err != nil {
		t.Fatalf("-check rejected a readable current manifest: %v", err)
	}
}
