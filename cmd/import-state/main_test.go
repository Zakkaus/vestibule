package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// This command runs once per installation, against the only copy of the previous generation's
// state, and it overwrites five live tables. Everything it refuses to do is a guard on that one
// irreversible moment, and none of it was exercised: internal/database has tests for
// ImportLegacyState, and the command around it — the flags, the refusals, the exit status and the
// report the operator reads — had none.

var (
	buildOnce   sync.Once
	builtBinary string
	buildErr    error
	buildOutput []byte
)

// The command is built once: every test here needs the real process, because the exit status and
// what reaches standard output are the parts an operator and a deployment script actually see.
func importStateBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		var directory string
		if directory, buildErr = os.MkdirTemp("", "vestibule-import-state"); buildErr != nil {
			return
		}
		builtBinary = filepath.Join(directory, "import-state")
		build := exec.Command("go", "build", "-o", builtBinary, ".")
		buildOutput, buildErr = build.CombinedOutput()
	})
	if buildErr != nil {
		t.Fatalf("building cmd/import-state: %v\n%s", buildErr, buildOutput)
	}
	return builtBinary
}

type invocation struct {
	stdout string
	stderr string
	err    error
}

func importState(t *testing.T, environment []string, args ...string) invocation {
	t.Helper()
	command := exec.Command(importStateBinary(t), args...)
	// A bare environment keeps a developer's own STATE_DIRECTORY or VT_DATABASE_URI out of the
	// test: the defaults this command reads come from the environment, so inheriting one would
	// make the result depend on the machine.
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}, environment...)
	var out, errOut strings.Builder
	command.Stdout = &out
	command.Stderr = &errOut
	err := command.Run()
	return invocation{stdout: out.String(), stderr: errOut.String(), err: err}
}

