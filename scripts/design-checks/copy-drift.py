#!/usr/bin/env python3
"""Compare a page that carries its own copy of the component rules against the library.

css-coverage pools definitions across every file it is given, which is right —
file by file, every cross-file definition looks like a violation, and that is how
a checker earns the reputation that gets it switched off. But the pooling hides
the failure this script exists for: a page that must render standalone keeps its
own copy of the rules, and the copy drifts.

It shipped twice. `textarea` and `select-trigger` were missing from one copy and
rendered as unstyled browser defaults inside the document that specifies them.
Later `overlay` went the same way: the demo opened, the focus returned, and the
backdrop was fully transparent with no blur — because the rule lived only in the
library and the page had its own stylesheet.

Usage: copy-drift.py <page.html> <library.css> ...
"""
import re
import sys
from pathlib import Path

SLOT = re.compile(r'\[data-slot="([\w-]+)"\]')
USE = re.compile(r'data-slot="([\w-]+)"')


def page_parts(path: Path) -> tuple[str, str]:
    text = path.read_text(encoding="utf-8")
    styles = "\n".join(re.findall(r"<style[^>]*>(.*?)</style>", text, re.S))
    markup = re.sub(r"<style[^>]*>.*?</style>", "", text, flags=re.S)
    return styles, markup


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2

    page = Path(argv[0])
    styles, markup = page_parts(page)
    own = set(SLOT.findall(styles))
    used = set(USE.findall(markup))

    library = set()
    for lib in argv[1:]:
        library |= set(SLOT.findall(Path(lib).read_text(encoding="utf-8")))

    problems = 0
    # The one that renders wrong without saying so.
    for name in sorted(used - own):
        where = "also missing from the library" if name not in library else "defined only in the library"
        print('FAIL %s uses [data-slot="%s"] and does not style it — %s'
              % (page.name, name, where))
        problems += 1
    # Drift in the other direction is not a rendering failure, but it means the
    # page is demonstrating something the library no longer provides.
    for name in sorted(own - library):
        print('WARN %s styles [data-slot="%s"] which the library does not define'
              % (page.name, name))

    if problems:
        print("copy-drift: %d slot(s) unstyled in %s" % (problems, page.name), file=sys.stderr)
        return 1
    print("copy-drift: passed; %s styles every one of the %d slots it uses"
          % (page.name, len(used)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
