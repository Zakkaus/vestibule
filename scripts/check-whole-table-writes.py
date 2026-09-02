#!/usr/bin/env python3
"""A write that clears a per-group table says what stops it running blind.

Three writers replace a whole table: they delete every row and insert the set
the process is holding. That is correct while the process holds the whole set
and catastrophic when it does not — a load that failed and degraded to empty
would, on the next snapshot, erase every group's rows. The plan records that
exact defect from the previous generation.

All three are guarded today, in two different spellings: a loadErr the caller
checks, and a writable flag cleared when the load fails. The risk is not those
three; it is the fourth, added later, by someone who did not know the pattern
had a guard.

So each whole-table write is listed here with the guard that protects it. A new
one fails until its guard is written down, which is the point: the sentence is
cheap and the omission is not.

Usage: check-whole-table-writes.py [database-directory]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
# A delete with no chat_id predicate takes every group's rows with it.
WHOLE_TABLE = re.compile(r'"(DELETE FROM ([a-z_]+)(?![^"]*chat_id)[^"]*)"')
PER_GROUP_TABLE = re.compile(r"CREATE TABLE (\w+) \((.*?)\n\);", re.S)

GUARDED = {
    "replacePending":
        "runs only inside the one-time legacy import transaction, which holds "
        "the whole set by construction",
    "replaceFailures":
        "reached only when vfailWritable is true; the flag is cleared where the "
        "load fails, in internal/verification/state.go",
    "replaceWarnings":
        "reached only through warningState.save, which refuses while loadErr is "
        "set, in internal/moderate/state.go",
}
FUNCTION = re.compile(r"^func (?:\([^)]*\) )?([A-Za-z][A-Za-z0-9]*)\(", re.M)


def per_group_tables(sql: str) -> set:
    return {name for name, body in PER_GROUP_TABLE.findall(sql) if "chat_id" in body}


def enclosing_functions(text: str) -> list:
    return [(m.start(), m.group(1)) for m in FUNCTION.finditer(text)]


def main() -> int:
    package = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "internal/database")
    migration = ROOT / "migrations/00-latest.sql"
    if not package.is_dir():
        print("FAIL check-whole-table-writes: %s is not a directory" % package)
        return 1
    if not migration.is_file():
        print("FAIL check-whole-table-writes: %s is missing, so no table is known"
              % migration)
        return 1

    tables = per_group_tables(migration.read_text(encoding="utf-8"))
    if not tables:
        print("FAIL check-whole-table-writes: no table carries a chat_id, which "
              "cannot be right — has the column naming changed?")
        return 1

    scanned = 0
    found = 0
    failures = []
    for path in sorted(package.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        scanned += 1
        text = path.read_text(encoding="utf-8")
        functions = enclosing_functions(text)
        for match in WHOLE_TABLE.finditer(text):
            statement, table = match.group(1), match.group(2)
            if table not in tables:
                continue
            found += 1
            owner = "(top level)"
            for start, name in functions:
                if start <= match.start():
                    owner = name
                else:
                    break
            if owner in GUARDED:
                continue
            failures.append("%s: %s in %s clears every group's rows and no guard "
                            "is recorded for it"
                            % (path.relative_to(ROOT), statement, owner))

    if scanned == 0:
        print("FAIL check-whole-table-writes: no Go source was read from %s" % package)
        return 1
    if found == 0:
        print("FAIL check-whole-table-writes: no whole-table write was found — has "
              "the statement shape changed?")
        return 1

    if failures:
        print("FAIL check-whole-table-writes: a write could erase groups it never read")
        for failure in failures:
            print("  " + failure)
        print("\nScope the delete to one chat, or record the guard in GUARDED.")
        return 1

    print("check-whole-table-writes: passed; %d whole-table writes across %d files, "
          "each with its guard recorded" % (found, scanned))
    return 0


if __name__ == "__main__":
    sys.exit(main())
