package migrations

import (
	"fmt"

	"go.mau.fi/util/dbutil"
)

// SchemaManifest is the release metadata an installer reads before downloading a binary.
type SchemaManifest struct {
	TargetSchemaVersion          int
	MinimumRollbackSchemaVersion int
}

// CurrentSchemaManifest derives the release metadata from this binary's migration table.
func CurrentSchemaManifest() (SchemaManifest, error) {
	return schemaManifestFor(Table, rollbackHistory)
}

// String returns a stable POSIX-shell-readable manifest.
func (manifest SchemaManifest) String() string {
	return fmt.Sprintf(
		"target_schema_version=%d\nminimum_rollback_schema_version=%d\n",
		manifest.TargetSchemaVersion,
		manifest.MinimumRollbackSchemaVersion,
	)
}

func schemaManifestFor(table dbutil.UpgradeTable, history compatibilityHistory) (SchemaManifest, error) {
	target := len(table)
	if target == 0 {
		return SchemaManifest{}, fmt.Errorf("schema manifest: migrations.Table has no upgrades")
	}
	floor, known := history.floor(target)
	if !known {
		return SchemaManifest{}, fmt.Errorf(
			"schema manifest: migrations.Table targets v%d, but no rollback declaration exists",
			target,
		)
	}
	return SchemaManifest{
		TargetSchemaVersion:          target,
		MinimumRollbackSchemaVersion: floor,
	}, nil
}
