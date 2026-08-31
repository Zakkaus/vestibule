//go:build !cgo

package database

import (
	"context"
	"database/sql"

	"modernc.org/sqlite"
)

func init() {
	driver := &sqlite.Driver{}
	driver.RegisterConnectionHook(setSQLitePragmas)
	sql.Register(DefaultType, driver)
}

func setSQLitePragmas(conn sqlite.ExecQuerierContext, _ string) error {
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := conn.ExecContext(context.Background(), pragma, nil); err != nil {
			return err
		}
	}
	return nil
}
