#!/usr/bin/env python3
"""Every request the console makes goes through the one transport.

web/src/lib/api.ts is the only place that attaches X-CSRF-Token and sends
credentials with the request. A screen that calls fetch directly loses both: a
write is refused as csrf_invalid, or the request arrives unauthenticated, and
neither failure looks like a mistake in the screen that caused it.

Twelve feature api.ts files and one transport were the state when this was
written — one way to reach the API, no exceptions to grant. Holding that is
cheaper than finding the thirteenth.

Usage: check-one-transport.py [source-directory] [transport-file]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
# Anything that opens a connection of its own.
NETWORK = re.compile(r"\b(?:fetch\(|XMLHttpRequest|axios|EventSource\(|"
                     r"sendBeacon\(|new WebSocket\()")


def shown(path: Path) -> str:
    """A path outside the repository still has to print. relative_to raises."""
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)


def main() -> int:
    source = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "web/src")
    transport = ROOT / (sys.argv[2] if len(sys.argv) > 2 else "web/src/lib/api.ts")
    if not source.is_dir():
        print("FAIL check-one-transport: %s is not a directory, so nothing was read"
              % source)
        return 1
    if not transport.is_file():
        print("FAIL check-one-transport: %s is missing; the rule has no subject"
              % transport)
        return 1

    if not NETWORK.search(transport.read_text(encoding="utf-8")):
        print("FAIL check-one-transport: %s makes no network call, so this check "
              "is looking for the wrong shape" % shown(transport))
        return 1

    scanned = 0
    failures = []
    for path in sorted(source.rglob("*")):
        if path.suffix not in (".ts", ".tsx") or path == transport:
            continue
        scanned += 1
        for number, line in enumerate(path.read_text(encoding="utf-8").split("\n"), 1):
            match = NETWORK.search(line)
            if match:
                failures.append("%s:%d opens its own connection with %s"
                                % (shown(path), number,
                                   match.group(0).rstrip("(")))

    if scanned == 0:
        print("FAIL check-one-transport: no TypeScript was read from %s" % source)
        return 1

    if failures:
        print("FAIL check-one-transport: something reaches the API without the "
              "transport, and so without a CSRF token")
        for failure in failures:
            print("  " + failure)
        print("\nUse the transport in web/src/lib/api.ts.")
        return 1

    print("check-one-transport: passed; %d files reach the API only through %s"
          % (scanned, shown(transport)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
