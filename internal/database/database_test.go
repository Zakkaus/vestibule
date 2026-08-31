package database

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.mau.fi/util/dbutil"
)

func testSQLiteConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		Type: "sqlite3-fk-wal",
		URI:  "file:" + filepath.Join(t.TempDir(), "vestibule.db") + "?_txlock=immediate",
	}
}

func TestOpenMigratesSQLite(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, table := range []string{
		"chat", "challenge", "rule", "verification_failure", "agent_tally",
		"verification_runtime", "warning_counter",
	} {
		exists, err := db.TableExists(context.Background(), table)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("migration did not create %s", table)
		}
	}
}

func TestOpenConfiguresSQLitePragmas(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var foreignKeys, synchronous, busyTimeout int
	if err = db.RawDB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err = db.RawDB.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&synchronous); err != nil {
		t.Fatal(err)
	}
	if err = db.RawDB.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	var journalMode string
	if err = db.RawDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 || synchronous != 1 || busyTimeout != 5000 || !strings.EqualFold(journalMode, "wal") {
		t.Fatalf(
			"SQLite pragmas = foreign_keys=%d synchronous=%d busy_timeout=%d journal_mode=%q",
			foreignKeys,
			synchronous,
			busyTimeout,
			journalMode,
		)
	}

	if _, err = db.Exec(ctx, "CREATE TABLE pragma_parent (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "CREATE TABLE pragma_child (parent_id INTEGER REFERENCES pragma_parent(id))"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "INSERT INTO pragma_child (parent_id) VALUES (1)"); err == nil {
		t.Fatal("foreign key violation succeeded")
	}
}

func TestPostgresDriverIsRegistered(t *testing.T) {
	db, err := dbutil.NewWithDialect(
		"postgres://vestibule@localhost/vestibule?sslmode=disable",
		"postgres",
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	cfg := testSQLiteConfig(t)
	db, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE version SET version=$1, compat=$2", 2, 2); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, cfg)
	if !errors.Is(err, dbutil.ErrUnsupportedDatabaseVersion) {
		t.Fatalf("newer schema error = %v, want %v", err, dbutil.ErrUnsupportedDatabaseVersion)
	}
	if !strings.Contains(err.Error(), "currently on v2") || !strings.Contains(err.Error(), "latest known: v1") {
		t.Fatalf("newer schema error is not actionable: %v", err)
	}
	t.Logf("startup rejected: %v", err)
}
