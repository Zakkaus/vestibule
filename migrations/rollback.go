package migrations

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

// ErrRollbackBlocked marks a target schema that cannot safely return to the requested schema.
var ErrRollbackBlocked = errors.New("schema rollback is not safe")

// RollbackReason identifies why a schema rollback cannot proceed.
type RollbackReason string

const (
	RollbackCompatible    RollbackReason = ""
	RollbackUnknownTarget RollbackReason = "unknown_target_schema"
	RollbackNotEarlier    RollbackReason = "not_an_earlier_schema"
	RollbackIncompatible  RollbackReason = "schema_incompatible"
)

// RollbackAssessment is the migration-table result a release flow can render before retrieval.
type RollbackAssessment struct {
	TargetVersion            int
	RollbackVersion          int
	MinimumCompatibleVersion int
	Reason                   RollbackReason
}

// CanRollback reports whether the target schema permits the requested earlier binary to start.
func (assessment RollbackAssessment) CanRollback() bool {
	return assessment.Reason == RollbackCompatible
}

// Err returns a displayable explanation when the rollback is unsafe.
func (assessment RollbackAssessment) Err() error {
	if assessment.CanRollback() {
		return nil
	}
	return rollbackBlockedError{assessment: assessment}
}

type rollbackBlockedError struct {
	assessment RollbackAssessment
}

func (err rollbackBlockedError) Error() string {
	assessment := err.assessment
	switch assessment.Reason {
	case RollbackUnknownTarget:
		return fmt.Sprintf(
			"cannot assess rollback: target schema v%d is not present in this binary's migrations",
			assessment.TargetVersion,
		)
	case RollbackNotEarlier:
		return fmt.Sprintf(
			"cannot roll back from schema v%d to v%d: the requested schema is newer than the target",
			assessment.TargetVersion,
			assessment.RollbackVersion,
		)
	case RollbackIncompatible:
		return fmt.Sprintf(
			"cannot roll back from schema v%d to v%d: the target requires schema v%d or newer",
			assessment.TargetVersion,
			assessment.RollbackVersion,
			assessment.MinimumCompatibleVersion,
		)
	default:
		return "cannot assess rollback: unknown schema compatibility result"
	}
}

func (err rollbackBlockedError) Unwrap() error {
	return ErrRollbackBlocked
}

// Fetch is the retrieval action a release flow performs after its rollback preflight succeeds.
type Fetch func(context.Context) error

// FetchAfterRollbackCheck refuses an unsafe target before invoking fetch.
func FetchAfterRollbackCheck(
	ctx context.Context,
	targetVersion, rollbackVersion int,
	fetch Fetch,
) (RollbackAssessment, error) {
	assessment := AssessRollback(targetVersion, rollbackVersion)
	if err := assessment.Err(); err != nil {
		return assessment, err
	}
	if fetch == nil {
		return assessment, errors.New("migration fetch callback is nil")
	}
	return assessment, fetch(ctx)
}

// AssessRollback derives a target schema's compatibility floor from the embedded migration headers.
func AssessRollback(targetVersion, rollbackVersion int) RollbackAssessment {
	floor, known := rollbackHistory.floor(targetVersion)
	if !known {
		return RollbackAssessment{
			TargetVersion:   targetVersion,
			RollbackVersion: rollbackVersion,
			Reason:          RollbackUnknownTarget,
		}
	}
	return (SchemaManifest{
		TargetSchemaVersion:          targetVersion,
		MinimumRollbackSchemaVersion: floor,
	}).AssessRollback(rollbackVersion)
}

// AssessRollback reports whether a retained release can start after applying this manifest.
func (manifest SchemaManifest) AssessRollback(rollbackVersion int) RollbackAssessment {
	assessment := RollbackAssessment{
		TargetVersion:            manifest.TargetSchemaVersion,
		RollbackVersion:          rollbackVersion,
		MinimumCompatibleVersion: manifest.MinimumRollbackSchemaVersion,
	}
	if manifest.TargetSchemaVersion < 1 ||
		manifest.MinimumRollbackSchemaVersion < 1 ||
		manifest.MinimumRollbackSchemaVersion > manifest.TargetSchemaVersion {
		assessment.Reason = RollbackUnknownTarget
		return assessment
	}
	if rollbackVersion > manifest.TargetSchemaVersion {
		assessment.Reason = RollbackNotEarlier
		return assessment
	}
	if rollbackVersion < manifest.MinimumRollbackSchemaVersion {
		assessment.Reason = RollbackIncompatible
		return assessment
	}
	return assessment
}

type upgradeFS interface {
	fs.ReadFileFS
	fs.ReadDirFS
}

type compatibilityHistory struct {
	floors map[int]int
}

func (history compatibilityHistory) floor(version int) (int, bool) {
	floor, ok := history.floors[version]
	return floor, ok
}

var rollbackHistory = buildCompatibilityHistory(rawUpgrades)

var migrationHeader = regexp.MustCompile(`^-- (?:v\d+ -> )?v(\d+)(?: \(compatible with v(\d+)\+\))?: .+$`)

func buildCompatibilityHistory(upgrades upgradeFS) compatibilityHistory {
	entries, err := upgrades.ReadDir(".")
	if err != nil {
		panic(fmt.Errorf("read embedded migrations: %w", err))
	}
	history := compatibilityHistory{floors: make(map[int]int, len(entries))}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := upgrades.ReadFile(entry.Name())
		if err != nil {
			panic(fmt.Errorf("read embedded migration %s: %w", entry.Name(), err))
		}
		version, floor := migrationCompatibility(entry.Name(), data)
		if existing, duplicate := history.floors[version]; duplicate && existing != floor {
			panic(fmt.Errorf("migrations for v%d disagree on compatibility floor: v%d and v%d", version, existing, floor))
		}
		history.floors[version] = floor
	}
	return history
}

func migrationCompatibility(name string, data []byte) (version, floor int) {
	header, _, _ := bytes.Cut(data, []byte("\n"))
	match := migrationHeader.FindSubmatch(header)
	if match == nil {
		panic(fmt.Errorf("migration %s has no dbutil header", name))
	}
	version, err := strconv.Atoi(string(match[1]))
	if err != nil {
		panic(fmt.Errorf("migration %s target version: %w", name, err))
	}
	floor = version
	if len(match[2]) == 0 {
		return version, floor
	}
	floor, err = strconv.Atoi(string(match[2]))
	if err != nil {
		panic(fmt.Errorf("migration %s compatibility floor: %w", name, err))
	}
	return version, floor
}
