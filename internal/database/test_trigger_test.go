package database

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"go.mau.fi/util/dbutil"
)

var testTriggerPattern = regexp.MustCompile(`(?s)^\s*CREATE TRIGGER (\w+)\s+(BEFORE|AFTER) (INSERT|DELETE) ON (\w+)(?:\s+WHEN (.*?))?\s+BEGIN\s+(.*?)\s+END\s*$`)
var testAbortPattern = regexp.MustCompile(`SELECT RAISE\(ABORT, ('[^']*')\);`)

// execTestTrigger keeps the existing fault-injection fixtures equivalent on both dialects.
func execTestTrigger(t *testing.T, ctx context.Context, db *Database, query string) (sql.Result, error) {
	t.Helper()
	if db.Dialect != dbutil.Postgres || !strings.Contains(query, "CREATE TRIGGER") {
		return db.Exec(ctx, query)
	}
	parts := testTriggerPattern.FindStringSubmatch(query)
	if parts == nil {
		t.Fatalf("unsupported test trigger: %s", query)
	}
	name, timing, event, table, condition, body := parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
	body = strings.ReplaceAll(body, "SELECT RAISE(IGNORE);", "RETURN NULL;")
	body = testAbortPattern.ReplaceAllString(body, "RAISE EXCEPTION $1;")
	if condition != "" {
		body = "IF " + condition + " THEN " + body + " END IF;"
	}
	row := "NEW"
	if event == "DELETE" {
		row = "OLD"
	}
	query = fmt.Sprintf(`CREATE FUNCTION %[1]s() RETURNS trigger LANGUAGE plpgsql AS $trigger$
		BEGIN %[2]s RETURN %[3]s; END; $trigger$;
		CREATE TRIGGER %[1]s %[4]s %[5]s ON %[6]s FOR EACH ROW EXECUTE FUNCTION %[1]s()`,
		name, body, row, timing, event, table)
	return db.Exec(ctx, query)
}
