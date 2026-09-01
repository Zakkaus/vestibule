#!/usr/bin/env python3
"""Enforce the two rules this design language states but never checked.

Section 02 says colour only expresses state, and hue enters from two places:
the token layer, and the five status classes. So the rule is not "no colour
values outside tokens" — an achromatic shadow is fine. The rule is about
**chroma**: nothing outside those two places may introduce a hue.

Section 03 says one radius seed with everything derived, and that "writing a
literal border-radius anywhere is a violation — this one needs a lint". It said
so and then shipped without one.

Usage: style-rules.py <file.css|file.html> ...
"""
import re
import sys
from pathlib import Path

# 0 and 50% are shapes, not steps on the scale. The pill used to be here too, as
# a literal `9999px`, and that hole let 20 literal pills accumulate across five
# files in two different spellings. It is `--radius-full` now, so nothing but a
# token, a calc, or a shape gets through.
RADIUS_OK = re.compile(r"^(0|50%|var\(|calc\(|inherit|initial|unset)")

# Physical box properties where a logical one exists. Non-negotiable 8 says logical
# by default, and this library had converted the ones people look at — `.ico`, the
# nav rail — while 85 `width`/`height` declarations sat in the shell and the
# component layer untouched, because nothing counted them. Media query features
# (`@media (min-width: …)`) are conditions, not declarations, and stay physical.
PHYSICAL = re.compile(
    r"(?:^|[;{])\s*((?:min-|max-)?(?:width|height)|"
    r"(?:margin|padding)-(?:left|right|top|bottom)|top|bottom|left|right)\s*:")
HEX = re.compile(r"#([0-9a-fA-F]{3,8})\b")
FUNC = re.compile(r"\b(rgba?|hsla?|oklch|oklab|lch|lab)\(([^()]*(?:\([^()]*\)[^()]*)*)\)")
STATUS_RULE = re.compile(r"\.status-[a-z]+[^{]*\{[^}]*\}", re.S)
# A palette scope is a token layer too: it redeclares seeds and surface values
# under an attribute instead of under :root, and hue is exactly what it exists
# to carry.
TOKEN_BLOCK = re.compile(
    r":root[^{]*\{[^}]*\}"
    r"|@media[^{]*prefers-color-scheme[^{]*\{(?:[^{}]|\{[^{}]*\})*\}"
    r"|\[data-theme[^{]*\{[^}]*\}"
    r"|\[data-palette[^{]*\{[^}]*\}", re.S)

failures: list[str] = []


def hex_has_hue(digits: str) -> bool:
    if len(digits) in (3, 4):
        digits = "".join(c * 2 for c in digits)
    if len(digits) < 6:
        return False
    r, g, b = (int(digits[i:i + 2], 16) for i in (0, 2, 4))
    return not (r == g == b)


def func_has_hue(name: str, args: str) -> bool:
    """True when the value carries chroma. Achromatic greys and shadows do not."""
    parts = [p for p in re.split(r"[\s,/]+", args.strip()) if p]
    if name.startswith("rgb"):
        nums = parts[:3]
        return len(set(nums)) > 1
    if name.startswith("hsl"):
        return len(parts) > 1 and not parts[1].startswith("0")
    if len(parts) > 1:                      # oklch / oklab / lch / lab: chroma is second
        try:
            return float(parts[1].rstrip("%")) > 0
        except ValueError:
            return True
    return True


def blank(match: re.Match) -> str:
    """Erase a span but keep its newlines, so reported line numbers stay true."""
    return "\n" * match.group(0).count("\n")


