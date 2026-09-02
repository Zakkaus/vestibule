package database

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/verification"
)

var legacyJSONNames = []string{
	"pending.json",
	"verifyfail.json",
	"agents.json",
	"heartbeat.json",
	"warns.json",
}

// ImportOptions selects the legacy state directory and an optional exact backup directory.
type ImportOptions struct {
	StateDirectory  string
	BackupDirectory string
	// Pending says what to do with challenges that were open when the previous
	// generation's state was written. It has no default: the plan and the command
	// disagreed about this for several phases, and a silent answer is how that
	// happened. PendingCarry or PendingDrop, stated by whoever runs the import.
	Pending PendingDisposition
}

// PendingDisposition is what an import does with the previous generation's open challenges.
type PendingDisposition string

const (
	// PendingCarry writes them into the new database, which then holds challenges the
	// previous generation is still settling if it has not been stopped.
	PendingCarry PendingDisposition = "carry"
	// PendingDrop leaves them behind. The applicants stay with whichever bot is still
	// answering for them; nothing about them is deleted from the backup.
	PendingDrop PendingDisposition = "drop"
)

// ImportReport contains post-commit validation results.
type ImportReport struct {
	BackupDirectory string
	PendingRows     int
	FailureRows     int
	AgentModels     int
	AgentTotal      int
	LastOnline      int64
	WarningRows     int
}

func (r ImportReport) ValidationText() string {
	return fmt.Sprintf(
		"pending: rows=%d; verified=group_id,user_id,nonce,deadline,mode,all_payload_fields\n"+
			"verifyfail: rows=%d; verified=group_id,user_id,count,last\n"+
			"agents: models=%d total=%d; verified=model,count,total\n"+
			"heartbeat: last_online=%d; verified=last_online\n"+
			"warns: rows=%d; verified=group_id,user_id,count",
		r.PendingRows, r.FailureRows, r.AgentModels, r.AgentTotal, r.LastOnline, r.WarningRows,
	)
}

type legacyState struct {
	pending   []verification.PendingRecord
	failures  []verification.FailureRecord
	agents    verification.AgentTally
	heartbeat verification.HeartbeatRecord
	warnings  []moderate.WarningRecord
}

// ImportLegacyState backs up, decodes, transactionally replaces, and validates all five snapshots.
func ImportLegacyState(ctx context.Context, db *Database, options ImportOptions) (ImportReport, error) {
	if strings.TrimSpace(options.StateDirectory) == "" {
		return ImportReport{}, fmt.Errorf("state directory is required")
	}
	switch options.Pending {
	case PendingCarry, PendingDrop:
	default:
		return ImportReport{}, fmt.Errorf(
			"pending disposition is required: %q keeps the previous generation's open "+
				"challenges, %q leaves them with the bot still answering for them",
			PendingCarry, PendingDrop)
	}
	// The import deletes and rebuilds every per-group table it owns. Run against a database a
	// bot is polling for, it replaces verifications that are in flight right now with a
	// snapshot of the generation being replaced. The polling lease is what says a bot is
	// there: migration happens with the old bot stopped, so an unexpired holder means this is
	// not that moment. Repeating the import against a cold database is unaffected, which the
	// phase-ten acceptance requires.
	if holder, err := NewUpdatePollLease(db).Holder(ctx, time.Now().Unix()); err != nil {
		return ImportReport{}, err
	} else if holder != "" {
		return ImportReport{}, fmt.Errorf(
			"an instance is polling Telegram against this database (lease held by %s); "+
				"stop it before importing, or the import replaces verifications it is running",
			holder)
	}
	backupDirectory, err := backupLegacyJSON(options)
	if err != nil {
		return ImportReport{}, err
	}
	state, err := loadLegacyState(options.StateDirectory)
	if err != nil {
		return ImportReport{BackupDirectory: backupDirectory}, err
	}
	if options.Pending == PendingDrop {
		state.pending = nil
	}
	if err = persistLegacyState(ctx, db, state); err != nil {
		return ImportReport{BackupDirectory: backupDirectory}, err
	}
	report, err := validateLegacyState(db, state)
	report.BackupDirectory = backupDirectory
	return report, err
}

func backupLegacyJSON(options ImportOptions) (string, error) {
	backupDirectory := options.BackupDirectory
	if backupDirectory == "" {
		stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
		backupDirectory = filepath.Join(options.StateDirectory, "json-import-backup-"+stamp)
	}
	if err := os.MkdirAll(filepath.Dir(backupDirectory), 0o700); err != nil {
		return "", fmt.Errorf("create backup parent: %w", err)
	}
	if err := os.Mkdir(backupDirectory, 0o700); err != nil {
		return "", fmt.Errorf("create backup directory %s: %w", backupDirectory, err)
	}
	for _, name := range legacyJSONNames {
		source := filepath.Join(options.StateDirectory, name)
		data, err := os.ReadFile(source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return backupDirectory, fmt.Errorf("back up %s: %w", name, err)
		}
		if err = writeBackup(filepath.Join(backupDirectory, name), data); err != nil {
			return backupDirectory, fmt.Errorf("back up %s: %w", name, err)
		}
	}
	if err := syncDirectory(backupDirectory); err != nil {
		return backupDirectory, fmt.Errorf("sync backup directory: %w", err)
	}
	if err := syncDirectory(filepath.Dir(backupDirectory)); err != nil {
		return backupDirectory, fmt.Errorf("sync backup parent: %w", err)
	}
	return backupDirectory, nil
}

