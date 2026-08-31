#!/usr/bin/env python3
"""Find declarations a later one in the same block silently overrides.

Two of these shipped in this library, and both were found by a person reading
the file rather than by any check:

  --ring     was a colour and a shadow sharing one name. The colour was declared
             later, so `box-shadow: var(--ring)` resolved to a bare colour, which
             is not a valid shadow, so the declaration was dropped and inputs had
             no focus ring at all. Nothing errored. Nothing looked wrong until
             someone tabbed through a form.

  --border   was raised for the dark theme by inserting a new declaration at the
             top of the theme block, above the original further down. Later wins.
             The change shipped, the screenshot was unchanged, and the same
             complaint came back a second time.

The second one is the reason this cannot be left to review: the fix looks right
in the diff. Only the block as a whole shows that it does nothing.

Usage: shadowed.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

# Properties whose repetition inside one block is a deliberate fallback for older
# engines rather than a mistake: the second declaration is meant to win where it
# parses and be ignored where it does not.
FALLBACK_OK = {"background", "background-image", "color", "width", "height",
               "inline-size", "block-size", "display", "position"}

failures: list[str] = []


def blank(m: re.Match) -> str:
    return "\n" * m.group(0).count("\n")


def stylesheet(path: Path) -> str:
    """Return the file with everything that is not CSS blanked, newlines kept."""
    text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        keep = [m.span(1) for m in re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S)]
        out, pos = [], 0
        for start, end in keep:
            out.append("\n" * text.count("\n", pos, start))
            out.append(text[start:end])
            pos = end
        out.append("\n" * text.count("\n", pos))
        text = "".join(out)
    return re.sub(r"/\*.*?\*/", blank, text, flags=re.S)


BLOCK = re.compile(r"([^{}]*)\{([^{}]*)\}")
DECL = re.compile(r"(?:^|;)\s*(--[\w-]+|[a-zA-Z-]+)\s*:")


def check(path: Path) -> None:
    css = stylesheet(path)
    for m in BLOCK.finditer(css):
        selector = " ".join(m.group(1).split())[-70:]
        body = m.group(2)
        seen: dict[str, int] = {}
        for d in DECL.finditer(body):
            name = d.group(1)
            line = css.count("\n", 0, m.start(2) + d.start()) + 1
            if name in seen and name.lower() not in FALLBACK_OK:
                failures.append(
                    "%s: %s declared twice in `%s` — line %d wins over line %d"
                    % (path.name, name, selector, line, seen[name]))
            seen[name] = line


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    for arg in argv:
        check(Path(arg))
    if failures:
        for f in failures:
            print("FAIL shadowed: " + f)
        return 1
    print("shadowed: passed; no declaration is overridden by a later one in its block")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
