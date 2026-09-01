#!/usr/bin/env python3
"""Find theme branches that escaped the token layer.

The design language says a component-level dark rule is a symptom of a token the
layer cannot express, and nothing checked it. Two shipped in this library: a card
takes a different elevation in dark, and the overlay scrim goes from 40% to 60%.
Each is one value differing by theme, which is what a token is for.

The gap cost a whole dispatch. An agent given the prose as a hard constraint
found the library breaking it, could not tell which of the two was
authoritative, and stopped — correctly. A rule with no check is a preference,
and a preference cannot answer that question.

Usage: theme-leak.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

THEME = re.compile(r"prefers-color-scheme|\[data-theme")
# A selector that only ever styles the document element is the token layer.
# Exactly the document element and nothing after it. A descendant selector such
# as `:root[data-theme="dark"] [data-slot="card"]` is a component rule wearing a
# root prefix, which is the shape this check exists to find.
ROOT_ONLY = re.compile(r"^:root$"
                       r"|^:root:not\(\[data-theme=\"[a-z]+\"\]\)$"
                       r"|^:root\[data-theme=\"[a-z]+\"\]$"
                       r"|^\[data-theme=\"[a-z]+\"\]$"
                       r"|^html$|^:root,\s*\[data-theme=\"[a-z]+\"\]$")

NON_CUSTOM = re.compile(r"(?:^|;|\{)\s*(?!--)[a-zA-Z-]+\s*:")

failures: list[str] = []


def stylesheet(path: Path) -> str:
    text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        text = "".join(m.group(1) for m in re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S))
    return re.sub(r"/\*.*?\*/", lambda m: "\n" * m.group(0).count("\n"), text, flags=re.S)


def rules(css: str, inside_theme: bool = False, offset: int = 0):
    """Yield (selector, line, inside a theme media query, body) for every rule."""
    i = 0
    while i < len(css):
        brace = css.find("{", i)
        if brace < 0:
            return
        head = css[i:brace].strip()
        depth, j = 1, brace + 1
        while depth and j < len(css):
            if css[j] == "{":
                depth += 1
            elif css[j] == "}":
                depth -= 1
            j += 1
        body = css[brace + 1:j - 1]
        line = css.count("\n", 0, brace) + 1 + offset
        if head.startswith("@"):
            if "{" in body:  # a nesting at-rule: descend
                yield from rules(body, inside_theme or bool(THEME.search(head)),
                                 offset + css.count("\n", 0, brace + 1))
        else:
            yield head, line, inside_theme, body
        i = j


def check(path: Path) -> None:
    css = stylesheet(path)
    for selector, line, in_theme, body in rules(css):
        themed = in_theme or bool(THEME.search(selector))
        if not themed:
            continue
        # A block declaring nothing but custom properties IS the token layer,
        # whatever its selector reads like. Enumerating selector shapes instead
        # flagged every palette preset — `[data-palette="nord"][data-theme="dark"]`
        # declares --background and --card and nothing else, which is the thing
        # this check wants people to do.
        if not NON_CUSTOM.search(body):
            continue
        if all(ROOT_ONLY.match(part.strip()) for part in selector.split(",")):
            continue
        failures.append("%s: line %d: %s sets a value by theme outside the token layer"
                        % (path.name, line, " ".join(selector.split())[:72]))


def main(argv: list[str]) -> int:
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
        print("theme-leak: cannot read " + ", ".join(unreadable), file=sys.stderr)
        return 2
    for arg in argv:
        check(Path(arg))
    if failures:
        for f in failures:
            print("FAIL theme-leak: " + f)
        return 1
    print("theme-leak: passed; every theme difference lives in the token layer")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
