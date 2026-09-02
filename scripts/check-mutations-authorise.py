#!/usr/bin/env python3
"""A handler that changes something authorises first.

Every mutating console endpoint does today: the per-group ones take
authorizedSession with WriteAccess, which routes to a live admin check rather
than a cached one, and the three that cannot take a group check what they can —
the operator role for an upgrade, a one-time token for a claim, nothing at all
for the session exchange that has no session yet.

Nothing held that. The next phase adds endpoints — a control group, a dry run, a
daily digest switch — and a handler that mutates without authorising looks
exactly like one that does until somebody tries it.

The check reads the method dispatch, follows the named handler, and asks whether
that function authorises. It cannot follow a handler that delegates the write
further down; the allowlist is where a case like that gets written down with its
reason rather than passing silently.

Usage: check-mutations-authorise.py [package-directory]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
MUTATING = re.compile(r"http\.Method(Post|Put|Patch|Delete)")
# Reading the method alone would miss a GET that changes state, and one exists on
# purpose: GET /enter/{token} spends a one-time token and creates a session,
# because a link in a chat can only be a GET. Naming it here puts it in the count
# and forces it through ALLOWED with a reason, instead of leaving it invisible.
# A GET added later still has to be noticed by a person and named here.
MUTATING_GETS = {
    "enter": "GET /enter/{token} in internal/console/api/server.go",
}
DISPATCH = re.compile(r"s\.([a-zA-Z][A-Za-z0-9]*)\(writer, request")
# The method and the call are on different lines: one `case` naming the method,
# then the handler on the lines that follow until the next case. Matching a
# single line found nothing at all, which the coverage guard caught.
#
# Stopping at any closing brace was the next mistake: a handler placed after an
# inner if-block went unseen, so adding an unauthorised call two lines lower
# passed. A case ends at the next case, at a default, or at the end of the
# function — not at the first brace that happens to close something.
NEXT_CASE = re.compile(r"^\s*(case |default:)|^\}")
FUNCTION = re.compile(r"^func \(s \*Server\) ([A-Za-z][A-Za-z0-9]*)\(", re.M)
AUTHORISES = re.compile(r"authorizedSession\([^)]*auth\.WriteAccess|"
                        r"Principal\.Role != auth\.RoleOperator|"
                        r"RedeemSetupToken|setupToken")
# Presence was the wrong question. A handler that authorises on its last line
# passes a search of its body and still touches the service first, so the check
# now compares positions: the authorisation has to come before the first call
# into a dependency. s.authorizedSession and s.authenticator are the machinery
# doing the asking, so they are not what is being ordered against.
SERVICE_CALL = re.compile(r"s\.(?!authorizedSession\b)(?!authenticator\b)"
                          r"[a-z][A-Za-z0-9]*\.[A-Z][A-Za-z0-9]*\(")

# handler -> why it does not take authorizedSession with WriteAccess.
ALLOWED = {
    "createSession": "creates the session; there is nothing to authorise against yet",
    "submitSetup": "gated by the one-time claim token in its path, and the route stops "
                   "being registered once the claim succeeds",
    "requestUpgrade": "instance-wide rather than per-group; checks the operator role and CSRF",
    "enter": "spends the one-time link token, which is the authorisation; there is no "
             "session to ask about until it succeeds",
}


def function_bodies(text: str) -> dict:
    bodies = {}
    matches = list(FUNCTION.finditer(text))
    for index, match in enumerate(matches):
        end = matches[index + 1].start() if index + 1 < len(matches) else len(text)
        bodies[match.group(1)] = text[match.start():end]
    return bodies


def main() -> int:
    package = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "internal/console/api")
    if not package.is_dir():
        print("FAIL check-mutations-authorise: %s is not a directory" % package)
        return 1

    bodies = {}
    sources = []
    for path in sorted(package.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        text = path.read_text(encoding="utf-8")
        sources.append((path, text))
        bodies.update(function_bodies(text))
    if not bodies:
        print("FAIL check-mutations-authorise: no handler was found in %s — has the "
              "handler shape changed?" % package.name)
        return 1

    mutating = {}
    for path, text in sources:
        lines = text.split("\n")
        for index, line in enumerate(lines):
            if not MUTATING.search(line):
                continue
            for handler in DISPATCH.findall(line):
                mutating.setdefault(handler, (path, index + 1))
            for following in lines[index + 1:]:
                if NEXT_CASE.match(following):
                    break
                for handler in DISPATCH.findall(following):
                    mutating.setdefault(handler, (path, index + 1))
    for handler, where in MUTATING_GETS.items():
        if handler not in bodies:
            print("FAIL check-mutations-authorise: %s is named as a state-changing GET "
                  "and no such handler is defined; the route it stood for has moved"
                  % handler)
            return 1
        mutating.setdefault(handler, (Path(where.split()[-1]), 0))
    if not mutating:
        print("FAIL check-mutations-authorise: no mutating dispatch was found — has "
              "the routing shape changed?")
        return 1

    failures = []
    for handler, reason in sorted(ALLOWED.items()):
        if handler not in bodies:
            failures.append("ALLOWED names %s, and no such handler is defined here (%s); "
                            "an exception for code that is gone reads as a decision about "
                            "code that is here" % (handler, reason))
    for handler, (path, number) in sorted(mutating.items()):
        if handler in ALLOWED:
            continue
        body = bodies.get(handler)
        if body is None:
            failures.append("%s:%d dispatches %s for a mutating method and no such "
                            "handler is defined here"
                            % (path.relative_to(ROOT), number, handler))
            continue
        authorised = AUTHORISES.search(body)
        if not authorised:
            failures.append("%s:%d routes a mutating method to %s, which authorises "
                            "nothing" % (path.relative_to(ROOT), number, handler))
            continue
        touches = SERVICE_CALL.search(body)
        if touches and touches.start() < authorised.start():
            failures.append("%s:%d routes a mutating method to %s, which calls %s "
                            "before it authorises anybody"
                            % (path.relative_to(ROOT), number, handler,
                               touches.group(0).rstrip("(")))

    if failures:
        print("FAIL check-mutations-authorise: something changes state without "
              "asking who is asking")
        for failure in failures:
            print("  " + failure)
        print("\nTake authorizedSession with auth.WriteAccess, or record the handler "
              "in ALLOWED with the reason it cannot.")
        return 1

    print("check-mutations-authorise: passed; %d mutating handlers authorise, "
          "%d listed exceptions" % (len(mutating) - len(ALLOWED), len(ALLOWED)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
