"""The CSS in a page, with everything that is not CSS blanked out in place.

A page holds CSS in two places: <style> blocks and style="" attributes. Reading only the blocks
leaves the demo markup unchecked, and in a design book the demos are most of the file — they are
also what people copy. A checker blind to the majority of its own subject reports a pass it did
not earn.

style-rules.py worked this out and said so in a comment. Its nine siblings kept reading blocks
only, so a hue, an undefined token, a repeated declaration or a percentage minimum written into an
attribute went unseen by all of them. This is that extractor, in one place, so the next check
written cannot inherit the old blindness.

Blanking in place rather than extracting keeps every line number true: a checker that points at
the wrong line is one nobody acts on.
"""

import re
from pathlib import Path

BLOCK = re.compile(r"<style[^>]*>(.*?)</style>", re.S)
ATTRIBUTE = re.compile(r'style="([^"]*)"')
COMMENT = re.compile(r"/\*.*?\*/", re.S)


def _blank(match: re.Match) -> str:
    return "\n" * match.group(0).count("\n")


def page_css(path: Path, text: str | None = None) -> str:
    """Return the file's CSS with non-CSS blanked, line numbers preserved."""
    if text is None:
        text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        keep = [m.span(1) for m in BLOCK.finditer(text)]
        keep += [m.span(1) for m in ATTRIBUTE.finditer(text)]
        keep.sort()
        out: list[str] = []
        position = 0
        for start, end in keep:
            if start < position:  # a style="" inside a <style> block
                continue
            out.append("\n" * text.count("\n", position, start))
            fragment = text[start:end]
            # An attribute has no braces, so wrap it in a rule the parsers recognise. The wrapper
            # adds no lines, so reported numbers stay true. The selector is unique per attribute:
            # a shared name made every attribute look like one rule declared over and over, and
            # the shadowing check reported each demo as shadowing the one above it.
            if "{" in fragment or ":" not in fragment:
                out.append(fragment)
            else:
                out.append("a%d{%s}" % (start, fragment))
            position = end
        out.append("\n" * text.count("\n", position))
        text = "".join(out)
    return COMMENT.sub(_blank, text)
