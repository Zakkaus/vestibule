#!/usr/bin/env python3
"""Reject a Telegram identity compiled into shipped code.

This repository is a general-purpose deployment: every instance runs its own
bot, in its own groups, for its own community. A handle written into the bundle
or the binary names somebody else's deployment, and it does so in the copy a
visitor reads before anything else exists. One shipped for a while: the entry
screen told every operator of every instance to open `@gentoo_zh_verify_bot`.

Fixtures and tests may name a bot, because they are demonstrating a rendering
rather than telling a person where to go. Two Telegram service accounts are
universal and stay allowed.
"""

from __future__ import annotations

import pathlib
import re
import sys

HANDLE = re.compile(r"@([A-Za-z][A-Za-z0-9_]{3,30}[Bb]ot)\b")
UNIVERSAL = {"BotFather", "botfather"}
SEARCHED = (
    ("web/src", (".ts", ".tsx", ".json")),
    ("internal", (".go", ".json")),
    ("cmd", (".go",)),
)
EXEMPT_SUFFIXES = (".spec.ts", "_test.go", ".test.ts", ".test.tsx")
EXEMPT_NAMES = {"fixtures.ts"}


def searched_files(root: pathlib.Path) -> list[pathlib.Path]:
    found: list[pathlib.Path] = []
    for directory, suffixes in SEARCHED:
        base = root / directory
        if not base.is_dir():
            print(f"FAIL check-no-baked-identity: {directory} is missing, so nothing was searched")
            raise SystemExit(1)
        for path in sorted(base.rglob("*")):
            if not path.is_file() or path.suffix not in suffixes:
                continue
            if path.name in EXEMPT_NAMES or path.name.endswith(EXEMPT_SUFFIXES):
                continue
            found.append(path)
    return found


def main() -> int:
    root = pathlib.Path(__file__).resolve().parent.parent
    files = searched_files(root)
    if not files:
        print("FAIL check-no-baked-identity: no file matched the search, so nothing was checked")
        return 1

    problems = []
    for path in files:
        text = path.read_text(encoding="utf-8", errors="replace")
        for number, line in enumerate(text.splitlines(), start=1):
            for handle in HANDLE.findall(line):
                if handle in UNIVERSAL:
                    continue
                problems.append(
                    f"FAIL check-no-baked-identity: {path.relative_to(root)}:{number} names "
                    f"@{handle}; every instance runs its own bot, so this handle has to come "
                    f"from the instance rather than from the build"
                )

    for problem in problems:
        print(problem)
    if problems:
        return 1
    print(f"check-no-baked-identity: passed; {len(files)} shipped files, no deployment's bot handle among them")
    return 0


if __name__ == "__main__":
    sys.exit(main())
