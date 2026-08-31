#!/usr/bin/env python3
"""Check the documents against themselves.

Prose rots in ways nothing else notices: a phase gets added and the sentence that
counts them does not, a document is renamed and three files keep pointing at the
old name. Both are mechanical, so both get a check rather than a habit.
"""
import re
import sys
from html.parser import HTMLParser
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


HOME_PATH = re.compile(r"(?<![\w/])/(?:home|Users)/[A-Za-z0-9._-]+/")


def check_home_paths(path: Path, text: str) -> None:
    """A public repository must not carry someone's working directory.

    Research notes arrived cited as "/home/<user>/code/memory/...". A reader
    cannot open that, so it is not evidence, and it publishes a directory
    layout for nothing.
    """
    for m in set(HOME_PATH.findall(text)):
        failures.append("%s: contains an absolute home path (%s...)"
                        % (path.relative_to(ROOT), m))


VOID_ELEMENTS = {"br", "hr", "img", "meta", "link", "input", "col", "source", "wbr"}


class _Balance(HTMLParser):
    """Track open elements so an unclosed one can be named with its line."""

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.stack: list[tuple[str, int]] = []
        self.problems: list[str] = []

    def handle_starttag(self, tag: str, attrs) -> None:
        if tag not in VOID_ELEMENTS:
            self.stack.append((tag, self.getpos()[0]))

    def handle_endtag(self, tag: str) -> None:
        if tag in VOID_ELEMENTS:
            return
        if not self.stack:
            self.problems.append("line %d: stray </%s>" % (self.getpos()[0], tag))
            return
        open_tag, open_line = self.stack[-1]
        if open_tag != tag:
            self.problems.append("line %d: </%s> closes <%s> opened on line %d"
                                 % (self.getpos()[0], tag, open_tag, open_line))
            return
        self.stack.pop()


def check_tag_balance(path: Path, text: str) -> None:
    """An unclosed tag renders anyway, so nothing else notices it.

    Twice now a missing </p> reached a commit: once written by hand, once by a
    patch. The page still displays, which is exactly why this needs a machine.
    """
    parser = _Balance()
    parser.feed(text)
    rel = path.relative_to(ROOT)
    for problem in parser.problems[:5]:
        failures.append("%s: %s" % (rel, problem))
    for tag, line in parser.stack[:5]:
        failures.append("%s: <%s> opened on line %d is never closed" % (rel, tag, line))


REF = re.compile(r"`((?:docs|web|scripts)/[A-Za-z0-9_./-]+\.(?:md|html|py|sh|ya?ml))`")


def check_links(path: Path) -> None:
    """Every document a present-tense document names has to exist."""
    for ref in sorted(set(REF.findall(path.read_text(encoding="utf-8")))):
        if not (ROOT / ref).exists():
            failures.append("%s: names %s, which does not exist" % (path.relative_to(ROOT), ref))


def check_screen_coverage() -> None:
    """The route table says it is exhaustive, so hold it to that.

    Every screen the design language lists has to be named by some row of the
    architecture's route table. A screen with no row is a screen nobody has
    worked out how to load or save, and the claim rots within weeks otherwise.
    """
    design = ROOT / "web" / "design.html"
    arch = ROOT / "docs" / "ARCHITECTURE.md"
    if not (design.exists() and arch.exists()):
        return
    d = design.read_text(encoding="utf-8")
    start = d.find("各屏职责")
    if start < 0:
        return
    table = d[d.find("<table", start):d.find("</table>", start)]
    screens = re.findall(r"<tr><td>([^<]+)</td>", table)
    a = arch.read_text(encoding="utf-8")
    first, last = a.find("| GET /livez"), a.find("这张表是穷举的")
    if first < 0 or last < 0:
        failures.append("architecture: the route table or its exhaustiveness claim is gone")
        return
    region = a[first:last]
    for screen in screens:
        if screen not in region:
            failures.append("no route names the %s screen, but the route table claims to be exhaustive" % screen)


def main() -> int:
    if not PLAN.exists():
        failures.append("docs/PLAN-v5.md is missing")
    else:
        check_plan_phases(PLAN.read_text(encoding="utf-8"))

    candidates = list((ROOT / "docs").rglob("*.md")) + [ROOT / "README.md", ROOT / "CONTRIBUTING.md"]
    candidates += sorted((ROOT / "web").glob("*.html"))
    for path in candidates:
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        check_home_paths(path, text)
        if path.suffix == ".html":
            check_tag_balance(path, text)
            continue
        check_headings(path, text)
        if str(path.relative_to(ROOT)) not in FORWARD_LOOKING:
            check_links(path)

    check_screen_coverage()

    if failures:
        for f in failures:
            print("FAIL check-docs: " + f, file=sys.stderr)
        return 1
    print("check-docs: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