def stylesheet(path: Path) -> str:
    """Return the file with everything that is not CSS blanked out in place.

    Extracting the style blocks instead would renumber every line, and a
    checker that points at the wrong line is one nobody acts on.
    """
    text = path.read_text(encoding="utf-8")
    if path.suffix != ".css":
        # Both places CSS lives in a page: <style> blocks, and style="" attributes.
        # Keeping only the blocks left the demo markup unchecked, which in a design
        # book is most of the file — and the demos are what people copy. A checker
        # blind to the majority of its own subject reports a pass it did not earn.
        keep = [m.span(1) for m in re.finditer(r"<style[^>]*>(.*?)</style>", text, re.S)]
        keep += [m.span(1) for m in re.finditer(r'style="([^"]*)"', text)]
        keep.sort()
        out, pos = [], 0
        for start, end in keep:
            if start < pos:                 # a style="" inside a <style> block
                continue
            out.append("\n" * text.count("\n", pos, start))
            # An attribute has no braces, so wrap it in a rule the parsers below
            # recognise. It occupies no extra lines, so reported numbers stay true.
            frag = text[start:end]
            out.append(frag if "{" in frag or ":" not in frag else "x{" + frag + "}")
            pos = end
        out.append("\n" * text.count("\n", pos))
        text = "".join(out)
    return re.sub(r"/\*.*?\*/", blank, text, flags=re.S)


def check(path: Path) -> None:
    css = stylesheet(path)

    # Spacing comes from the ladder. 0 and auto are not steps on it.
    # em is type-relative padding (inline code, badges) and is not layout spacing,
    # so it does not come from the ladder.
    SPACE_OK = re.compile(r"^(0|auto|inherit|initial|unset|var\(|calc\(|env\(|-?[\d.]+(%|em)$)")
    for m in re.finditer(r"(?<![\w-])(padding|margin|gap|row-gap|column-gap)"
                         r"(?:-(?:block|inline)(?:-(?:start|end))?|-(?:top|right|bottom|left))?"
                         r"\s*:\s*([^;}]+)", css):
        # Collapse calc()/var() groups first: their inner spaces are not values.
        flat = re.sub(r"(?:calc|var|clamp|min|max)\([^()]*(?:\([^()]*\)[^()]*)*\)", "var(x)", m.group(2))
        for part in flat.split():
            if not SPACE_OK.match(part.strip()):
                failures.append("%s: literal %s %r on stylesheet line %d"
                                % (path.name, m.group(1), m.group(2).strip(),
                                   css.count("\n", 0, m.start()) + 1))
                break

    # Strip media/container conditions before looking for physical properties:
    # `@media (min-width: 60rem)` is a condition, not a declaration.
    no_conditions = re.sub(r"@(?:media|container)[^{]*", lambda m: " " * len(m.group(0)), css)
    for m in PHYSICAL.finditer(no_conditions):
        failures.append("%s: physical %s on stylesheet line %d — a logical property exists"
                        % (path.name, m.group(1), css.count("\n", 0, m.start()) + 1))

    for m in re.finditer(r"border-radius\s*:\s*([^;}]+)", css):
        for part in m.group(1).split():
            if not RADIUS_OK.match(part.strip()):
                failures.append("%s: literal border-radius %r on stylesheet line %d"
                                % (path.name, m.group(1).strip(), css.count("\n", 0, m.start()) + 1))
                break

    # Hue is permitted in the token layer and in the five status classes, nowhere else.
    outside = STATUS_RULE.sub(" ", TOKEN_BLOCK.sub(" ", css))
    for m in HEX.finditer(outside):
        if hex_has_hue(m.group(1)):
            failures.append("%s: hue outside the token layer: #%s" % (path.name, m.group(1)))
    for m in FUNC.finditer(outside):
        if "from var(" in m.group(2):        # derived from a token, not invented
            continue
        if func_has_hue(m.group(1), m.group(2)):
            failures.append("%s: hue outside the token layer: %s(%s)"
                            % (path.name, m.group(1), m.group(2).strip()[:40]))


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    for arg in argv:
        check(Path(arg))
    if failures:
        for f in failures:
            print("FAIL style-rules: " + f)
        return 1
    print("style-rules: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
