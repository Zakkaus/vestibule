package migrations

import (
	"fmt"
	"testing"
	"testing/fstest"

	"go.mau.fi/util/dbutil"
)

func TestSchemaManifestRegeneratesForAddedMigration(t *testing.T) {
	initialUpgrades := fstest.MapFS{
		"00-initial.sql": &fstest.MapFile{Data: []byte("-- v0 -> v1: Initial schema [incompatible: test fixture]\nCREATE TABLE initial (id INTEGER);\n")},
	}
	before := schemaManifestForTest(t, initialUpgrades)
	assertSchemaManifest(t, "initial", before, 1, 1)

	upgradesWithAddition := fstest.MapFS{
		"00-initial.sql": initialUpgrades["00-initial.sql"],
		"01-added.sql":   &fstest.MapFile{Data: []byte("-- v1 -> v2: Add schema [incompatible: test fixture]\nALTER TABLE initial ADD COLUMN label TEXT;\n")},
	}
	after := schemaManifestForTest(t, upgradesWithAddition)
	assertSchemaManifest(t, "after adding a migration", after, 2, 2)
}

func assertSchemaManifest(t *testing.T, name string, manifest SchemaManifest, target, floor int) {
	t.Helper()
	if manifest.TargetSchemaVersion != target {
		t.Errorf("%s target schema = v%d, want v%d", name, manifest.TargetSchemaVersion, target)
	}
	if manifest.MinimumRollbackSchemaVersion != floor {
		t.Errorf("%s minimum rollback schema = v%d, want v%d", name, manifest.MinimumRollbackSchemaVersion, floor)
	}
	wantText := fmt.Sprintf(
		"target_schema_version=%d\nminimum_rollback_schema_version=%d\n",
		target,
		floor,
	)
	if got := manifest.String(); got != wantText {
		t.Errorf("%s manifest = %q, want %q", name, got, wantText)
	}
}

func schemaManifestForTest(t *testing.T, upgrades fstest.MapFS) SchemaManifest {
	t.Helper()
	table := dbutil.BuildUpgradeTable().WithFS(upgrades).Finish()
	manifest, err := schemaManifestFor(table, buildCompatibilityHistory(upgrades))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestParseSchemaManifestAcceptsOnlyCompleteReleaseMetadata(t *testing.T) {
	parsed, err := ParseSchemaManifest([]byte(
		"target_schema_version=3\nminimum_rollback_schema_version=2\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.TargetSchemaVersion != 3 || parsed.MinimumRollbackSchemaVersion != 2 {
		t.Fatalf("parsed manifest = %#v, want target v3 and rollback floor v2", parsed)
	}

	for name, data := range map[string]string{
		"missing floor":  "target_schema_version=3\n",
		"repeated field": "target_schema_version=3\ntarget_schema_version=2\nminimum_rollback_schema_version=1\n",
		"unknown field":  "target_schema_version=3\nminimum_rollback_schema_version=1\nsource=other\n",
		"non-numeric":    "target_schema_version=next\nminimum_rollback_schema_version=1\n",
		"floor too new":  "target_schema_version=2\nminimum_rollback_schema_version=3\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, parseErr := ParseSchemaManifest([]byte(data)); parseErr == nil {
				t.Fatalf("ParseSchemaManifest(%q) succeeded, want refusal", data)
			}
		})
	}
}
