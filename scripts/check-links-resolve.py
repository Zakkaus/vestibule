#!/usr/bin/env python3
"""Every link in the console points at a route that exists.

A mistyped path compiles, renders, and goes nowhere. TypeScript cannot catch it
because the target is a string; the render gate walks the route table rather than
the links into it; and the screen that carries the broken link looks fine until
somebody clicks.

Three shapes reach a route: a literal to="/x", a pathname inside an object
target, and a template literal that starts with a path before its query. All
three are checked against the routes declared in web/src/app/App.tsx, which is
the same source the render gate reads, so the two cannot disagree about what
exists.

Usage: check-links-resolve.py [App.tsx] [source-directory]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
ROUTE = re.compile(r'path:\s*"([a-z-]+)"')
LINKS = (
    re.compile(r'to="(/[A-Za-z0-9_./-]*)"'),
    re.compile(r'pathname:\s*"(/[A-Za-z0-9_./-]*)"'),
    re.compile(r'to=\{`(/[A-Za-z0-9_./-]*)'),
)


def main() -> int:
    app = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "web/src/app/App.tsx")
    source = ROOT / (sys.argv[2] if len(sys.argv) > 2 else "web/src")
    if not app.is_file():
        print("FAIL check-links-resolve: %s is missing, so no route is known" % app)
        return 1
    if not source.is_dir():
        print("FAIL check-links-resolve: %s is not a directory" % source)
        return 1

    routes = {"/" + name for name in ROUTE.findall(app.read_text(encoding="utf-8"))}
    if not routes:
        print("FAIL check-links-resolve: no route was read from %s — has the route "
              "table changed shape?" % app.name)
        return 1
    routes.add("/")

    found = 0
    failures = []
    for path in sorted(source.rglob("*")):
        if path.suffix != ".tsx":
            continue
        text = path.read_text(encoding="utf-8")
        for number, line in enumerate(text.split("\n"), 1):
            for pattern in LINKS:
                for target in pattern.findall(line):
                    found += 1
                    if target in routes:
                        continue
                    failures.append("%s:%d links to %s, which is not a route"
                                    % (path.relative_to(ROOT), number, target))

    if found == 0:
        print("FAIL check-links-resolve: no link target was found in %s — has the "
              "link shape changed?" % source.name)
        return 1

    if failures:
        print("FAIL check-links-resolve: a link points where no route is")
        for failure in failures:
            print("  " + failure)
        print("\nRoutes come from %s." % app.relative_to(ROOT))
        return 1

    print("check-links-resolve: passed; %d link targets all name one of %d routes"
          % (found, len(routes)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
