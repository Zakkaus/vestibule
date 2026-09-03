#!/usr/bin/env python3
"""A percentage inside a min-size resolves to zero against a shrink-to-fit parent.

`min-inline-size: min(var(--field-w), 100%)` was written to stop a control overflowing a
narrow card, and it never once applied. The host wraps each control in a bare `<div>`
that shrinks to fit its content, and against a containing block whose size depends on its
contents the spec resolves a percentage min-size to zero. Every control fell back to its
content width. Nothing errored, and every checker stayed green, because the declaration
is there and well-formed — it just computes to nothing.

The same shape is safe on `inline-size` and `max-inline-size`, which is why the rule is
narrow: a percentage in a **min** track is the one the spec zeroes rather than treats as
indefinite. So this reports `min-inline-size` and `min-block-size` whose value mentions a
percentage, including inside `min()`, `max()`, `clamp()` and `calc()`.

The fix is never a different percentage. Give the parent a definite size, or state the
minimum in absolute units and let `max-inline-size: 100%` do the clamping — that one is
allowed to be indefinite.

Usage: percentage-min.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

from _page_css import page_css

MIN_SIZE = re.compile(
    r"(?:^|[;{])\s*(min-(?:inline|block)-size|min-width|min-height)\s*:\s*([^;}]+)")
failures: list[str] = []
seen = 0


def stylesheet(path: Path) -> str:
    """The page's CSS, blocks and style="" attributes alike. See _page_css.py."""
    return page_css(path)

def check(path: Path) -> None:
    global seen
    css = stylesheet(path)
    for m in MIN_SIZE.finditer(css):
        seen += 1
        value = m.group(2).strip()
        if "%" not in value:
            continue
        line = css.count("\n", 0, m.start(2)) + 1
        failures.append(
            "%s:%d: `%s: %s` — a percentage in a min track resolves to zero against a "
            "shrink-to-fit parent, so this declaration can compute to nothing while "
            "reading as correct" % (path.name, line, m.group(1), value[:48]))


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    # A path that cannot be read is not a finding about the library. Without this
    # the open below raises, Python exits 1, and that is the same code a real
    # violation uses — so one mistyped argument reads as a failing check. The test
    # is the read itself rather than is_file(): the two agree on a missing path and
    # part company on one whose mode or encoding stops the open that follows.
    unreadable = []
    for a in argv:
        try:
            Path(a).read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError) as e:
            unreadable.append("%s (%s)" % (a, getattr(e, "strerror", None) or e.__class__.__name__))
    if unreadable:
        print("percentage-min: cannot read " + ", ".join(unreadable), file=sys.stderr)
        return 2
    for a in argv:
        check(Path(a))
    if seen == 0:
        print("FAIL percentage-min: no min-size declaration matched at all — either "
              "none exist or the pattern stopped matching")
        return 1
    if failures:
        for f in failures:
            print("FAIL percentage-min: " + f)
        return 1
    print("percentage-min: passed; %d min-size declarations, none stated as a percentage"
          % seen)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
