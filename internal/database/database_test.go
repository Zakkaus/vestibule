package database

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"

	"github.com/Zakkaus/vestibule/migrations"
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

const (
	settingsMigrationExistingChatID int64 = -1009000000931
	settingsMigrationNewChatID      int64 = -1009000000932
)

func TestSettingsRevisionMigrationAllowsSchemaV1Rollback(t *testing.T) {
	ctx := context.Background()
	cfg := testSQLiteConfig(t)
	legacy := openWithUpgradeTable(t, ctx, cfg, migrations.Table[:1])
	insertSchemaV1Chat(t, ctx, legacy, settingsMigrationExistingChatID)
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	current, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	assertSchemaV1ChatDefaults(t, ctx, current.Database, settingsMigrationExistingChatID)
	if err = current.Close(); err != nil {
		t.Fatal(err)
	}

	rolledBack := openWithUpgradeTable(t, ctx, cfg, migrations.Table[:1])
	t.Cleanup(func() { _ = rolledBack.Close() })
	insertSchemaV1Chat(t, ctx, rolledBack, settingsMigrationNewChatID)
	assertSchemaV1ChatDefaults(t, ctx, rolledBack, settingsMigrationNewChatID)
}

func openWithUpgradeTable(t *testing.T, ctx context.Context, cfg Config, upgrades dbutil.UpgradeTable) *dbutil.Database {
	t.Helper()
	handle, err := dbutil.NewWithDialect(cfg.URI, cfg.Type)
	if err != nil {
		t.Fatal(err)
	}
	handle.UpgradeTable = upgrades
	if err = handle.Upgrade(ctx); err != nil {
		_ = handle.Close()
		t.Fatal(err)
	}
	return handle
}

func insertSchemaV1Chat(t *testing.T, ctx context.Context, db *dbutil.Database, chatID int64) {
	t.Helper()
	if _, err := db.Exec(ctx, "INSERT INTO chat (id, title) VALUES ($1, '') ON CONFLICT (id) DO NOTHING", chatID); err != nil {
		t.Fatal(err)
	}
}

func assertSchemaV1ChatDefaults(t *testing.T, ctx context.Context, db *dbutil.Database, chatID int64) {
	t.Helper()
	var settings string
	var revision int64
	if err := db.QueryRow(ctx, "SELECT settings, settings_revision FROM chat WHERE id=$1", chatID).Scan(&settings, &revision); err != nil {
		t.Fatal(err)
	}
	if settings != "{}" || revision != 0 {
		t.Fatalf("schema-v1 chat defaults = settings:%q revision:%d, want {} and 0", settings, revision)
	}
}

const settingsStoreTestChatID int64 = -1009000000901

func newTestSettingsStore(t *testing.T) *SettingsStore {
	t.Helper()
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSettingsStore(db)
}

func seedSettingsStore(t *testing.T, store *SettingsStore) {
	t.Helper()
	enabled := false
	if err := store.SeedSettings([]settings.Record{{
		ChatID: settingsStoreTestChatID, Revision: 4, Overrides: settings.GroupOverrides{Enabled: &enabled},
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsStoreLoadsSeededRecord(t *testing.T) {
	store := newTestSettingsStore(t)
	seedSettingsStore(t, store)
	records, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ChatID != settingsStoreTestChatID || records[0].Revision != 4 ||
		records[0].Overrides.Enabled == nil || *records[0].Overrides.Enabled {
		t.Fatalf("seeded records = %+v", records)
	}
}

func TestSettingsStoreCompareAndSwapWritesNextRevision(t *testing.T) {
	store := newTestSettingsStore(t)
	seedSettingsStore(t, store)
	nextEnabled := true
	actual, written, err := store.CompareAndSwapSettings(
		settingsStoreTestChatID, 4, settings.GroupOverrides{Enabled: &nextEnabled})
	if err != nil || !written || actual != 5 {
		t.Fatalf("CompareAndSwapSettings success = actual:%d written:%v error:%v", actual, written, err)
	}
}

func TestSettingsStoreCompareAndSwapRejectsStaleRevision(t *testing.T) {
	store := newTestSettingsStore(t)
	seedSettingsStore(t, store)
	nextEnabled := true
	_, _, err := store.CompareAndSwapSettings(settingsStoreTestChatID, 4, settings.GroupOverrides{Enabled: &nextEnabled})
	if err != nil {
		t.Fatal(err)
	}
	enabled := false
	actual, written, err := store.CompareAndSwapSettings(
		settingsStoreTestChatID, 4, settings.GroupOverrides{Enabled: &enabled})
	if err != nil || written || actual != 5 {
		t.Fatalf("stale CompareAndSwapSettings = actual:%d written:%v error:%v", actual, written, err)
	}
}

func TestSettingsStoreCompareAndSwapPersistsRecord(t *testing.T) {
	store := newTestSettingsStore(t)
	seedSettingsStore(t, store)
	nextEnabled := true
	_, _, err := store.CompareAndSwapSettings(settingsStoreTestChatID, 4, settings.GroupOverrides{Enabled: &nextEnabled})
	if err != nil {
		t.Fatal(err)
	}
	records, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Revision != 5 ||
		records[0].Overrides.Enabled == nil || !*records[0].Overrides.Enabled {
		t.Fatalf("committed records = %+v", records)
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
	if _, err = db.Exec(ctx, "UPDATE version SET version=$1, compat=$2", 3, 3); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(ctx, cfg)
	if !errors.Is(err, dbutil.ErrUnsupportedDatabaseVersion) {
		t.Fatalf("newer schema error = %v, want %v", err, dbutil.ErrUnsupportedDatabaseVersion)
	}
	if !strings.Contains(err.Error(), "currently on v3") || !strings.Contains(err.Error(), "latest known: v2") {
		t.Fatalf("newer schema error is not actionable: %v", err)
	}
	t.Logf("startup rejected: %v", err)
}
