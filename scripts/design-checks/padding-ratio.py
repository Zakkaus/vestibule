#!/usr/bin/env python3
"""Horizontal padding exceeds vertical on anything that holds a line of text.

A line box carries its own leading, so equal padding on all four sides reads tight at
the sides and loose above and below. Every text-bearing control in a system should sit
in one band; a set spread across 0.33, 1.00, 1.50 and 2.00 is what "the spacing is off"
describes when nobody can point at a single rule being broken.

Measured here before the band existed:

    tabs-trigger   12 / 4  = 0.33   inverted, adjacent tab labels nearly touching
    table-cell     12 / 12 = 1.00   against its own header at 8 / 12
    table-head      8 / 12 = 1.50
    select-trigger  8 / 12 = 1.50
    badge           4 / 8  = 2.00

Containers are exempt and named below: a card is not holding one line, and equal padding
is right for it. A ratio under 1 is reported whatever the component, because inverted
padding is a mistake rather than a choice.

Usage: padding-ratio.py <file.css> ...
"""
import re
import sys
from pathlib import Path

# Components whose job is a surface rather than a line of text.
CONTAINERS = {"card", "panel", "dialog", "sheet", "popover"}
# Rows that span their container edge to edge and take their horizontal padding from it.
# Zero is the correct horizontal value here, and the exception lives in the check rather
# than in a report nobody reads twice — a gate that is permanently red gets skipped on
# the first busy round, exactly like one that is permanently green.
FULL_BLEED = {"setting"}
LOW, HIGH = 1.3, 2.5

SCALE = {"--sp-1": 4, "--sp-2": 8, "--sp-3": 12, "--sp-4": 16,
         "--sp-5": 24, "--sp-6": 32, "--sp-7": 48, "--sp-8": 64}
BASE = re.compile(r'^\[data-slot="([a-z-]+)"\]$')
failures: list[str] = []


def px(value: str) -> float | None:
    value = value.strip()
    m = re.fullmatch(r"var\((--sp-\d)\)", value)
    if m:
        return SCALE.get(m.group(1))
    if value == "0":
        return 0.0
    m = re.fullmatch(r"([\d.]+)rem", value)
    if m:
        return float(m.group(1)) * 16
    m = re.fullmatch(r"([\d.]+)px", value)
    return float(m.group(1)) if m else None


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
        print("padding-ratio: cannot read " + ", ".join(unreadable), file=sys.stderr)
        return 2
    measured = 0
    for arg in argv:
        path = Path(arg)
        text = path.read_text(encoding="utf-8")
        if path.suffix != ".css":
            text = "\n".join(m.group(1) for m in
                             re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S))
        text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
        for m in re.finditer(r"([^{}]+)\{([^{}]*)\}", text):
            hit = BASE.fullmatch(" ".join(m.group(1).split()))
            if not hit:
                continue
            # Last declaration wins, as the runtime resolves it. Both spellings are
            # read: the shorthand, and the pair of logical longhands — a component
            # writing `padding-inline` with its block padding coming from a fixed
            # height was invisible to the first version, which is most of what a
            # control does.
            slot = hit.group(1)
            body = m.group(2)
            y = x = None
            found = re.findall(r"(?:^|;)\s*padding\s*:\s*([^;]+)", body)
            if found:
                parts = found[-1].split()
                if len(parts) == 2:
                    y, x = px(parts[0]), px(parts[1])
                elif len(parts) == 1:
                    y = x = px(parts[0])
            blocks = re.findall(r"(?:^|;)\s*padding-block\s*:\s*([^;]+)", body)
            inlines = re.findall(r"(?:^|;)\s*padding-inline\s*:\s*([^;]+)", body)
            if blocks:
                y = px(blocks[-1].split()[0])
            if inlines:
                x = px(inlines[-1].split()[0])
            if y is None or x is None:
                continue
            if y == 0:
                # A control whose height is declared and whose content is centred takes
                # its vertical space from the box, not from padding. Zero is correct
                # there and a ratio is meaningless, so require the two things that make
                # it correct rather than skipping quietly.
                declares_height = re.search(r"(?:^|;)\s*block-size\s*:", body)
                centres = re.search(r"align-items\s*:\s*center", body)
                if not (declares_height and centres):
                    failures.append("%s has no vertical padding and neither a declared "
                                    "block-size nor centred content — its vertical space "
                                    "comes from nowhere in particular" % slot)
                continue
            measured += 1
            ratio = x / y
            if slot in FULL_BLEED:
                if x != 0:
                    failures.append("%s is listed as full-bleed but declares %gpx "
                                    "horizontal padding" % (slot, x))
                continue
            if ratio < 1:
                failures.append("%s has %gpx vertical against %gpx horizontal — inverted; "
                                "a line box already carries its leading" % (slot, y, x))
            elif slot in CONTAINERS:
                continue
            elif not (LOW <= ratio <= HIGH):
                failures.append("%s is at %.2f, outside %g-%g — %gpx vertical against "
                                "%gpx horizontal" % (slot, ratio, LOW, HIGH, y, x))
    if measured == 0:
        print("FAIL padding-ratio: no component declared a two-value padding, so nothing "
              "was measured")
        return 1
    if failures:
        for f in sorted(set(failures)):
            print("FAIL padding-ratio: " + f)
        return 1
    print("padding-ratio: passed; %d components, every text-bearing one between %g and %g"
          % (measured, LOW, HIGH))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
