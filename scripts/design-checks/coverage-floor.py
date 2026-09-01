#!/usr/bin/env python3
"""Every file handed to a check has to contain something for it to check.

The failure mode this exists for: a check keeps running, keeps printing passed, and
covers less than it did. A pattern-matching check fails by matching nothing, and matching
nothing looks exactly like finding no problems. The seven stylesheet checks here all
print passed on an empty file and on a file whose rules the parser could not reach.

They do fail loudly on a path that does not exist — that was measured, all four tested
exit 1 with a traceback. The gap is a file that exists and yields nothing: a rename that
emptied it, a build step that stopped writing it, a syntax error that made every rule
unreachable, or the recommended command still naming a file whose content moved.

So this asserts the floor rather than the ceiling: each named file must yield at least
one rule and one declaration, and the run prints the counts so a drop is visible in the
output rather than only at zero.

Usage: coverage-floor.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

failures: list[str] = []


def stylesheet(path: Path) -> str:
    text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        text = "\n".join(m.group(1) for m in
                         re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S))
    return re.sub(r"/\*.*?\*/", "", text, flags=re.S)


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    counts = []
    for arg in argv:
        path = Path(arg)
        if not path.is_file():
            failures.append("%s: not a file" % arg)
            continue
        css = stylesheet(path)
        rules = len(re.findall(r"[^{}]+\{[^{}]*\}", css))
        decls = len(re.findall(r"(?:^|[;{])\s*[a-zA-Z-]+\s*:", css))
        if rules == 0:
            failures.append("%s: the parser found no rules at all — the file exists and "
                            "yields nothing, which every check reads as passing" % path.name)
        elif decls == 0:
            failures.append("%s: %d rules and no declarations" % (path.name, rules))
        counts.append((path.name, rules, decls))
    if failures:
        for f in failures:
            print("FAIL coverage-floor: " + f)
        return 1
    print("coverage-floor: passed; " + ", ".join(
        "%s %d/%d" % (n, r, d) for n, r, d in counts))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
