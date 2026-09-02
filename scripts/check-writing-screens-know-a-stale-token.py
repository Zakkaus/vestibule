#!/usr/bin/env python3
"""A screen that changes something must recognise a stale security token.

Every state-changing request carries X-CSRF-Token, and the API answers a stale one with
csrf_invalid. A screen that does not name that code falls through to its generic
"unavailable" message, which misdiagnoses the failure and offers a retry that fails the
same way: the token is refreshed by re-entering the console, not by trying again.

Counted across the console, twelve of thirteen screens with an error presenter already
held this. The thirteenth was the version screen, whose POST /api/status/upgrade
answered csrf_invalid with "the host cannot accept an upgrade request right now".

The rule is therefore not new policy; it is the shape the console already had, written
down before the next screen forgets it. A screen that issues no mutating request is not
subject to it, and this check works that out from the screen's own api.ts rather than
from a list somebody has to maintain.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
FEATURES = ROOT / "web" / "src" / "features"
PRESENTER = re.compile(r"function [A-Za-z]*[Ee]rror(?:MessageKey|Presentation)\(")
MUTATION = re.compile(r'method:\s*"(?:POST|PATCH|PUT|DELETE)"')


def main() -> int:
    if not FEATURES.is_dir():
        print("FAIL check-writing-screens-know-a-stale-token: %s does not exist, so this "
              "check read nothing" % FEATURES)
        return 1

    failures = []
    presenters = 0
    writing = 0
    for screen in sorted(FEATURES.iterdir()):
        if not screen.is_dir():
            continue
        component = "".join(path.read_text() for path in sorted(screen.glob("*.tsx")))
        if not PRESENTER.search(component):
            continue
        presenters += 1
        api = screen / "api.ts"
        if not api.exists() or not MUTATION.search(api.read_text()):
            continue
        writing += 1
        if "csrf_invalid" not in component:
            failures.append(
                "%s changes something and never names csrf_invalid, so a stale token "
                "reaches the reader as whatever its generic failure says"
                % screen.relative_to(ROOT))

    if presenters == 0 or writing == 0:
        print("FAIL check-writing-screens-know-a-stale-token: found %d screens with an "
              "error presenter and %d of them writing, so this check read nothing"
              % (presenters, writing))
        return 1
    for failure in failures:
        print("FAIL check-writing-screens-know-a-stale-token: %s" % failure)
    if failures:
        return 1
    print("check-writing-screens-know-a-stale-token: passed; %d screens present errors, "
          "the %d that write all name csrf_invalid" % (presenters, writing))
    return 0


if __name__ == "__main__":
    sys.exit(main())
