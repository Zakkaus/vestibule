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
