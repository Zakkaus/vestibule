#!/usr/bin/env python3
"""Require every SQL migration to state its rollback compatibility explicitly.

`dbutil` treats an omitted compatibility clause as compatible only with the
migration's target version. That default is safe, but silent: a later release
flow cannot distinguish an intentional incompatibility from a forgotten header.
Every migration therefore either uses dbutil's `(compatible with vN+)` clause or
adds `[incompatible: reason]` to its header message.
"""
import re
import sys
from pathlib import Path

HEADER = re.compile(r"^-- (?:v\d+ -> )?v\d+(?: \(compatible with v\d+\+\))?: .+$")
COMPATIBILITY = re.compile(r"\(compatible with v\d+\+\)")
INCOMPATIBILITY = re.compile(r"\[incompatible:\s*[^\]\s][^\]]*\]")


def declaration_error(path: Path) -> str | None:
    try:
        first_line = path.read_text(encoding="utf-8").splitlines()[0]
    except IndexError:
        return "is empty and has no dbutil migration header"

    if not HEADER.fullmatch(first_line):
        return "does not start with a dbutil migration header"
    if COMPATIBILITY.search(first_line) or INCOMPATIBILITY.search(first_line):
        return ""
    return "does not declare a compatibility floor or explain incompatibility with [incompatible: reason]"


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: check-migration-declarations.py MIGRATIONS_DIRECTORY", file=sys.stderr)
        return 2

    directory = Path(argv[1])
    if not directory.is_dir():
        print("FAIL check-migration-declarations: %s is not a migration directory" % directory)
        return 1

    migrations = sorted(path for path in directory.glob("*.sql") if path.is_file())
    if not migrations:
        print("FAIL check-migration-declarations: %s has zero SQL migrations; nothing was checked" % directory)
        return 1

    failed = False
    for migration in migrations:
        error = declaration_error(migration)
        if error:
            print("FAIL check-migration-declarations: %s %s" % (migration, error))
            failed = True

    if failed:
        print("\nState a dbutil compatibility floor, or explain why this migration cannot offer one.")
        return 1

    print("check-migration-declarations: passed; %d SQL migrations explicitly state rollback compatibility" % len(migrations))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
