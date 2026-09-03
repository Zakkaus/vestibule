#!/usr/bin/env python3
"""Check the implemented subset of the architecture's console route table.

The phase-six clause originally claimed complete equality. The plan records the
remaining route-table differences as a decision for phases nine and ten, so
this check proves the implemented subset is documented and prints that named
exemption rather than treating the deferred rows as a pass.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ARCHITECTURE = ROOT / "docs" / "ARCHITECTURE.md"
API = ROOT / "internal" / "console" / "api"
TABLE_START = "### 路由"
TABLE_END = "**这张表是穷举的"

# Each route maps to the function and source tokens that admit that exact form.
LIVE_ROUTES = {
    "GET /livez": ("server.go", "serveHTTP", ('request.URL.Path == "/livez"',)),
    "GET /readyz": ("server.go", "serveHTTP", ('request.URL.Path == "/readyz"',)),
    "POST /api/session": ("server.go", "apiRoute", ('request.URL.Path == "/api/session"', "http.MethodPost")),
    "GET /api/session": ("server.go", "apiRoute", ('request.URL.Path == "/api/session"', "http.MethodGet")),
    "GET /enter/{token}": ("server.go", "serveHTTP", ('strings.HasPrefix(request.URL.Path, "/enter/")',)),
    "GET · POST /setup/{token}": (
        "setup.go",
        "setupRoute",
        ("case http.MethodGet:", "case http.MethodPost:", "s.setup.SetupAvailable(token)"),
    ),
    "GET /api/chats": ("server.go", "apiRoute", ('request.URL.Path == "/api/chats"',)),
    "GET /api/chats/{id}/queue": ("server.go", "queueRoute", ("case http.MethodGet:", "len(rest) == 0")),
    "POST /api/chats/{id}/queue/{cid}": ("server.go", "queueRoute", ("case http.MethodPost:", "len(rest) == 1")),
    "GET /api/chats/{id}/settings": ("settings.go", "settingsRoute", ("case http.MethodGet:",)),
    "PATCH /api/chats/{id}/settings": ("settings.go", "settingsRoute", ("case http.MethodPatch:",)),
    "GET · PUT /api/chats/{id}/rules": ("rules.go", "rulesRoute", ("request.Method == http.MethodGet", "request.Method == http.MethodPut")),
    "GET /api/chats/{id}/audit": ("server.go", "auditRoute", ("case http.MethodGet:",)),
    "POST /api/chats/{id}/audit/{aid}/undo": ("server.go", "auditRoute", ("case http.MethodPost:", 'rest[1] == "undo"')),
    "GET /api/chats/{id}/stats": ("stats.go", "statsRoute", ("request.Method == http.MethodGet",)),
    "GET /api/status": ("server.go", "statusRoute", ('request.URL.Path == "/api/status"',)),
    "GET /api/status/release": ("server.go", "statusRoute", ('request.URL.Path == "/api/status/release"',)),
    "GET /api/process/settings": ("server.go", "apiRoute", ('request.URL.Path == "/api/process/settings"',)),
    "POST /api/status/upgrade": ("server.go", "statusRoute", ('request.URL.Path == "/api/status/upgrade"', "http.MethodPost")),
}

# These table rows are intentionally ahead of the implementation. Keep this list
# explicit: a new undocumented row must fail rather than quietly become exempt.
DEFERRED_ROWS = {
    "GET /api/chats/{id}/overview",
    "PATCH /api/chats/{id}",
    "POST /api/chats/{id}/rules/test",
    "GET · PUT /api/chats/{id}/feeds",
    "GET /api/chats/{id}/packages",
    "POST /api/chats/{id}/packages",
    "GET · PATCH /api/me/preferences",
    "GET /verify/{token}",
}


def route_rows() -> set[str]:
    if not ARCHITECTURE.is_file():
        raise ValueError(f"route-table target is missing: {ARCHITECTURE}")
    text = ARCHITECTURE.read_text(encoding="utf-8")
    start = text.find(TABLE_START)
    end = text.find(TABLE_END, start)
    if start < 0 or end < 0:
        raise ValueError("architecture route table or its exhaustiveness marker is missing")

    rows = set()
    for line in text[start:end].splitlines():
        if not line.startswith("|"):
            continue
        cells = line.split("|")
        if len(cells) < 3:
            continue
        route = cells[1].strip()
        if route and route != "路径" and set(route) != {"-"}:
            rows.add(route)
    if not rows:
        raise ValueError("architecture route table has no route rows")
    return rows


def function_body(path: Path, name: str) -> str:
    if not path.is_file():
        raise ValueError(f"route source target is missing: {path}")
    text = path.read_text(encoding="utf-8")
    marker = f"func (s *Server) {name}("
    start = text.find(marker)
    if start < 0:
        raise ValueError(f"{path}: route function {name} is missing")
    next_function = text.find("\nfunc ", start + len(marker))
    return text[start:] if next_function < 0 else text[start:next_function]

def route_has_method(documented: set[str], method: str, path: str) -> str | None:
    """Return the matching table row, if its method includes this operation."""
    for route in documented:
        methods, _, documented_path = route.rpartition(" ")
        if documented_path != path:
            continue
        if method in {part.strip() for part in methods.split("·")}:
            return route
    return None


def documented_path(documented: set[str], path: str) -> bool:
    return any(route.rpartition(" ")[2] == path for route in documented)


def implemented_path_segments(documented: set[str]) -> list[str]:
    """Find source branches that add paths beyond the named live routes."""
    failures = []
    for function in ("serveHTTP", "apiRoute", "statusRoute"):
        try:
            body = function_body(API / "server.go", function)
        except ValueError as error:
            failures.append(str(error))
            continue
        for path in sorted(set(re.findall(r'request\.URL\.Path\s*==\s*"([^"]+)"', body))):
            if not documented_path(documented, path):
                failures.append(
                    f"implemented exact path {path!r} has no exhaustive route-table "
                    "row; clients cannot discover or safely rely on it"
                )

    try:
        chat_body = function_body(API / "server.go", "chatRoute")
    except ValueError as error:
        return failures + [str(error)]

    for segment in sorted(set(re.findall(r'case\s+"([^"]+)":', chat_body))):
        path = f"/api/chats/{{id}}/{segment}"
        routes = [route for route in documented if route.rpartition(" ")[2] == path]
        if not routes:
            failures.append(
                f"implemented chat path segment {segment!r} has no exhaustive "
                "route-table row; clients cannot discover or safely rely on it"
            )
        elif any(route in DEFERRED_ROWS for route in routes):
            failures.append(
                f"implemented chat path segment {segment!r} is still marked "
                f"deferred by the route table ({', '.join(sorted(routes))})"
            )

    try:
        audit_body = function_body(API / "server.go", "auditRoute")
    except ValueError as error:
        return failures + [str(error)]
    get_start = audit_body.find("case http.MethodGet:")
    post_start = audit_body.find("case http.MethodPost:", get_start + 1)
    if get_start < 0:
        get_body = ""
    else:
        get_body = audit_body[get_start:post_start if post_start >= 0 else len(audit_body)]
    for segment in sorted(set(re.findall(r'rest\s*\[\s*0\s*\]\s*==\s*"([^"]+)"', get_body))):
        path = f"/api/chats/{{id}}/audit/{segment}"
        route = route_has_method(documented, "GET", path)
        if route is None:
            failures.append(
                f"implemented GET audit path segment {segment!r} has no "
                "exhaustive route-table row; clients cannot discover or safely rely on it"
            )
        elif route in DEFERRED_ROWS:
            failures.append(
                f"implemented GET audit path segment {segment!r} is still marked "
                f"deferred by the route table ({route})"
            )
    return failures


def main() -> int:
    try:
        documented = route_rows()
    except ValueError as error:
        print(f"FAIL check-console-routes: {error}", file=sys.stderr)
        return 1

    failures = []
    for route, (filename, function, tokens) in LIVE_ROUTES.items():
        if route not in documented:
            failures.append(f"route table no longer documents live route {route}")
            continue
        try:
            body = function_body(API / filename, function)
        except ValueError as error:
            failures.append(str(error))
            continue
        for token in tokens:
            if token not in body:
                failures.append(f"{route}: {filename}:{function} no longer contains {token!r}")
    failures.extend(implemented_path_segments(documented))

    not_live = documented - set(LIVE_ROUTES)
    if not_live != DEFERRED_ROWS:
        unexpected = sorted(not_live - DEFERRED_ROWS)
        missing = sorted(DEFERRED_ROWS - not_live)
        if unexpected:
            failures.append("unclassified documented route rows: " + ", ".join(unexpected))
        if missing:
            failures.append("deferred route rows became live or disappeared: " + ", ".join(missing))

    if failures:
        for failure in failures:
            print(f"FAIL check-console-routes: {failure}", file=sys.stderr)
        return 1

    for route in sorted(LIVE_ROUTES):
        print(f"ok route table documents live route: {route}")
    print(
        "EXEMPT route-table rows not implemented: "
        + ", ".join(sorted(DEFERRED_ROWS))
        + " — remaining ownership is recorded in docs/PLAN-v5.md"
    )
    print(
        "check-console-routes: passed; %d live rows match, %d deferred rows named"
        % (len(LIVE_ROUTES), len(DEFERRED_ROWS))
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
