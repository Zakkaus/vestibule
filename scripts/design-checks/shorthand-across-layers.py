#!/usr/bin/env python3
"""A shorthand in one layer resets the whole group written in another.

Within one stylesheet a shorthand is ordinary and this check ignores it. The failure is
across layers, where the writer of the later rule cannot see what the earlier one set:

  a `background` shorthand in a project layer erased the `background-image`,
  `-size`, `-position` and `-repeat` that a component layer used to draw the arrow
  on a native `select`. Every dropdown in the product lost its arrow.

  a `padding` shorthand in the same place overwrote the `padding-inline-end` that
  had reserved room for that arrow, so the arrow sat on top of the option text.

Neither is a shadowed declaration in the sense the shadowing check means: the two
declarations have different property names, so nothing keyed on the name sees a repeat.
What makes it a defect is that one name owns the other's group.

So: for a selector two files both style, report a shorthand in one against any longhand
of its group in the other. A single file styling a selector twice is the shadowing
check's job, not this one.

Give it stylesheets that load together as separate layers. Do **not** give it a page
carrying its own inline copy of those same rules: every shared declaration then reads
as a cross-layer collision with itself, which is `copy-drift.py`'s question, not this
one. Running it that way produced four confident failures about `[data-slot="button"]`
disagreeing with a copy of itself.

Usage: shorthand-across-layers.py <file.css|file.html> <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

from _page_css import page_css

GROUPS = {
    "background": {"background-color", "background-image", "background-size",
                   "background-position", "background-repeat", "background-attachment",
                   "background-clip", "background-origin"},
    "padding": {"padding-block", "padding-inline", "padding-block-start",
                "padding-block-end", "padding-inline-start", "padding-inline-end",
                "padding-top", "padding-bottom", "padding-left", "padding-right"},
    "margin": {"margin-block", "margin-inline", "margin-block-start", "margin-block-end",
               "margin-inline-start", "margin-inline-end", "margin-top", "margin-bottom",
               "margin-left", "margin-right"},
    "border": {"border-width", "border-style", "border-color", "border-block",
               "border-inline", "border-block-start", "border-inline-start"},
    "font": {"font-family", "font-size", "font-weight", "line-height", "font-style",
             "font-variant"},
    "inset": {"inset-block", "inset-inline", "inset-block-start", "inset-block-end",
              "inset-inline-start", "inset-inline-end"},
    "outline": {"outline-width", "outline-style", "outline-color"},
    "overflow": {"overflow-x", "overflow-y"},
    "gap": {"row-gap", "column-gap"},
    "flex": {"flex-grow", "flex-shrink", "flex-basis"},
    "transition": {"transition-property", "transition-duration",
                   "transition-timing-function", "transition-delay"},
    "animation": {"animation-name", "animation-duration", "animation-timing-function",
                  "animation-delay", "animation-iteration-count", "animation-direction"},
    "list-style": {"list-style-type", "list-style-position", "list-style-image"},
}

failures: list[str] = []


def stylesheet(path: Path) -> str:
    """The page's CSS, blocks and style="" attributes alike. See _page_css.py."""
    return page_css(path)

def declarations(path: Path) -> dict[str, set[str]]:
    out: dict[str, set[str]] = {}
    for m in re.finditer(r"([^{}]+)\{([^{}]*)\}", stylesheet(path)):
        selector = " ".join(m.group(1).split())
        if not selector or selector.startswith("@"):
            continue
        props = {d.group(1) for d in re.finditer(r"(?:^|;)\s*([a-z-]+)\s*:", m.group(2))}
        out.setdefault(selector, set()).update(props)
    return out


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    paths = [Path(a) for a in argv]
    per = {p: declarations(p) for p in paths}
    for i, a in enumerate(paths):
        for b in paths[i + 1:]:
            for selector in sorted(set(per[a]) & set(per[b])):
                for short, longs in GROUPS.items():
                    for first, second in ((a, b), (b, a)):
                        if short in per[first][selector]:
                            clash = sorted(per[second][selector] & longs)
                            if clash:
                                failures.append(
                                    "`%s` — %s writes the `%s` shorthand, %s writes %s; "
                                    "whichever loads later wipes the other"
                                    % (selector[:56], first.name, short, second.name,
                                       ", ".join("`%s`" % c for c in clash)))
    if failures:
        for f in sorted(set(failures)):
            print("FAIL shorthand-across-layers: " + f)
        return 1
    print("shorthand-across-layers: passed; no shorthand in one file owns a group "
          "another file writes longhand")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
