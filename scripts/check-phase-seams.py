#!/usr/bin/env python3
"""Hold the seams between phases that the frontend is able to cross early.

A screen built against fixtures can grow a control for an endpoint that belongs
to a later phase. It happened once: the queue screen carries a 撤销 button on
every banned row, and the endpoints it would need — GET .../audit and
POST .../audit/{aid}/undo — belong to the audit screen in phase seven. An agent
sent to wire the frontend stopped and reported it, which cost a round.

Nothing in web/ names those endpoints today. This freezes that while it is true,
so the next slice cannot invent them instead of stopping.
"""
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB = ROOT / "web"
SOURCE_SUFFIXES = {".ts", ".tsx"}

# The audit screen is phase seven's. Until its own feature directory exists,
# no part of the console may reach for the endpoints it owns.
LATER_PHASE_ENDPOINT = "/audit"
OWNING_FEATURE = "features/audit"

failures: list[str] = []


def sources() -> list[Path]:
    if not WEB.is_dir():
        failures.append("web/ is missing, so this check examined nothing")
        return []
    found = [path for directory in ("src", "e2e")
             for path in (WEB / directory).rglob("*")
             if path.suffix in SOURCE_SUFFIXES and path.is_file()]
    if not found:
        failures.append("no TypeScript sources under web/src or web/e2e — "
                        "this check would pass on an empty tree")
    return found


def main() -> int:
    files = sources()
    for path in files:
        if OWNING_FEATURE in path.as_posix():
            continue
        text = path.read_text(encoding="utf-8")
        if LATER_PHASE_ENDPOINT in text:
            failures.append(
                "%s names %s, which belongs to the audit screen in phase seven; "
                "stop and say so rather than inventing the endpoint"
                % (path.relative_to(ROOT), LATER_PHASE_ENDPOINT))
    if failures:
        for failure in failures:
            print("FAIL check-phase-seams: " + failure, file=sys.stderr)
        return 1
    print("check-phase-seams: passed; %d sources, none reaches for %s"
          % (len(files), LATER_PHASE_ENDPOINT))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
