#!/usr/bin/env python3
"""Rules the comment syntax swallowed, which every other check reads as present.

Two of these shipped in a real stylesheet and four checks stayed green on both:

  a selector repeated immediately before a comment
      `.tr.placeholder > .td/* the empty-table message */`
      `.tr.placeholder > .td { white-space: nowrap; }`
      With the comment removed the two run together into
      `.tr.placeholder > .td .tr.placeholder > .td`, a descendant selector that
      cannot match anything. The nowrap never applied, in the one state it exists
      for, and the fix that added it was reported as done.

  a second `*/` inside a long comment
      Nine lines of prose land in the stylesheet as the leading part of the
      selector for whatever rule follows, and the parser discards that rule.

Neither is a shadowed declaration, an undefined var, a literal value or a theme
leak, so nothing that reads declarations can see them: the problem is that the
declaration never reaches the CSSOM at all. What a source reader can check is the
comment boundaries themselves, which is what this does.

Usage: comment-boundaries.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

failures: list[str] = []
# Characters that cannot appear in the part of a selector preceding a `{`.
ILLEGAL_IN_SELECTOR = re.compile(r"[;{}]|[^\s]\s*/\*")


def stylesheet(path: Path) -> str:
    text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        text = "".join(m.group(1) for m in
                       re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S))
    return text


def check(path: Path) -> None:
    css = stylesheet(path)
    depth = 0
    i = 0
    # Walk the text, tracking whether we are inside a comment, and note both the
    # boundaries and what sits immediately either side of them.
    while True:
        start = css.find("/*", i)
        if start < 0:
            break
        end = css.find("*/", start + 2)
        if end < 0:
            failures.append("%s:%d: comment opened and never closed"
                            % (path.name, css.count("\n", 0, start) + 1))
            return
        inner = css[start + 2:end]
        if "*/" in inner:                                   # cannot happen; kept for clarity
            pass
        before = css[:start].rstrip()
        if before and before[-1] not in "{};,>+~ \n\t" and not before.endswith("*/"):
            line = css.count("\n", 0, start) + 1
            tail = before.split("\n")[-1][-46:]
            failures.append("%s:%d: a comment opens directly after %r — with the comment "
                            "removed this joins the next selector" % (path.name, line, tail))
        i = end + 2

    # A stray `*/` outside any comment: scan with comments blanked and look for one.
    blanked = re.sub(r"/\*.*?\*/", lambda m: "\n" * m.group(0).count("\n"), css, flags=re.S)
    for m in re.finditer(r"\*/", blanked):
        failures.append("%s:%d: `*/` outside any comment — everything above it up to the "
                        "previous `/*` is being parsed as CSS"
                        % (path.name, blanked.count("\n", 0, m.start()) + 1))

    # A selector prelude that contains something a selector cannot contain.
    for m in re.finditer(r"(^|[};])([^{};]{1,300}?)\{", blanked, re.S):
        prelude = m.group(2)
        if prelude.lstrip().startswith("@"):
            continue
        bad = ILLEGAL_IN_SELECTOR.search(prelude)
        if bad and "/*" not in bad.group(0):
            line = blanked.count("\n", 0, m.start(2)) + 1
            failures.append("%s:%d: selector prelude contains %r, which a selector cannot"
                            % (path.name, line, bad.group(0)))


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
        print("comment-boundaries: cannot read " + ", ".join(unreadable), file=sys.stderr)
        return 2
    for a in argv:
        check(Path(a))
    if failures:
        for f in sorted(set(failures)):
            print("FAIL comment-boundaries: " + f)
        return 1
    print("comment-boundaries: passed; every comment closes where it opens and no "
          "selector is joined to one")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
