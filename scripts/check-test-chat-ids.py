#!/usr/bin/env python3
"""Reject Telegram supergroup IDs outside the reserved synthetic prefix block."""

import re
import sys
from pathlib import Path

RESERVED_PREFIX = b"-1009"
CHAT_ID = re.compile(rb"(?<![0-9])-100[0-9]{9,10}(?![0-9])")


def is_test_asset(path: Path) -> bool:
    return path.name.endswith("_test.go") or "testdata" in path.parts


def display_path(path: Path) -> str:
    try:
        return str(path.relative_to(Path.cwd()))
    except ValueError:
        return str(path)


def collect_assets(targets: list[str]) -> tuple[list[Path], list[str]]:
    assets: set[Path] = set()
    failures: list[str] = []
    for raw_target in targets:
        target = Path(raw_target)
        if not target.exists():
            failures.append(f"target does not exist: {raw_target}")
            continue
        candidates = [target] if target.is_file() else target.rglob("*")
        for candidate in candidates:
            if candidate.is_file() and is_test_asset(candidate):
                assets.add(candidate.resolve())
    return sorted(assets), failures


def main(argv: list[str]) -> int:
    if not argv:
        print("FAIL check-test-chat-ids: provide at least one test root")
        return 1

    assets, failures = collect_assets(argv)
    if not assets:
        failures.append("coverage is zero: no Go test files or testdata assets were found")

    identifiers = 0
    for path in assets:
        try:
            data = path.read_bytes()
        except OSError as error:
            failures.append(f"cannot read {display_path(path)}: {error}")
            continue
        for match in CHAT_ID.finditer(data):
            identifiers += 1
            raw_value = match.group()
            if raw_value.startswith(RESERVED_PREFIX):
                continue
            line = data.count(b"\n", 0, match.start()) + 1
            failures.append(
                f"{display_path(path)}:{line}: {raw_value.decode()} is outside the reserved "
                f"{RESERVED_PREFIX.decode()} prefix block"
            )

    if identifiers == 0:
        failures.append("coverage is zero: no Telegram supergroup identifier was found")
    if failures:
        print("FAIL check-test-chat-ids:")
        for failure in failures:
            print(f"  {failure}")
        return 1

    print(
        "check-test-chat-ids: passed; "
        f"{identifiers} identifiers in {len(assets)} test assets use the reserved prefix block"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
