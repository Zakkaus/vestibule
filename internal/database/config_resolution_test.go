package database

import (
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/util/dbutil"
)

// The documented way to point the bot at PostgreSQL is to set the URI and nothing else, so
// the scheme is what picks the dialect. Nothing held that: with the prefix test gone the
// resolver hands back the SQLite dialect and the DSN as a filename, and the bot opens and
// migrates an empty SQLite file named after the connection string. It then starts and reports
// itself healthy with no groups, no settings and no open challenges -- every applicant
// waiting in the real database is simply not there.
func TestAPostgresURIPicksThePostgresDialectWithNoExplicitType(t *testing.T) {
	stateDirectory := t.TempDir()
	for _, uri := range []string{
		"postgres://vestibule@localhost/vestibule?sslmode=disable",
		"postgresql://vestibule@localhost/vestibule?sslmode=disable",
		"POSTGRES://vestibule@localhost/vestibule?sslmode=disable",
	} {
		databaseType, resolved, err := resolveConfig(Config{URI: uri, StateDirectory: stateDirectory})
		if err != nil {
			t.Fatalf("resolve %q: %v", uri, err)
		}
		dialect, err := dbutil.ParseDialect(databaseType)
		if err != nil {
			t.Fatalf("resolve %q gave an unparseable type %q: %v", uri, databaseType, err)
		}
		if dialect != dbutil.Postgres {
			t.Errorf("%q resolved to the %s dialect (type %q); the bot would open an empty "+
				"SQLite file named after the connection string and start with no state at all",
				uri, dialect, databaseType)
		}
		if resolved != uri {
			t.Errorf("%q resolved to URI %q", uri, resolved)
		}
	}

	// The positive control: nothing about the resolver forces PostgreSQL. With no URI it
	// still resolves to the default SQLite file in the state directory.
	databaseType, resolved, err := resolveConfig(Config{StateDirectory: stateDirectory})
	if err != nil {
		t.Fatal(err)
	}
	dialect, err := dbutil.ParseDialect(databaseType)
	if err != nil {
		t.Fatal(err)
	}
	if dialect != dbutil.SQLite || resolved != sqliteFileURI(filepath.Join(stateDirectory, DefaultFilename)) {
		t.Fatalf("empty URI resolved to %s %q, want the default SQLite file", dialect, resolved)
	}
	if _, err = os.Stat(stateDirectory); err != nil {
		t.Fatal(err)
	}
}
