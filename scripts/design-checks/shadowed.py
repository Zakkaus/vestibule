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

A third one is not in a block at all. The same selector can be written twice in
one file, and then the property is shadowed across blocks with nothing inside
either block to see:

  .cbi-section-table  had a bleed rule giving it `calc(100% + var(--sp-4) * 2)`,
                      and a second rule further down setting `inline-size: 100%`.
                      The negative margin from the first still applied and the
                      width compensating for it did not, so every table in the
                      product lost a strip off its right edge while its left edge
                      looked correct.

So the file is checked twice: once inside each block, and once across blocks that
share a selector.

Usage: shadowed.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

# A repeat inside one block is a deliberate fallback only when the two declarations
# are two generations of the same value: an old one every engine parses, then a
# newer one that wins where it is understood and is dropped where it is not. The
# property name alone cannot tell you that. Exempting by name — `color`, `width`,
# `display` and six others were exempt here once — means `color: red; color: blue`
# passes, which is the exact mistake this script exists to catch, and it did pass.
# So decide on the values: the later one has to reach for something the earlier one
# does not.
MODERN = re.compile(
    r"color-mix\(|oklch\(|oklab\(|\blab\(|\blch\(|light-dark\(|clamp\(|"
    r"-webkit-|-moz-|fill-available|\b\d[\d.]*(dvh|svh|lvh|dvw|svw|lvw|dvb|svb|lvb)\b|"
    r"min-content|max-content|fit-content|\banchor\("
)


def is_fallback(earlier: str, later: str) -> bool:
    """True when the later value uses a construct the earlier one does not."""
    return bool(MODERN.search(later)) and not MODERN.search(earlier)

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
DECL = re.compile(r"(?:^|;)\s*(--[\w-]+|[a-zA-Z-]+)\s*:([^;]*)")


def check(path: Path) -> None:
    css = stylesheet(path)
    for m in BLOCK.finditer(css):
        selector = " ".join(m.group(1).split())[-70:]
        body = m.group(2)
        seen: dict[str, tuple[int, str]] = {}
        for d in DECL.finditer(body):
            name = d.group(1)
            value = d.group(2).strip()
            line = css.count("\n", 0, m.start(2) + d.start()) + 1
            if name in seen and not is_fallback(seen[name][1], value):
                failures.append(
                    "%s: %s declared twice in `%s` — line %d wins over line %d"
                    % (path.name, name, selector, line, seen[name][0]))
            seen[name] = (line, value)


KEYFRAME_SEL = re.compile(r"^(from|to|[\d.]+%)$")


# Two rules in different conditions are not shadowing each other — a media query
# overriding a base rule is the point of a media query. Only two rules in the same
# condition can silently shadow, so the condition is part of the key.
def check_across_blocks(path: Path) -> None:
    """Two blocks in one condition, same selector, both declaring one property."""
    css = stylesheet(path)
    # Keyframe bodies reuse `from`/`to` by design; they are not one selector.
    scrubbed = re.sub(r"@keyframes[^{]*\{(?:[^{}]|\{[^{}]*\})*\}",
                      lambda m: "\n" * m.group(0).count("\n"), css)
    conditions = []
    for m in re.finditer(r"@[\w-][^{;]*\{", scrubbed):
        i = m.end() - 1
        depth, j = 0, i
        while j < len(scrubbed):
            if scrubbed[j] == "{":
                depth += 1
            elif scrubbed[j] == "}":
                depth -= 1
                if depth == 0:
                    break
            j += 1
        conditions.append((m.start(), j, " ".join(m.group(0)[:-1].split())))

    seen: dict[tuple[str, str, str], tuple[int, str]] = {}
    for m in BLOCK.finditer(scrubbed):
        selector = " ".join(m.group(1).split())
        if not selector or selector.startswith("@") or KEYFRAME_SEL.match(selector):
            continue
        cond = " / ".join(c for a, b_, c in conditions if a <= m.start() < b_)
        for d in DECL.finditer(m.group(2)):
            name, value = d.group(1), d.group(2).strip()
            line = scrubbed.count("\n", 0, m.start(2) + d.start()) + 1
            key = (cond, selector, name)
            if key in seen and not is_fallback(seen[key][1], value):
                where = ("`%s` inside `%s`" % (selector[-50:], cond)) if cond else "`%s`" % selector[-70:]
                failures.append(
                    "%s: %s sets %s in two separate blocks — line %d wins over "
                    "line %d" % (path.name, where, name, line, seen[key][0]))
            seen[key] = (line, value)


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    for arg in argv:
        check(Path(arg))
        check_across_blocks(Path(arg))
    if failures:
        for f in failures:
            print("FAIL shadowed: " + f)
        return 1
    print("shadowed: passed; no declaration is overridden by a later one in its block")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
