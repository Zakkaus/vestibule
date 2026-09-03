#!/usr/bin/env python3
"""Every console font size stays on the ramp documented in design section 02.

A size outside the ramp introduces an unreviewed level into the visual hierarchy. The
checker reads the ramp rather than copying it, then checks both reference pages and every
source stylesheet. Four departures predate the checker; their exact source lines are held
so they cannot multiply or move unnoticed.

Usage: check-type-ramp.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DESIGN = ROOT / "web" / "design.html"
REFERENCE_PAGES = (DESIGN, ROOT / "web" / "architecture.html")
STYLE_ROOT = ROOT / "web" / "src"
CHECKS = ROOT / "scripts" / "design-checks"
sys.path.insert(0, str(CHECKS))
from _page_css import page_css  # noqa: E402

SECTION = re.compile(r'<section id="s2"[^>]*>.*?</section>', re.S)
ROW = re.compile(
    r'<div class="r">.*?<span class="spec">(?P<spec>[^<]+)</span>'
    r'.*?<span class="samp[^"]*" style="(?P<style>[^"]+)"',
    re.S,
)
FONT_SIZE = re.compile(r"\bfont-size\s*:\s*([^;}\n]+)")
NUMBER = re.compile(r"(?:\d+(?:\.\d+)?|\.\d+)")
RANGE = re.compile(r"(\d+(?:\.\d+)?)–(\d+(?:\.\d+)?)rem")
CLAMP = re.compile(
    r"clamp\(\s*((?:\d+(?:\.\d+)?|\.\d+)rem)\s*,.+,\s*"
    r"((?:\d+(?:\.\d+)?|\.\d+)rem)\s*\)"
)

HELD_OFF_RAMP = {
    (
        "web/design.html",
        ".sw .vl { font-family: var(--font-mono); font-size: 0.625rem; color: var(--muted-foreground); overflow-wrap: anywhere; line-height: 1.4; }",
    ),
    (
        "web/design.html",
        ".chart .lbl { fill: var(--muted-foreground); font-size: 12px; font-family: var(--font-mono); }",
    ),
    (
        "web/design.html",
        ".chart .val { fill: var(--muted-foreground); font-size: 12px; font-family: var(--font-mono); font-weight: 500; }",
    ),
    (
        "web/src/styles/shell.css",
        ".brand .name { font-size: .8rem; font-weight: 700; letter-spacing: -.01em; }",
    ),
}


def shown(path: Path) -> str:
    try:
        return str(path.relative_to(ROOT))
    except ValueError:
        return str(path)


def normalise(value: str) -> str:
    value = re.sub(r"\s+", " ", value.strip())
    return re.sub(r"(?<![\d.])\.(\d+rem)\b", r"0.\1", value)


def documented_ramp() -> tuple[set[str], set[tuple[str, str]], list[str]]:
    if not DESIGN.is_file():
        return set(), set(), [f"{shown(DESIGN)} is missing, so the ramp has no source"]
    match = SECTION.search(DESIGN.read_text(encoding="utf-8"))
    if not match:
        return set(), set(), [f'{shown(DESIGN)} has no section id="s2" type ramp']
    rows = list(ROW.finditer(match.group(0)))
    failures = []
    if len(rows) != 13:
        failures.append(f"section 02 has {len(rows)} ramp rows instead of 13")
    sizes: set[str] = set()
    ranges: set[tuple[str, str]] = set()
    for row in rows:
        spec = row.group("spec").split("/", 1)[0].strip()
        if not spec.endswith("rem") or re.findall(r"[A-Za-z%]+", spec) != ["rem"]:
            failures.append(f"ramp entry {row.group('spec')!r} is not expressed only in rem")
            continue
        row_sizes = {normalise(number + "rem") for number in NUMBER.findall(spec)}
        sizes.update(row_sizes)
        range_match = RANGE.fullmatch(spec)
        if range_match:
            ranges.add(tuple(normalise(number + "rem") for number in range_match.groups()))
        sample = FONT_SIZE.search(row.group("style"))
        if not sample or normalise(sample.group(1)) not in row_sizes:
            failures.append(
                f"ramp sample {row.group('spec')!r} does not render at the size it documents"
            )
    return sizes, ranges, failures


def size_is_documented(value: str, sizes: set[str], ranges: set[tuple[str, str]]) -> bool:
    value = normalise(value)
    if value in sizes:
        return True
    match = CLAMP.fullmatch(value)
    return bool(match and tuple(normalise(part) for part in match.groups()) in ranges)


def font_sizes_stay_on_the_documented_ramp() -> int:
    sizes, ranges, failures = documented_ramp()
    paths = [*REFERENCE_PAGES, *sorted(STYLE_ROOT.rglob("*.css"))]
    if len(paths) == len(REFERENCE_PAGES):
        failures.append(f"no source stylesheets were found under {shown(STYLE_ROOT)}")
    seen_held = set()
    declarations = 0
    for path in paths:
        if not path.is_file():
            failures.append(f"{shown(path)} is missing, so its font sizes were not checked")
            continue
        original = path.read_text(encoding="utf-8")
        css = page_css(path, original)
        lines = original.splitlines()
        for match in FONT_SIZE.finditer(css):
            declarations += 1
            if size_is_documented(match.group(1), sizes, ranges):
                continue
            number = css.count("\n", 0, match.start()) + 1
            source = lines[number - 1].strip()
            key = (shown(path), source)
            if key in HELD_OFF_RAMP:
                seen_held.add(key)
                continue
            failures.append(
                f"{shown(path)}:{number} uses {match.group(1).strip()}, outside section 02's "
                "type ramp; the screen gains an unreviewed hierarchy level"
            )
    for path, source in sorted(HELD_OFF_RAMP - seen_held):
        failures.append(f"held off-ramp declaration disappeared from {path}; remove its baseline row: {source}")
    if failures:
        print("FAIL check-type-ramp:")
        for failure in failures:
            print("  " + failure)
        return 1
    print(
        "check-type-ramp: passed; 13 ramp rows, %d rem sizes and %d declarations; "
        "%d pre-existing departures held without growth"
        % (len(sizes), declarations, len(seen_held))
    )
    return 0


if __name__ == "__main__":
    sys.exit(font_sizes_stay_on_the_documented_ramp())