func writeBackup(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err = directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}

func loadLegacyState(directory string) (legacyState, error) {
	var state legacyState
	loads := []struct {
		name string
		dst  any
	}{
		{name: "pending.json", dst: &state.pending},
		{name: "verifyfail.json", dst: &state.failures},
		{name: "agents.json", dst: &state.agents},
		{name: "heartbeat.json", dst: &state.heartbeat},
		{name: "warns.json", dst: &state.warnings},
	}
	for _, item := range loads {
		path := filepath.Join(directory, item.name)
		if err := rejectMissingCorrupt(path); err != nil {
			return state, err
		}
		if err := store.Load(path, item.dst); err != nil {
			return state, fmt.Errorf("load %s: %w", item.name, err)
		}
	}
	return state, nil
}

func rejectMissingCorrupt(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if _, err := os.Stat(path + ".corrupt"); err == nil {
		return fmt.Errorf("%s is missing while %s.corrupt exists; repair the source before importing", filepath.Base(path), filepath.Base(path))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect %s.corrupt: %w", filepath.Base(path), err)
	}
	return nil
}

func persistLegacyState(ctx context.Context, db *Database, state legacyState) error {
	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()
	return db.DoTxn(ctx, nil, func(ctx context.Context) error {
		if err := replacePending(ctx, db, state.pending); err != nil {
			return err
		}
		if err := replaceFailures(ctx, db, state.failures); err != nil {
			return err
		}
		if err := replaceAgents(ctx, db, state.agents); err != nil {
			return err
		}
		if err := upsertRuntimeValue(ctx, db, heartbeatKey, state.heartbeat.LastOnline); err != nil {
			return err
		}
		return replaceWarnings(ctx, db, state.warnings)
	})
}

func validateLegacyState(db *Database, expected legacyState) (ImportReport, error) {
	verificationStore := NewVerificationStore(db)
	pending, err := verificationStore.LoadPending("")
	if err != nil {
		return ImportReport{}, err
	}
	failures, err := verificationStore.LoadFailures("")
	if err != nil {
		return ImportReport{}, err
	}
	agents, err := verificationStore.LoadAgents("")
	if err != nil {
		return ImportReport{}, err
	}
	heartbeat, err := verificationStore.LoadHeartbeat("")
	if err != nil {
		return ImportReport{}, err
	}
	warnings, err := NewWarningStore(db).LoadWarnings()
	if err != nil {
		return ImportReport{}, err
	}
	if err = compareLegacyState(expected, legacyState{pending, failures, agents, heartbeat, warnings}); err != nil {
		return ImportReport{}, err
	}
	return ImportReport{
		PendingRows: len(pending), FailureRows: len(failures), AgentModels: len(agents.Counts),
		AgentTotal: agents.Total, LastOnline: heartbeat.LastOnline, WarningRows: len(warnings),
	}, nil
}

func compareLegacyState(expected, actual legacyState) error {
	sortPending(expected.pending)
	sortPending(actual.pending)
	if len(expected.pending) != len(actual.pending) {
		return fmt.Errorf("validate pending records: database differs from JSON")
	}
	for index := range expected.pending {
		expectedJSON, _ := json.Marshal(expected.pending[index])
		actualJSON, _ := json.Marshal(actual.pending[index])
		if !bytes.Equal(expectedJSON, actualJSON) {
			return fmt.Errorf("validate pending records: database differs from JSON")
		}
	}
	sortFailures(expected.failures)
	sortFailures(actual.failures)
	if len(expected.failures) != len(actual.failures) ||
		len(expected.failures) > 0 && !reflect.DeepEqual(expected.failures, actual.failures) {
		return fmt.Errorf("validate verification failures: database differs from JSON")
	}
	if expected.agents.Total != actual.agents.Total || !maps.Equal(expected.agents.Counts, actual.agents.Counts) {
		return fmt.Errorf("validate agent tally: database differs from JSON")
	}
	if expected.heartbeat != actual.heartbeat {
		return fmt.Errorf("validate heartbeat: database differs from JSON")
	}
	sortWarnings(expected.warnings)
	sortWarnings(actual.warnings)
	if len(expected.warnings) != len(actual.warnings) ||
		len(expected.warnings) > 0 && !reflect.DeepEqual(expected.warnings, actual.warnings) {
		return fmt.Errorf("validate warning records: database differs from JSON")
	}
	return nil
}

func sortPending(records []verification.PendingRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].GroupID != records[j].GroupID {
			return records[i].GroupID < records[j].GroupID
		}
		return records[i].UserID < records[j].UserID
	})
}

func sortFailures(records []verification.FailureRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].GroupID != records[j].GroupID {
			return records[i].GroupID < records[j].GroupID
		}
		return records[i].UserID < records[j].UserID
	})
}

func sortWarnings(records []moderate.WarningRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].GroupID != records[j].GroupID {
			return records[i].GroupID < records[j].GroupID
		}
		return records[i].UserID < records[j].UserID
	})
}
