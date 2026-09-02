package migrations

import (
	"fmt"
	"strconv"
	"strings"

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

// ParseSchemaManifest reads the strict two-field release metadata format.
func ParseSchemaManifest(data []byte) (SchemaManifest, error) {
	values := make(map[string]int, 2)
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, raw, ok := strings.Cut(line, "=")
		if !ok || raw == "" {
			return SchemaManifest{}, fmt.Errorf("schema manifest: malformed line %q", line)
		}
		if _, exists := values[key]; exists {
			return SchemaManifest{}, fmt.Errorf("schema manifest: repeated field %q", key)
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			return SchemaManifest{}, fmt.Errorf("schema manifest: %s must be a positive integer", key)
		}
		switch key {
		case "target_schema_version", "minimum_rollback_schema_version":
			values[key] = value
		default:
			return SchemaManifest{}, fmt.Errorf("schema manifest: unknown field %q", key)
		}
	}
	manifest := SchemaManifest{
		TargetSchemaVersion:          values["target_schema_version"],
		MinimumRollbackSchemaVersion: values["minimum_rollback_schema_version"],
	}
	if manifest.TargetSchemaVersion == 0 || manifest.MinimumRollbackSchemaVersion == 0 {
		return SchemaManifest{}, fmt.Errorf("schema manifest: both fields are required")
	}
	if manifest.MinimumRollbackSchemaVersion > manifest.TargetSchemaVersion {
		return SchemaManifest{}, fmt.Errorf(
			"schema manifest: rollback floor v%d exceeds target v%d",
			manifest.MinimumRollbackSchemaVersion,
			manifest.TargetSchemaVersion,
		)
	}
	return manifest, nil
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
