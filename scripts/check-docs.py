#!/usr/bin/env python3
"""Check this repository's documents against themselves.

Structure, style rules and coverage of the two pages belong to the vendored
design-system checks in scripts/design-checks. What is left here is what only
this repository knows: its plan, its route table, and its own cross-references.

Each check exists because the thing it catches shipped once and looked fine.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLAN = ROOT / "docs" / "PLAN-v5.md"

# A plan and an architecture describe the target state, so they may name files
# that do not exist yet. A README describes the present and may not.
FORWARD_LOOKING = {"docs/PLAN-v5.md", "docs/ARCHITECTURE.md"}

CN_NUM = {"零": 0, "一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6,
          "七": 7, "八": 8, "九": 9, "十": 10, "十一": 11, "十二": 12,
          "十三": 13, "十四": 14, "十五": 15}

HEADING = re.compile(r"^(#{1,6}) .*$", re.M)
HOME_PATH = re.compile(r"(?<![\w/])/(?:home|Users)/[A-Za-z0-9._-]+/")
REF = re.compile(r"`((?:docs|web|scripts)/[A-Za-z0-9_./-]+\.(?:md|html|py|sh|ya?ml))`")

failures: list[str] = []


def check_phase_count(path: Path, text: str, real: int) -> None:
    """Any document stating how many phases there are must state the real number.

    The plan was checked and nothing else was, so the README went on saying nine
    after the count reached eleven. A number is wrong in whichever file it sits.
    """
    for m in re.finditer(r"([零一二三四五六七八九十]+)个阶段", text):
        stated = CN_NUM.get(m.group(1))
        if stated in (None, 1) or stated == real:
            continue  # "一个阶段一个分支" states a ratio, not a total
        failures.append('%s: says "%s个阶段" but there are %d'
                        % (path.relative_to(ROOT), m.group(1), real))
    for m in re.finditer(r"\b(nine|ten|eleven|twelve|thirteen)\s+phases\b", text, re.I):
        word = {"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13}[m.group(1).lower()]
        if word != real:
            failures.append('%s: says "%s phases" but there are %d'
                            % (path.relative_to(ROOT), m.group(1), real))


def phase_count(text: str) -> int:
    return len(re.findall(r"^### 阶段([零一二三四五六七八九十]+) · (.+)$", text, re.M))


def check_plan_phases(text: str) -> None:
    """The phase table, the phase sections and any prose count must agree."""
    sections = re.findall(r"^### 阶段([零一二三四五六七八九十]+) · (.+)$", text, re.M)
    rows = re.findall(r"^\| ([零一二三四五六七八九十]+) \| `([^`]+)` \|", text, re.M)
    if not sections:
        failures.append("plan: no phase sections found")
        return
    if len(rows) != len(sections):
        failures.append("plan: %d rows in the phase table, %d phase sections"
                        % (len(rows), len(sections)))
    elif [r[0] for r in rows] != [s[0] for s in sections]:
        failures.append("plan: the phase table and the phase sections are in different orders")


def check_headings(path: Path, text: str) -> None:
    """A heading line carrying a second marker is a botched patch application.

    Applying a patch across a moved region produced `### A### A` on one line. It
    renders as a heading, so a read-through does not catch it.
    """
    for m in HEADING.finditer(text):
        if "#" in m.group(0)[len(m.group(1)):].lstrip():
            failures.append("%s: heading line contains a second marker: %s"
                            % (path.relative_to(ROOT), m.group(0)[:60]))


def check_home_paths(path: Path, text: str) -> None:
    """A public repository must not carry someone's working directory.

    Research arrived cited as an absolute path under a home directory. No reader
    can open it, so it is not evidence, and it publishes a directory layout for
    nothing. The vendored html-structure check covers the pages; this covers the
    Markdown, which nothing else reads.
    """
    for hit in sorted(set(HOME_PATH.findall(text))):
        failures.append("%s: contains an absolute home path (%s...)"
                        % (path.relative_to(ROOT), hit))


def check_links(path: Path, text: str) -> None:
    """Every document a present-tense document names has to exist."""
    for ref in sorted(set(REF.findall(text))):
        if not (ROOT / ref).exists():
            failures.append("%s: names %s, which does not exist"
                            % (path.relative_to(ROOT), ref))


def check_screen_coverage() -> None:
    """The route table calls itself exhaustive, so hold it to that.

    Declaring it exhaustive immediately exposed four screens with no write path.
    Without the check the claim would have been true for about a fortnight.
    """
    design = ROOT / "web" / "design.html"
    arch = ROOT / "docs" / "ARCHITECTURE.md"
    if not (design.exists() and arch.exists()):
        return
    d = design.read_text(encoding="utf-8")
    start = d.find("各屏职责")
    if start < 0:
        return
    screens = re.findall(r"<tr><td>([^<]+)</td>",
                         d[d.find("<table", start):d.find("</table>", start)])
    a = arch.read_text(encoding="utf-8")
    first, last = a.find("| GET /livez"), a.find("这张表是穷举的")
    if first < 0 or last < 0:
        failures.append("architecture: the route table or its exhaustiveness claim is gone")
        return
    region = a[first:last]
    for screen in screens:
        if screen not in region:
            failures.append("no route names the %s screen, but the route table "
                            "claims to be exhaustive" % screen)


def main() -> int:
    phases = 0
    if not PLAN.exists():
        failures.append("docs/PLAN-v5.md is missing")
    else:
        plan_text = PLAN.read_text(encoding="utf-8")
        phases = phase_count(plan_text)
        check_plan_phases(plan_text)

    documents = list((ROOT / "docs").rglob("*.md")) + [
        ROOT / "README.md",
        ROOT / "README.zh-CN.md",   # it drifted to "八个阶段尚未开始" while unchecked
        ROOT / "CONTRIBUTING.md",
    ]
    for path in documents:
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        check_headings(path, text)
        check_home_paths(path, text)
        check_phase_count(path, text, phases)
        if str(path.relative_to(ROOT)) not in FORWARD_LOOKING:
            check_links(path, text)

    check_screen_coverage()

    if failures:
        for f in failures:
            print("FAIL check-docs: " + f, file=sys.stderr)
        return 1
    print("check-docs: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
