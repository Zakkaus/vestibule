#!/usr/bin/env python3
"""Components that sit side by side must agree on the properties that show it.

Self-consistency is what the other checks here test: one selector, one value, no
shadowing. This tests the other kind — two components that share a row have to agree,
and nothing about either rule on its own says they disagree.

It found the case it was written for on the first run: a button carried
`border-radius: var(--radius-lg)` and a field carried `var(--radius-md)`. Both are on
the ladder, both are internally consistent, and a toolbar with a button beside an input
had two different corner radii. Heights had already been made equal; corners had not,
because nothing compares one component against another.

Only base rules are compared — a selector that is exactly `[data-slot="x"]`. Variants
are supposed to differ: `[data-size="sm"]` is smaller on purpose and a pill is rounder
on purpose, and comparing those produces noise that buries the real finding. That was
measured: comparing every rule reported five properties as inconsistent, four of them
size and shape variants doing their job.

Peer groups and the properties they must agree on are declared below, because which
components share a row is a design decision rather than something a parser can infer.

Usage: peer-consistency.py <file.css> ...
"""
import re
import sys
from pathlib import Path

# Components that share a row, and what a reader notices when they disagree.
GROUPS = {
    "controls on one row": (
        ("button", "input", "select-trigger"),
        ("block-size", "border-radius", "font-size"),
    ),
    "text inputs": (
        ("input", "textarea"),
        ("border-radius", "font-size", "padding-inline"),
    ),
}

BASE = re.compile(r'^\[data-slot="([a-z-]+)"\]$')


def normalise(value: str) -> str:
    """`.875rem` and `0.875rem` are one value written two ways.

    Reporting them as a disagreement is noise, and noise is what gets a check turned
    off. This one did exactly that on its second run.
    """
    v = " ".join(value.split())
    v = re.sub(r"(?<![\w.])\.(\d)", r"0.\1", v)
    v = re.sub(r"(\d)\.0+(?![\d])", r"\1", v)
    return v.lower()
failures: list[str] = []


def base_declarations(paths: list[Path]) -> dict[str, dict[str, str]]:
    out: dict[str, dict[str, str]] = {}
    for path in paths:
        text = path.read_text(encoding="utf-8")
        if path.suffix != ".css":
            text = "\n".join(m.group(1) for m in
                             re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S))
        text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
        for m in re.finditer(r"([^{}]+)\{([^{}]*)\}", text):
            for part in m.group(1).split(","):
                hit = BASE.fullmatch(part.strip())
                if not hit:
                    continue
                slot = out.setdefault(hit.group(1), {})
                for d in re.finditer(r"(?:^|;)\s*([a-z-]+)\s*:\s*([^;]+)", m.group(2)):
                    # Assign, do not setdefault. At equal specificity the last
                    # declaration wins, so the browser reads the last one and a check
                    # keeping the first reads a value nothing renders. Appending a
                    # second radius to this library made it report agreement while the
                    # page disagreed.
                    slot[d.group(1)] = normalise(d.group(2))
    return out


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
        print("peer-consistency: cannot read " + ", ".join(unreadable), file=sys.stderr)
        return 2
    decls = base_declarations([Path(a) for a in argv])
    compared = 0
    for group, (slots, props) in GROUPS.items():
        present = [s for s in slots if s in decls]
        if len(present) < 2:
            failures.append("group %r has fewer than two of its components in these "
                            "files, so it compared nothing" % group)
            continue
        for prop in props:
            values: dict[str, list[str]] = {}
            for slot in present:
                v = decls[slot].get(prop)
                if v:
                    values.setdefault(v, []).append(slot)
            if len(values) > 1:
                compared += 1
                failures.append(
                    "%s: %s disagrees — %s" % (group, prop, "; ".join(
                        "%s on %s" % (v, " and ".join(s)) for v, s in values.items())))
            elif values:
                compared += 1
    if compared == 0 and not failures:
        print("FAIL peer-consistency: nothing was compared — no group had two "
              "components carrying any of its properties")
        return 1
    if failures:
        for f in failures:
            print("FAIL peer-consistency: " + f)
        return 1
    print("peer-consistency: passed; %d property comparisons across %d groups"
          % (compared, len(GROUPS)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
