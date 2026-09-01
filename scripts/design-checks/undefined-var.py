#!/usr/bin/env python3
"""Find custom properties a stylesheet reads but nothing defines.

A `var(--x)` with no definition and no fallback does not fall back to some
default: the whole declaration becomes invalid and is dropped. Nothing errors,
nothing warns, and the property simply is not applied.

It shipped here. A mechanical pass converted every literal spacing value in
`docs/document.css` to `var(--sp-N)`, but that sheet is a separate system with
its own `:root` and never got the ladder. Every padding, margin and gap in the
file resolved to nothing at once, so the whole document rendered with zero
spacing — and the overflow check still passed, because a page with no spacing
does not overflow. Only opening it showed anything was wrong.

The style-rules checker cannot see this: the file is entirely compliant. It
references the scale exactly as it should. What is missing is the scale.

Pass every stylesheet that ships together in one invocation. Definitions pool
across files for the same reason they do in css-coverage: run file by file and
every cross-file reference looks like a violation.

Usage: undefined-var.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

# Properties a consumer is expected to define. A reference to one of these is a
# documented extension point, not a mistake — but it must carry a fallback.
EXTENSION_POINTS: set[str] = set()

failures: list[str] = []


def blank(m: re.Match) -> str:
    return "\n" * m.group(0).count("\n")


def stylesheet(path: Path) -> str:
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


DEF = re.compile(r"(--[\w-]+)\s*:")
# A name is not "defined" everywhere just because it is defined somewhere. A token
# declared only inside a dark block and read from an unconditional rule resolves to
# nothing in light mode, and the whole declaration is dropped — the same failure the
# set difference below exists to catch, one scope down. So collect the names that
# appear ONLY under a theme condition and report a reference from outside one.
THEME_BLOCK = re.compile(r"@media[^{]*prefers-color-scheme[^{]*\{|\[data-theme=")


def theme_spans(css: str) -> list[tuple[int, int]]:
    """Character ranges governed by a theme condition, by brace matching."""
    spans = []
    for m in THEME_BLOCK.finditer(css):
        i = css.find("{", m.start())
        if i < 0:
            continue
        depth, j = 0, i
        while j < len(css):
            if css[j] == "{":
                depth += 1
            elif css[j] == "}":
                depth -= 1
                if depth == 0:
                    break
            j += 1
        spans.append((m.start(), j))
    return spans
# A reference with a comma has a fallback; those are deliberate and allowed.
USE = re.compile(r"var\(\s*(--[\w-]+)\s*(,)?")


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2

    defined: set[str] = set()
    theme_only: set[str] = set()
    unconditional: set[str] = set()
    uses: list[tuple[str, str, int, bool]] = []
    for arg in argv:
        path = Path(arg)
        css = stylesheet(path)
        defined |= set(DEF.findall(css))
        spans = theme_spans(css)
        for m in DEF.finditer(css):
            bucket = theme_only if any(a <= m.start() < b for a, b in spans) else unconditional
            bucket.add(m.group(1))
        for m in USE.finditer(css):
            if m.group(2):                     # has a fallback
                continue
            in_theme = any(a <= m.start() < b for a, b in spans)
            uses.append((m.group(1), path.name, css.count("\n", 0, m.start()) + 1, in_theme))

    for name, where, line, in_theme in uses:
        if name not in defined and name not in EXTENSION_POINTS:
            failures.append("%s: line %d reads %s, which nothing defines — "
                            "the whole declaration is dropped" % (where, line, name))
        elif (not in_theme and name in theme_only and name not in unconditional
              and name not in EXTENSION_POINTS):
            failures.append("%s: line %d reads %s from an unconditional rule, but %s is "
                            "declared only inside a theme block — it resolves to nothing "
                            "in the other theme" % (where, line, name, name))

    if failures:
        for f in sorted(set(failures)):
            print("FAIL undefined-var: " + f)
        return 1
    print("undefined-var: passed; %d custom properties, every reference resolves"
          % len(defined))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
