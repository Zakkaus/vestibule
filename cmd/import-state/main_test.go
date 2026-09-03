package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"modernc.org/sqlite"
)

const closeFailureDriverName = "sqlite-import-state-close-failure-test"

var (
	errForcedDatabaseClose = errors.New("forced database close failure")
	closeAttempts          atomic.Int32
)

func init() {
	sql.Register(closeFailureDriverName, closeFailingDriver{Driver: &sqlite.Driver{}})
}

type closeFailingDriver struct {
	driver.Driver
}

func (d closeFailingDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return closeFailingConn{Conn: conn}, nil
}

type closeFailingConn struct {
	driver.Conn
}

func (c closeFailingConn) Close() error {
	closeAttempts.Add(1)
	if err := c.Conn.Close(); err != nil {
		return err
	}
	return errForcedDatabaseClose
}

var legacyStateFiles = []string{
	"pending.json",
	"verifyfail.json",
	"agents.json",
	"heartbeat.json",
	"warns.json",
}

func buildImportState(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "import-state")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building cmd/import-state: %v\n%s", err, out)
	}
	return binary
}

func copyLegacyState(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range legacyStateFiles {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestSuccessfulImportPrintsItsBackupAndValidationReport(t *testing.T) {
	stateDirectory := copyLegacyState(t)
	backupDirectory := filepath.Join(t.TempDir(), "backup")
	output, err := exec.Command(
		buildImportState(t),
		"-state-dir", stateDirectory,
		"-database-type", database.DefaultType,
		"-backup-dir", backupDirectory,
		"-pending", "carry",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("successful import exited %v: %s", err, output)
	}

	want := fmt.Sprintf("backup: %s\n"+
		"pending: rows=2; verified=group_id,user_id,nonce,deadline,mode,all_payload_fields\n"+
		"verifyfail: rows=2; verified=group_id,user_id,count,last\n"+
		"agents: models=3 total=6; verified=model,count,total\n"+
		"heartbeat: last_online=1787574896; verified=last_online\n"+
		"warns: rows=3; verified=group_id,user_id,count\n", backupDirectory)
	if string(output) != want {
		t.Fatalf("successful import left the operator without its backup location and five-line validation report\nstdout:\n%s\nwant:\n%s", output, want)
	}
}

func TestImportWithoutBackupDirectoryCreatesTimestampedBackupInStateDirectory(t *testing.T) {
	stateDirectory := copyLegacyState(t)
	output, err := exec.Command(
		buildImportState(t),
		"-state-dir", stateDirectory,
		"-database-type", database.DefaultType,
		"-pending", "drop",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("import without -backup-dir failed before preserving the legacy JSON: %v\n%s", err, output)
	}

	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 6 || !strings.HasPrefix(lines[0], "backup: ") {
		t.Fatalf("import without -backup-dir did not identify its backup directory: %q", output)
	}
	backupDirectory := strings.TrimPrefix(lines[0], "backup: ")
	if filepath.Dir(backupDirectory) != stateDirectory {
		t.Fatalf("import without -backup-dir saved recovery JSON outside the state directory: %s", backupDirectory)
	}
	stamp := strings.TrimPrefix(filepath.Base(backupDirectory), "json-import-backup-")
	if stamp == filepath.Base(backupDirectory) {
		t.Fatalf("import without -backup-dir did not use the json-import-backup timestamp name: %s", backupDirectory)
	}
	if _, err = time.Parse("20060102T150405.000000000Z", stamp); err != nil {
		t.Fatalf("import without -backup-dir did not use a UTC backup timestamp: %s", backupDirectory)
	}
	for _, name := range legacyStateFiles {
		if _, err = os.Stat(filepath.Join(backupDirectory, name)); err != nil {
			t.Fatalf("import without -backup-dir did not preserve %s in the recovery backup: %v", name, err)
		}
	}
}

func TestSuccessfulImportReportsDatabaseCloseFailure(t *testing.T) {
	validStateDirectory := copyLegacyState(t)
	if err := run(context.Background(), []string{
		"-state-dir", validStateDirectory,
		"-database-type", database.DefaultType,
		"-backup-dir", filepath.Join(t.TempDir(), "backup"),
		"-pending", "carry",
	}); err != nil {
		t.Fatalf("valid import rejected before database close: %v", err)
	}

	closeAttempts.Store(0)
	err := run(context.Background(), []string{
		"-state-dir", copyLegacyState(t),
		"-database-type", closeFailureDriverName,
		"-backup-dir", filepath.Join(t.TempDir(), "backup"),
		"-pending", "carry",
	})
	if !errors.Is(err, errForcedDatabaseClose) {
		t.Fatalf("import hid a database close failure that can leave the SQLite WAL uncheckpointed: %v", err)
	}
	if closeAttempts.Load() == 0 {
		t.Fatal("import returned without closing the database that holds the SQLite WAL")
	}
}
