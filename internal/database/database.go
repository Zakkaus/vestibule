// Package database owns database setup, repositories, and legacy state import.
package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zakkaus/vestibule/migrations"
	"go.mau.fi/util/dbutil"

	_ "github.com/lib/pq"
	_ "go.mau.fi/util/dbutil/litestream"
)

const (
	DefaultType     = "sqlite3-fk-wal"
	DefaultFilename = "vestibule.db"
)

var snapshotWriteMu sync.Mutex

// Config selects a database. An empty URI uses one SQLite file in StateDirectory.
type Config struct {
	Type           string
	URI            string
	StateDirectory string
}

// Database is the migrated dbutil handle shared by repositories.
type Database struct {
	*dbutil.Database
}

// Open resolves the default SQLite location, opens the database, and applies embedded migrations.
func Open(ctx context.Context, cfg Config) (*Database, error) {
	databaseType, uri, err := resolveConfig(cfg)
	if err != nil {
		return nil, err
	}
	handle, err := dbutil.NewWithDialect(uri, databaseType)
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", databaseType, err)
	}
	handle.UpgradeTable = migrations.Table
	if handle.Dialect == dbutil.SQLite {
		handle.RawDB.SetMaxOpenConns(1)
		handle.RawDB.SetMaxIdleConns(1)
	}
	if err = handle.Upgrade(ctx); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("upgrade database schema: %w", err)
	}
	return &Database{Database: handle}, nil
}

func resolveConfig(cfg Config) (databaseType, uri string, err error) {
	databaseType = strings.TrimSpace(cfg.Type)
	uri = strings.TrimSpace(cfg.URI)
	if databaseType == "" {
		if strings.HasPrefix(strings.ToLower(uri), "postgres") {
			databaseType = "postgres"
		} else {
			databaseType = DefaultType
		}
	}
	dialect, err := dbutil.ParseDialect(databaseType)
	if err != nil {
		return "", "", fmt.Errorf("database type: %w", err)
	}
	if uri != "" {
		return databaseType, uri, nil
	}
	if dialect == dbutil.Postgres {
		return "", "", fmt.Errorf("database URI is required for PostgreSQL")
	}
	if cfg.StateDirectory == "" {
		return databaseType, "file:vestibule-memory?mode=memory&cache=shared&_txlock=immediate", nil
	}
	if err = os.MkdirAll(cfg.StateDirectory, 0o700); err != nil {
		return "", "", fmt.Errorf("create database directory: %w", err)
	}
	return databaseType, sqliteFileURI(filepath.Join(cfg.StateDirectory, DefaultFilename)), nil
}

func sqliteFileURI(path string) string {
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	query.Set("_txlock", "immediate")
	uri.RawQuery = query.Encode()
	return uri.String()
}
