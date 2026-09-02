#!/usr/bin/env python3
"""internal/verification reads the time through its own clock, not the wall.

The package carries an injectable clock — wallNow, and now for the configured
timezone — so that timeouts, cooldowns and the outage pause can be driven in a
test instead of waited out. Four call sites read time.Now() directly anyway, and
two of them decided something a person sees: whether a minute proof is accepted,
and whether a challenge may be resent. In production the injected clock is
time.Now, so nothing behaved differently; what was missing was any way to make
those paths fail on purpose.

Three occurrences stay and are listed here with the reason each is unavoidable.
Anything else in the package is a decision made against a clock nobody can move.

Usage: check-one-clock.py [package-directory]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CALL = re.compile(r"\btime\.Now\(\)")

# Exact source lines, so a new call cannot arrive by resembling an old one.
ALLOWED = {
    "return time.Now()":
        "wallNow's own fallback when no clock was injected",
    "return strconv.FormatInt(time.Now().UnixNano(), 36) // fallback; uniqueness is what matters":
        "a nonce when crypto/rand fails; the value is never compared to a time",
    "lastOnline:        time.Now(),":
        "the constructor's seed, set before the service can call its own method",
}


def main() -> int:
    package = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "internal/verification")
    if not package.is_dir():
        print("FAIL check-one-clock: %s is not a directory, so nothing was read"
              % package)
        return 1

    scanned = 0
    failures = []
    for path in sorted(package.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        scanned += 1
        for number, line in enumerate(path.read_text(encoding="utf-8").split("\n"), 1):
            if not CALL.search(line):
                continue
            if line.strip() in ALLOWED:
                continue
            failures.append("%s:%d reads the wall clock: %s"
                            % (path.relative_to(ROOT), number, line.strip()))

    if scanned == 0:
        print("FAIL check-one-clock: no Go source was read from %s" % package)
        return 1

    if failures:
        print("FAIL check-one-clock: this package decides against a clock a test "
              "cannot move")
        for failure in failures:
            print("  " + failure)
        print("\nUse v.wallNow(), or v.now() when the configured timezone matters.")
        print("If a call genuinely cannot, add its exact line to ALLOWED with a reason.")
        return 1

    print("check-one-clock: passed; %d files read the time through the injected "
          "clock, %d listed exceptions" % (scanned, len(ALLOWED)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
