#!/usr/bin/env python3
"""Check the documents against themselves.

Prose rots in ways nothing else notices: a phase gets added and the sentence that
counts them does not, a document is renamed and three files keep pointing at the
old name. Both are mechanical, so both get a check rather than a habit.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLAN = ROOT / "docs" / "PLAN-v5.md"

# A plan and an architecture describe the target state, so they are allowed to
# name files that do not exist yet. A README describes the present and is not.
FORWARD_LOOKING = {"docs/PLAN-v5.md", "docs/ARCHITECTURE.md"}

CN_NUM = {
    "零": 0, "一": 1, "二": 2, "三": 3, "四": 4, "五": 5,
    "六": 6, "七": 7, "八": 8, "九": 9, "十": 10,
    "十一": 11, "十二": 12, "十三": 13, "十四": 14, "十五": 15,
}
failures: list[str] = []


def check_plan_phases(text: str) -> None:
    """The phase table, the phase sections and any prose count must agree."""
    sections = re.findall(r"^### 阶段([零一二三四五六七八九十]+) · (.+)$", text, re.M)
    rows = re.findall(r"^\| ([零一二三四五六七八九十]+) \| `([^`]+)` \|", text, re.M)
    if not sections:
        failures.append("plan: no phase sections found")
        return
    if len(rows) != len(sections):
        failures.append("plan: %d rows in the phase table, %d phase sections" % (len(rows), len(sections)))
    elif [r[0] for r in rows] != [s[0] for s in sections]:
        failures.append("plan: the phase table and the phase sections are in different orders")
    for m in re.finditer(r"([零一二三四五六七八九十]+)个阶段", text):
        stated = CN_NUM.get(m.group(1))
        # "一个阶段一个分支" states a ratio, not a total.
        if stated in (None, 1) or stated == len(sections):
            continue
        failures.append('plan: prose says "%s个阶段" but there are %d' % (m.group(1), len(sections)))


HEADING = re.compile(r"^(#{1,6}) .*$", re.M)


def check_headings(path: Path, text: str) -> None:
    """A heading line carrying a second heading marker is a botched merge.

    Applying a patch across a moved region produced `### A### A` on one line.
    It renders as a heading, so it survives a read-through.
    """
    for m in HEADING.finditer(text):
        line = m.group(0)
        if "#" in line[len(m.group(1)):].lstrip():
            failures.append("%s: heading line contains a second marker: %s"
                            % (path.relative_to(ROOT), line[:60]))


REF = re.compile(r"`((?:docs|web|scripts)/[A-Za-z0-9_./-]+\.(?:md|html|py|sh|ya?ml))`")


def check_links(path: Path) -> None:
    """Every document a present-tense document names has to exist."""
    for ref in sorted(set(REF.findall(path.read_text(encoding="utf-8")))):
        if not (ROOT / ref).exists():
            failures.append("%s: names %s, which does not exist" % (path.relative_to(ROOT), ref))


def main() -> int:
    if not PLAN.exists():
        failures.append("docs/PLAN-v5.md is missing")
    else:
        check_plan_phases(PLAN.read_text(encoding="utf-8"))

    candidates = list((ROOT / "docs").rglob("*.md")) + [ROOT / "README.md", ROOT / "CONTRIBUTING.md"]
    for path in candidates:
        if not path.exists():
            continue
        check_headings(path, path.read_text(encoding="utf-8"))
        if str(path.relative_to(ROOT)) not in FORWARD_LOOKING:
            check_links(path)

    if failures:
        for f in failures:
            print("FAIL check-docs: " + f, file=sys.stderr)
        return 1
    print("check-docs: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
