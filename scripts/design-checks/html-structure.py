#!/usr/bin/env python3
"""Structural faults in a hand-written page that still renders.

Each of these shipped in a real document and none of them looked wrong:

  unclosed tag        A <p> missing its close. The browser recovers, so the
                      page looks fine and the next element inherits the wrong
                      parent.
  markdown leak       Text written as **emphasis** in a file that is HTML, not
                      Markdown, so readers see the asterisks.
  duplicate id        Two elements sharing one id; every anchor to it lands on
                      the first.
  dead anchor         href="#x" with no element carrying that id.
  absolute home path  /home/<user>/... in a document meant for other people. It
                      is not a citation they can follow, and it publishes a
                      directory layout.

Usage: html-structure.py <file.html> ...
"""
import re
import sys
from html.parser import HTMLParser
from pathlib import Path

VOID = {"area", "base", "br", "col", "embed", "hr", "img", "input",
        "link", "meta", "param", "source", "track", "wbr"}
HOME = re.compile(r"(?<![\w/])/(?:home|Users)/[A-Za-z0-9._-]+/")
MARKDOWN = re.compile(r"\*\*[^*\n]{1,80}\*\*")

failures: list[str] = []


class Balance(HTMLParser):
    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.stack: list[tuple[str, int]] = []
        self.problems: list[str] = []

    def handle_starttag(self, tag, attrs):
        if tag not in VOID:
            self.stack.append((tag, self.getpos()[0]))

    def handle_endtag(self, tag):
        if tag in VOID:
            return
        if not self.stack:
            self.problems.append("line %d: stray </%s>" % (self.getpos()[0], tag))
            return
        if self.stack[-1][0] != tag:
            open_tag, open_line = self.stack[-1]
            self.problems.append("line %d: </%s> closes <%s> opened on line %d"
                                 % (self.getpos()[0], tag, open_tag, open_line))
            return
        self.stack.pop()


def check(path: Path) -> None:
    text = path.read_text(encoding="utf-8")
    name = path.name

    parser = Balance()
    parser.feed(text)
    for problem in parser.problems:
        failures.append("%s: %s" % (name, problem))
    for tag, line in parser.stack:
        failures.append("%s: <%s> opened on line %d is never closed" % (name, tag, line))

    ids: dict[str, int] = {}
    for m in re.finditer(r'\bid="([^"]+)"', text):
        line = text.count("\n", 0, m.start()) + 1
        if m.group(1) in ids:
            failures.append("%s: line %d: id %r already used on line %d"
                            % (name, line, m.group(1), ids[m.group(1)]))
        else:
            ids[m.group(1)] = line

    for m in re.finditer(r'href="#([^"]+)"', text):
        if m.group(1) and m.group(1) not in ids:
            failures.append("%s: line %d: anchor #%s has no target"
                            % (name, text.count("\n", 0, m.start()) + 1, m.group(1)))

    # Blank the script and style blocks but keep their newlines, or every line
    # number after the first one is wrong.
    prose = re.sub(r"<(script|style)[^>]*>.*?</\1>",
                   lambda m: "\n" * m.group(0).count("\n"), text, flags=re.S)
    for m in MARKDOWN.finditer(prose):
        failures.append("%s: line %d: Markdown emphasis in HTML: %s"
                        % (name, prose.count("\n", 0, m.start()) + 1, m.group(0)[:40]))
    for m in HOME.finditer(text):
        failures.append("%s: line %d: absolute home path (%s...)"
                        % (name, text.count("\n", 0, m.start()) + 1, m.group(0)))


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    for arg in argv:
        check(Path(arg))
    if failures:
        for f in failures:
            print("FAIL html-structure: " + f)
        return 1
    print("html-structure: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
