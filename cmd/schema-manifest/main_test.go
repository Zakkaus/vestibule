package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/migrations"
)

func TestPositionalArgumentsAreRefused(t *testing.T) {
	if err := run(nil, io.Discard); err != nil {
		t.Fatalf("run without positional arguments: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"deploy/vestibule-schema-manifest"}, &stdout)
	if err == nil {
		t.Fatal("run with a positional argument succeeded; a release gate would verify nothing")
	}
	if !strings.Contains(err.Error(), "accepts no positional arguments") {
		t.Fatalf("positional argument error = %q, want a clear refusal", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run with a positional argument wrote %q before refusing", stdout.String())
	}
}

func TestOutputAndCheckAreMutuallyExclusive(t *testing.T) {
	directory := t.TempDir()
	checkPath := filepath.Join(directory, "current-manifest")
	if err := os.WriteFile(checkPath, currentManifestText(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-check", checkPath}, io.Discard); err != nil {
		t.Fatalf("run with only -check: %v", err)
	}

	outputPath := filepath.Join(directory, "regenerated-manifest")
	err := run([]string{"-output", outputPath, "-check", checkPath}, io.Discard)
	if err == nil {
		t.Fatal("run with -output and -check succeeded; the requested output could be left stale")
	}
	if !strings.Contains(err.Error(), "accepts only one of -output or -check") {
		t.Fatalf("combined flag error = %q, want a clear refusal", err)
	}
}

func TestCheckVerifiesTheNamedManifest(t *testing.T) {
	directory := t.TempDir()
	currentPath := filepath.Join(directory, "current-manifest")
	if err := os.WriteFile(currentPath, currentManifestText(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-check", currentPath}, io.Discard); err != nil {
		t.Fatalf("checking the current manifest: %v", err)
	}

	stalePath := filepath.Join(directory, "stale-manifest")
	if err := os.WriteFile(stalePath, []byte("target_schema_version=0\nminimum_rollback_schema_version=0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := run([]string{"-check", stalePath}, &stdout)
	if err == nil {
		t.Fatal("a stale manifest passed -check; the release gate would accept mismatched schema metadata")
	}
	if !strings.Contains(err.Error(), "does not match migrations.Table") {
		t.Fatalf("stale manifest error = %q, want a mismatch refusal", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("-check wrote %q to standard output instead of only verifying", stdout.String())
	}
}

func TestOutputWritesTheCompleteManifest(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "vestibule-schema-manifest")
	var stdout bytes.Buffer
	if err := run([]string{"-output", outputPath}, &stdout); err != nil {
		t.Fatalf("writing the manifest: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("-output also wrote %q to standard output", stdout.String())
	}

	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("reading the output manifest: %v", err)
	}
	want := currentManifestText(t)
	if !bytes.Equal(got, want) {
		t.Fatalf("-output wrote %q, want the full manifest %q; an incomplete release manifest blocks installation", got, want)
	}
}

func currentManifestText(t *testing.T) []byte {
	t.Helper()
	manifest, err := migrations.CurrentSchemaManifest()
	if err != nil {
		t.Fatal(err)
	}
	return []byte(manifest.String())
}
