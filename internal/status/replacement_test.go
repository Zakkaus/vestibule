package status

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplacementWritesOnlyVersionIntentWhenUnitExists(t *testing.T) {
	stateDirectory := t.TempDir()
	writeReplacementFile(t, stateDirectory, replacementUnitFile, "available=yes\n")
	replacement := NewReplacement(stateDirectory)

	if err := replacement.Request("v2.0.0"); err != nil {
		t.Fatal(err)
	}
	request, err := os.ReadFile(filepath.Join(stateDirectory, replacementRequestFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(request) != "v2.0.0\n" {
		t.Fatalf("replacement request = %q, want only target version", request)
	}
	info, err := os.Stat(filepath.Join(stateDirectory, replacementRequestFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("replacement request mode = %o, want 600", info.Mode().Perm())
	}
}

func TestReplacementRejectsAddressAndUnavailableUnit(t *testing.T) {
	stateDirectory := t.TempDir()
	replacement := NewReplacement(stateDirectory)

	if err := replacement.Request("v2.0.0"); !errors.Is(err, ErrReplacementUnavailable) {
		t.Fatalf("unavailable request error = %v, want ErrReplacementUnavailable", err)
	}
	writeReplacementFile(t, stateDirectory, replacementUnitFile, "available=yes\n")
	for _, version := range []string{
		"https://attacker.example/vestibule",
		"v2.0.0\nhttps://attacker.example/vestibule",
		"version=v2.0.0",
	} {
		if err := replacement.Request(version); !errors.Is(err, ErrInvalidReplacementVersion) {
			t.Fatalf("request %q error = %v, want ErrInvalidReplacementVersion", version, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDirectory, replacementRequestFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid request created %s: %v", replacementRequestFile, err)
	}
}

func TestReplacementStatusReadsHostMarkerAndResult(t *testing.T) {
	stateDirectory := t.TempDir()
	writeReplacementFile(t, stateDirectory, replacementUnitFile, "available=yes\n")
	writeReplacementFile(t, stateDirectory, replacementResultFile, "status=rolled_back\nrequested_version=v2.0.0\nreason=healthcheck_failed\n")

	got := NewReplacement(stateDirectory).Status()
	if !got.UnitAvailable || got.LastResult == nil || got.LastResult.Status != "rolled_back" ||
		got.LastResult.RequestedVersion != "v2.0.0" || got.LastResult.Reason != "healthcheck_failed" {
		t.Fatalf("replacement status = %+v, want available rolled-back result", got)
	}
}

func TestReplacementStatusHidesMalformedHostResult(t *testing.T) {
	stateDirectory := t.TempDir()
	writeReplacementFile(t, stateDirectory, replacementUnitFile, "available=yes\n")
	writeReplacementFile(t, stateDirectory, replacementResultFile, "status=applied\nrequested_version=v2.0.0\nreason=complete\nsource=https://attacker.example\n")

	got := NewReplacement(stateDirectory).Status()
	if !got.UnitAvailable || got.LastResult != nil {
		t.Fatalf("replacement status = %+v, want available without malformed result", got)
	}
}

func writeReplacementFile(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
