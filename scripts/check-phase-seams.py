#!/usr/bin/env python3
"""Hold the seams between phases that the frontend is able to cross early.

A screen built against fixtures can grow a control for an endpoint that belongs
to a later phase. It happened once: the queue screen carried a 撤销 button on
every banned row while GET .../audit and POST .../audit/{aid}/undo did not
exist. An agent sent to wire the frontend stopped and reported it, which cost a
round.

Until the audit screen existed, nothing under web/ was allowed to name those
endpoints. It exists now, so that rule has done its work and would only reject
the screen it was protecting. What survives it is the reason the button was
wrong in the first place: the design language gives the waiting queue 放行、
拒绝、封禁 and gives 撤销 to the audit screen, which knows who placed a ban.
The queue does not, so the queue may not reach for it.
"""
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
WEB = ROOT / "web"
SOURCE_SUFFIXES = {".ts", ".tsx"}

AUDIT_FEATURE = WEB / "src" / "features" / "audit"
AUDIT_ENDPOINT = "/audit"
OWNING_FEATURE = "features/audit"
QUEUE_FEATURE = "features/queue"
QUEUE_FORBIDDEN = (AUDIT_ENDPOINT, "revoke")

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


def before_the_screen_exists(files: list[Path]) -> None:
    for path in files:
        if OWNING_FEATURE in path.as_posix():
            continue
        if AUDIT_ENDPOINT in path.read_text(encoding="utf-8"):
            failures.append(
                "%s names %s, which belongs to the audit screen in phase seven; "
                "stop and say so rather than inventing the endpoint"
                % (path.relative_to(ROOT), AUDIT_ENDPOINT))


def after_the_screen_exists(files: list[Path]) -> None:
    queue = [path for path in files if QUEUE_FEATURE in path.as_posix()]
    if not queue:
        failures.append("no file under web/src/features/queue — this check would "
                        "pass on a tree with no queue screen at all")
        return
    for path in queue:
        text = path.read_text(encoding="utf-8")
        for forbidden in QUEUE_FORBIDDEN:
            if forbidden in text:
                failures.append(
                    "%s names %s; 撤销 belongs to the audit screen, which knows "
                    "who placed the ban" % (path.relative_to(ROOT), forbidden))


def main() -> int:
    files = sources()
    if AUDIT_FEATURE.is_dir():
        after_the_screen_exists(files)
        state = "the audit screen owns 撤销; the queue does not reach for it"
    else:
        before_the_screen_exists(files)
        state = "none reaches for %s" % AUDIT_ENDPOINT
    if failures:
        for failure in failures:
            print("FAIL check-phase-seams: " + failure, file=sys.stderr)
        return 1
    print("check-phase-seams: passed; %d sources, %s" % (len(files), state))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
