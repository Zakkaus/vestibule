package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"strings"

	"github.com/lib/pq"
)

type testConfigOwner interface {
	Helper()
	Fatalf(string, ...any)
	Errorf(string, ...any)
	Cleanup(func())
}

// TestConfig preserves sqlite unless VESTIBULE_TEST_POSTGRES_URI selects a private PostgreSQL database.
// The DSN must allow CREATE DATABASE; reuse the returned config for multiple handles and close them in cleanup.
func TestConfig(t testConfigOwner, sqlite Config) Config {
	t.Helper()
	dsn := os.Getenv("VESTIBULE_TEST_POSTGRES_URI")
	if dsn == "" {
		return sqlite
	}
	var parsed *url.URL
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		var err error
		parsed, err = url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse PostgreSQL test URI: %v", err)
		}
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	name := "vestibule_test_" + strings.ToLower(rand.Text())
	quoted := pq.QuoteIdentifier(name)
	if _, err = admin.ExecContext(context.Background(), "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close()
		t.Fatalf("create PostgreSQL test database: %v", err)
	}
	t.Cleanup(func() {
		defer func() { _ = admin.Close() }()
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("drop PostgreSQL test database: %v", err)
		}
	})
	sqlite.Type = "postgres"
	sqlite.URI = dsn + " dbname='" + name + "'"
	if parsed != nil {
		parsed.Path, parsed.RawPath = "/"+name, ""
		query := parsed.Query()
		query.Del("dbname")
		parsed.RawQuery = query.Encode()
		sqlite.URI = parsed.String()
	}
	return sqlite
}
