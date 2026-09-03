package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest this command prints is what deploy/install-common.sh reads to decide whether an
// upgrade or a rollback is safe. Nothing in the repository exercised the command itself: the
// release gate runs scripts/check-schema-manifest.py, which shells out to it and believes the
// exit code. Every flag below is therefore a promise made to a script that has no way of
// noticing when the promise stops being kept.

// currentManifest is the text the command derives from migrations.Table, taken through the
// command's own printing path so a test never has a second idea of what the manifest is.
func currentManifest(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := run(nil, &out); err != nil {
		t.Fatalf("printing the manifest: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("the manifest is empty; every assertion below would hold vacuously")
	}
	return out.Bytes()
}

func writeFile(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The whole point of -check is to catch a manifest that no longer describes migrations.Table.
// A comparison that stops reporting a difference leaves CI printing "passed" while the released
// manifest names a target and a minimum-rollback schema version that are not the real ones, and
// install-common.sh decides an upgrade or a rollback from exactly those two numbers.
func TestCheckRefusesAManifestThatDiffersFromTheMigrationTable(t *testing.T) {
	directory := t.TempDir()
	text := currentManifest(t)
	drifted := writeFile(t, filepath.Join(directory, "drifted"), append(bytes.Clone(text), []byte("target_schema_version 99\n")...))

	err := run([]string{"-check", drifted}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("-check accepted a manifest with a line appended; a released manifest could then " +
			"name schema versions that install-common.sh trusts and migrations.Table does not")
	}
	if !strings.Contains(err.Error(), drifted) {
		t.Errorf("refusal = %q, want the path named so an operator knows which file to regenerate", err)
	}

	// Without this control a refusal proves only that the invocation was malformed.
	matching := writeFile(t, filepath.Join(directory, "matching"), text)
	if err := run([]string{"-check", matching}, &bytes.Buffer{}); err != nil {
		t.Fatalf("-check rejected the manifest the command itself prints: %v", err)
	}
}

// Passing -check has to select the comparison. If it does not, -check degrades into printing the
// manifest and exiting zero, and the release gate reports a passing schema check on every release
// without ever having compared anything.
func TestCheckComparesRatherThanPrints(t *testing.T) {
	matching := writeFile(t, filepath.Join(t.TempDir(), "manifest"), currentManifest(t))
	var out bytes.Buffer
	if err := run([]string{"-check", matching}, &out); err != nil {
		t.Fatalf("-check on a matching manifest: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("-check wrote %q to standard output; it printed the manifest instead of "+
			"comparing it, which is the same as not checking at all", out.String())
	}
}

// A -check target that cannot be read is not a target that disagrees. Reporting it as a mismatch
// sends the operator to regenerate a manifest that is fine, and leaves the real fault — a path
// that is wrong, or a file the process may not read — undiagnosed.
func TestAnUnreadableCheckTargetIsRefusedAsUnreadable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-written-yet")
	err := run([]string{"-check", missing}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("-check accepted a target that does not exist")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("refusal = %q, want the unreadable path named", err)
	}
	if strings.Contains(err.Error(), "does not match") {
		t.Errorf("refusal = %q; a file that could not be read is reported as a manifest that "+
			"disagrees, so the operator regenerates a manifest that was never the problem", err)
	}
}

// -output is how the manifest is regenerated, and .github/workflows/release.yml attaches whatever
// it wrote to the release. A truncated or empty file passes every gate here and fails every
// install: deploy/install-common.sh refuses a manifest it considers incomplete.
func TestOutputWritesTheWholeManifest(t *testing.T) {
	text := currentManifest(t)
	path := filepath.Join(t.TempDir(), "vestibule-schema-manifest")
	if err := run([]string{"-output", path}, &bytes.Buffer{}); err != nil {
		t.Fatalf("-output: %v", err)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what -output wrote: %v", err)
	}
	if !bytes.Equal(written, text) {
		t.Fatalf("-output wrote %d bytes, want the %d-byte manifest\nwrote:\n%s\nwant:\n%s",
			len(written), len(text), written, text)
	}
	// Regenerating and then verifying is the sequence a release performs, so the file -output
	// leaves behind has to satisfy the check that ships with it.
	if err := run([]string{"-check", path}, &bytes.Buffer{}); err != nil {
		t.Fatalf("-check rejected the file -output had just written: %v", err)
	}
}

// -output and -check both name a file and mean opposite things. Resolving them silently means
// -check wins, the -output file is never written, and an operator who regenerated and verified in
// one command ships the stale manifest believing it was rewritten.
func TestOutputAndCheckTogetherAreRefused(t *testing.T) {
	directory := t.TempDir()
	matching := writeFile(t, filepath.Join(directory, "manifest"), currentManifest(t))
	output := filepath.Join(directory, "regenerated")

	err := run([]string{"-output", output, "-check", matching}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("-output and -check were accepted together")
	}
	if _, statErr := os.Stat(output); statErr == nil {
		t.Error("the -output file exists after the refusal; the command did half of what it was asked")
	}

	// Either flag on its own is the supported use, so the refusal is about the combination.
	if err := run([]string{"-output", output}, &bytes.Buffer{}); err != nil {
		t.Fatalf("-output alone: %v", err)
	}
	if err := run([]string{"-check", matching}, &bytes.Buffer{}); err != nil {
		t.Fatalf("-check alone: %v", err)
	}
}

// `go run ./cmd/schema-manifest deploy/vestibule-schema-manifest` is the natural typo for the
// check invocation. Treated as a target it would verify nothing; treated as a stray argument and
// printed to standard output it would exit zero, and a release gate written that way reports
// success on a manifest it never looked at.
func TestAPositionalArgumentIsRefusedRatherThanTreatedAsATarget(t *testing.T) {
	path := writeFile(t, filepath.Join(t.TempDir(), "manifest"), currentManifest(t))
	var out bytes.Buffer
	err := run([]string{path}, &out)
	if err == nil {
		t.Fatal("a positional argument was accepted; the command reported success without " +
			"comparing anything")
	}
	if out.Len() != 0 {
		t.Errorf("standard output carries %q; the command printed the manifest and would have "+
			"exited zero for a gate that meant to verify it", out.String())
	}

	// With no arguments at all, printing to standard output is the intended behaviour.
	out.Reset()
	if err := run(nil, &out); err != nil {
		t.Fatalf("printing the manifest with no arguments: %v", err)
	}
	if out.Len() == 0 {
		t.Error("no arguments printed nothing")
	}
}
