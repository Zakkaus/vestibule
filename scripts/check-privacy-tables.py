#!/usr/bin/env python3
"""Every table holding a person's identifier is named in the privacy statement.

docs/PRIVACY.md lists what the software stores about people, table by table. A
list like that is worth exactly as much as the thing keeping it true: the schema
grows, someone adds a table with a user_id in it, and the document goes on
describing the old set while reading as though it were complete.

A privacy statement that is quietly incomplete is worse than none, because
people rely on it. So the schema decides what the document must name.

A table is in scope when it has a user_id or chat_id column — that is what makes
a row about somebody rather than about the instance. Both language versions must
name it, so a translation cannot fall behind either.

The direction is one-way. The document may describe more than the schema holds:
logs and behaviour are not tables.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MIGRATION = ROOT / "migrations" / "00-latest.sql"
STATEMENTS = [ROOT / "docs" / "PRIVACY.md", ROOT / "docs" / "PRIVACY.zh-CN.md"]
IDENTIFYING = ("user_id", "chat_id")


def person_bearing_tables(sql: str) -> list:
    """A table is about a person directly, or by referencing one that is.

    The first version looked only for a user_id or chat_id column and declared
    pending_action clean. It carries challenge_id, a foreign key into challenge,
    whose primary key is the string chat:user:nonce — the two identifiers are
    inside it. A row saying what the bot is about to do to somebody is about
    that somebody, whatever the column is called.

    So references are followed. A table referencing a table in scope is in
    scope, repeatedly, until nothing new is added.
    """
    tables = {}
    for match in re.finditer(r"CREATE TABLE (\w+) \((.*?)\n\);", sql, re.S):
        name, body = match.group(1), match.group(2)
        columns, references = [], set()
        for line in body.split("\n"):
            line = line.strip()
            if not line or line.startswith("--"):
                continue
            reference = re.search(r"REFERENCES\s+(\w+)\s*\(", line)
            if reference:
                references.add(reference.group(1))
            if line.upper().startswith(("PRIMARY", "FOREIGN", "UNIQUE", "CHECK")):
                continue
            columns.append(line.split()[0].rstrip(","))
        tables[name] = (columns, references)

    found = {name for name, (columns, _) in tables.items()
             if any(column in IDENTIFYING for column in columns)}
    while True:
        grown = {name for name, (_, references) in tables.items()
                 if references & found}
        if grown <= found:
            break
        found |= grown
    return sorted(found)


def main() -> int:
    if not MIGRATION.exists():
        print("check-privacy-tables: no migration to read")
        return 0

    required = person_bearing_tables(MIGRATION.read_text(encoding="utf-8"))
    if not required:
        print("FAIL check-privacy-tables: no table carries a user_id or chat_id, "
              "which cannot be right — has the column naming changed?")
        return 1

    failed = False
    for statement in STATEMENTS:
        if not statement.exists():
            print("FAIL check-privacy-tables: %s is missing" % statement.name)
            failed = True
            continue
        text = statement.read_text(encoding="utf-8")
        for table in required:
            if re.search(r"`%s`" % re.escape(table), text):
                continue
            print("FAIL check-privacy-tables: %s holds a person's identifier and "
                  "%s does not name it" % (table, statement.name))
            failed = True

    if failed:
        print("\nA privacy statement that is quietly incomplete is worse than none.")
        print("Add the table, what it is about, and what it holds — in both languages.")
        return 1

    print("check-privacy-tables: passed; %d tables carry an identifier, "
          "both statements name all of them" % len(required))
    return 0


if __name__ == "__main__":
    sys.exit(main())