// legacyState copies the five snapshots the previous generation wrote into a fresh directory, so
// each test mutates its own copy.
func legacyState(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"pending.json", "verifyfail.json", "agents.json", "heartbeat.json", "warns.json"} {
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

// The five lines the report prints are the operator's whole record of the migration: how many rows
// of each snapshot landed, and which directory now holds the only copy of the JSON that was
// replaced. Print nothing and a successful migration leaves nobody able to say where the
// pre-migration state went, so there is nothing to roll back to that anyone can find.
func TestASuccessfulImportPrintsTheBackupDirectoryAndTheValidationReport(t *testing.T) {
	directory := legacyState(t)
	backup := filepath.Join(t.TempDir(), "migration-backup")
	result := importState(t, []string{"STATE_DIRECTORY=" + directory}, "-backup-dir", backup, "-pending", "carry")
	if result.err != nil {
		t.Fatalf("import: %v\n%s%s", result.err, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stdout, "backup: "+backup) {
		t.Errorf("standard output does not name the backup directory %s; the operator has no "+
			"record of where the replaced JSON went\n%s", backup, result.stdout)
	}
	for _, line := range []string{
		"pending: rows=", "verifyfail: rows=", "agents: models=", "heartbeat: last_online=", "warns: rows=",
	} {
		if !strings.Contains(result.stdout, line) {
			t.Errorf("standard output is missing the %q line of the validation report\n%s", line, result.stdout)
		}
	}
}

// An import that failed must not read as one that succeeded. Suppress the failure and the command
// prints a clean backup line and exits zero after writing nothing or half of something; the
// operator stops the old bot and starts the new one on a database whose warning counts and
// verification-failure counts are empty, so every warned member is back to a clean slate and every
// cooldown restarts from zero.
func TestAFailedImportExitsNonZeroAndPrintsNoReport(t *testing.T) {
	directory := legacyState(t)
	if err := os.WriteFile(filepath.Join(directory, "warns.json"), []byte(`[{"group_id":`), 0o600); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "failed-backup")
	result := importState(t, []string{"STATE_DIRECTORY=" + directory}, "-backup-dir", backup, "-pending", "carry")
	if result.err == nil {
		t.Fatalf("the command reported success after an import that could not read warns.json\n%s", result.stdout)
	}
	if !strings.Contains(result.stderr, "import legacy state") {
		t.Errorf("standard error = %q, want the import named as what failed", result.stderr)
	}
	if strings.Contains(result.stdout, "verified=") {
		t.Errorf("a validation report was printed for an import that failed; it reads as proof "+
			"that a migration happened\n%s", result.stdout)
	}
}

// Without a state directory there is nothing to import, and the refusal has to come before the
// database is opened. Opening it applies the migrations and creates the file, so an operator who
// forgot the flag would get a database created and touched where they expected one line of
// refusal.
func TestNoStateDirectoryIsRefusedBeforeADatabaseIsOpened(t *testing.T) {
	databaseFile := filepath.Join(t.TempDir(), "vestibule.db")
	uri := "VT_DATABASE_URI=file:" + databaseFile + "?_txlock=immediate"

	result := importState(t, []string{"STATE_DIRECTORY=", uri}, "-pending", "carry")
	if result.err == nil {
		t.Fatal("the command ran with no state directory")
	}
	for _, name := range []string{"-state-dir", "STATE_DIRECTORY"} {
		if !strings.Contains(result.stderr, name) {
			t.Errorf("refusal = %q, want %s named so the operator knows what to supply", result.stderr, name)
		}
	}
	if _, err := os.Stat(databaseFile); err == nil {
		t.Errorf("%s exists: the command opened and migrated a database before refusing", databaseFile)
	}

	// The same invocation with a state directory has to work, or the refusal above proves only
	// that the command line was broken.
	result = importState(t, []string{"STATE_DIRECTORY=" + legacyState(t), uri},
		"-backup-dir", filepath.Join(t.TempDir(), "control-backup"), "-pending", "carry")
	if result.err != nil {
		t.Fatalf("the same call with a state directory: %v\n%s%s", result.err, result.stdout, result.stderr)
	}
}

// Production runs this command without -backup-dir. Every test of ImportLegacyState supplies one,
// so the branch the installed command actually takes had never run: the backup has to land in the
// state directory under a timestamped name, next to the JSON it is preserving.
func TestWithoutABackupDirectoryTheBackupLandsInTheStateDirectory(t *testing.T) {
	directory := legacyState(t)
	result := importState(t, []string{"STATE_DIRECTORY=" + directory}, "-pending", "carry")
	if result.err != nil {
		t.Fatalf("import with the production default: %v\n%s%s", result.err, result.stdout, result.stderr)
	}
	backups, err := filepath.Glob(filepath.Join(directory, "json-import-backup-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("the state directory holds %d timestamped backups, want exactly one: %v", len(backups), backups)
	}
	if !strings.Contains(result.stdout, "backup: "+backups[0]) {
		t.Errorf("the report names a different backup than the one on disk\nreport:\n%s\ndisk: %s",
			result.stdout, backups[0])
	}
	for _, name := range []string{"pending.json", "verifyfail.json", "agents.json", "heartbeat.json", "warns.json"} {
		if _, err := os.Stat(filepath.Join(backups[0], name)); err != nil {
			t.Errorf("default backup is missing %s: %v", name, err)
		}
	}
}

// SQLite in WAL mode has written nothing durable until the handle is closed and the journal is
// checkpointed. A command that returns with the handle still open can exit with the imported rows
// living only in the write-ahead log.
func TestTheDatabaseHandleIsClosedBeforeTheCommandReturns(t *testing.T) {
	if _, err := os.ReadDir("/proc/self/fd"); err != nil {
		t.Skipf("this test reads open descriptors from /proc: %v", err)
	}
	databaseDirectory := t.TempDir()
	databaseFile := filepath.Join(databaseDirectory, "vestibule.db")
	t.Setenv("STATE_DIRECTORY", "")
	t.Setenv("VT_DATABASE_TYPE", "")
	t.Setenv("VT_DATABASE_URI", "")

	if held := descriptorsUnder(t, databaseDirectory); len(held) != 0 {
		t.Fatalf("the database is already open before the import: %v", held)
	}
	err := run(context.Background(), []string{
		"-state-dir", legacyState(t),
		"-database-uri", "file:" + databaseFile + "?_txlock=immediate",
		"-backup-dir", filepath.Join(t.TempDir(), "backup"),
		"-pending", "carry",
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if held := descriptorsUnder(t, databaseDirectory); len(held) != 0 {
		t.Errorf("the command returned holding %v open; a WAL import can return before its "+
			"journal is checkpointed, so the process exits with the write still only in the log", held)
	}
}

func descriptorsUnder(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	var held []string
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, directory+string(os.PathSeparator)) {
			held = append(held, target)
		}
	}
	return held
}
