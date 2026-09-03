package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/migrations"
)

func buildSchemaManifest(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "schema-manifest")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building cmd/schema-manifest: %v\n%s", err, out)
	}
	return binary
}

func TestCheckRefusesAManifestThatDiffersFromMigrationTable(t *testing.T) {
	manifest, err := migrations.CurrentSchemaManifest()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vestibule-schema-manifest")
	if err = os.WriteFile(path, []byte(manifest.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := buildSchemaManifest(t)
	if output, checkErr := exec.Command(binary, "-check", path).CombinedOutput(); checkErr != nil {
		t.Fatalf("checking the manifest derived from migrations.Table rejected valid release metadata: %v\n%s", checkErr, output)
	}

	if err = os.WriteFile(path, append([]byte(manifest.String()), "stale\n"...), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(binary, "-check", path).CombinedOutput()
	if err == nil {
		t.Fatal("-check accepted stale release metadata; an operator could roll back to a binary that cannot read the database")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 {
		t.Fatalf("-check rejected stale release metadata without a non-zero exit: %v", err)
	}
	if !strings.Contains(string(output), "does not match migrations.Table") {
		t.Fatalf("-check did not explain that stale release metadata differs from migrations.Table: %s", output)
	}
}
