package migrations

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"go.mau.fi/util/dbutil"
)

func TestLatestMigrationFiltersForBothDialects(t *testing.T) {
	data, err := rawUpgrades.ReadFile("00-latest.sql")
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) < 2 {
		t.Fatal("latest migration has no body")
	}
	for _, test := range []struct {
		name    string
		dialect string
	}{
		{name: "SQLite", dialect: "sqlite3"},
		{name: "PostgreSQL", dialect: "postgres"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := dbutil.NewWithDB(&sql.DB{}, test.dialect)
			if err != nil {
				t.Fatal(err)
			}
			filtered, err := db.Internals().FilterSQLUpgrade(lines[1:])
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{"CREATE TABLE chat", "CREATE TABLE challenge", "CREATE TABLE rule"} {
				if !strings.Contains(filtered, required) {
					t.Errorf("filtered migration is missing %q", required)
				}
			}
			sum := sha256.Sum256([]byte(filtered))
			t.Logf("%s static dbutil transform: bytes=%d sha256=%s", test.name, len(filtered), fmt.Sprintf("%x", sum))
		})
	}
}
