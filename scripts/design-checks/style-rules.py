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
# The five status classes are the whole allowance. The first pattern here accepted any
# selector that merely began with one, descendants included, so `.status-ok .icon { color:
# #e00 }` put an invented hue on an arbitrary element inside a status region and was waved
# through. A selector list of status classes is still one rule about those classes.
STATUS_RULE = re.compile(r"(?:\.status-[a-z]+\s*,\s*)*\.status-[a-z]+\s*\{[^}]*\}", re.S)
# A relative colour derives from a token, but only the channels it leaves as keywords. In
# oklch(from var(--x) l 0.3 25) the lightness is derived and the chroma and hue are literals,
# which is a hue invented outside the token layer through the idiom that looks most like
# staying inside it.
RELATIVE_CHANNELS = {"oklch": (2, 3), "lch": (2, 3), "oklab": (2, 3), "lab": (2, 3),
                     "hsl": (1, 2), "hsla": (1, 2), "rgb": (1, 2), "rgba": (1, 2)}
# Hue can also be written where CSS is not: an SVG presentation attribute is markup, so the
# extractor never sees it, and a diagram in the page that documents the hue rule was free to
# break it. currentColor, none and a token reference are not literals.
SVG_PAINT = re.compile(r"\b(fill|stroke|stop-color|flood-color|lighting-color)=\"([^\"]+)\"")
PAINT_OK = re.compile(r"^(none|currentcolor|inherit|transparent|url\(|var\()", re.I)
# A hue does not have to be written in numbers. `color: crimson` passed every check here: the
# detector above knows hexadecimal and colour functions, and CSS also has a hundred and forty
# names. The achromatic ones are left out for the same reason hex_has_hue lets #333 through --
# what the rule forbids is a hue, not a literal.
NAMED_ACHROMATIC = {
    "transparent", "currentcolor", "white", "black", "gray", "grey", "silver", "gainsboro",
    "whitesmoke", "dimgray", "dimgrey", "darkgray", "darkgrey", "lightgray", "lightgrey",
    "slategray", "slategrey", "darkslategray", "darkslategrey", "lightslategray", "lightslategrey",
}
NAMED_HUES = {
    "aliceblue", "antiquewhite", "aqua", "aquamarine", "azure", "beige", "bisque", "blanchedalmond",
    "blue", "blueviolet", "brown", "burlywood", "cadetblue", "chartreuse", "chocolate", "coral",
    "cornflowerblue", "cornsilk", "crimson", "cyan", "darkblue", "darkcyan", "darkgoldenrod",
    "darkgreen", "darkkhaki", "darkmagenta", "darkolivegreen", "darkorange", "darkorchid",
    "darkred", "darksalmon", "darkseagreen", "darkslateblue", "darkturquoise", "darkviolet",
    "deeppink", "deepskyblue", "dodgerblue", "firebrick", "floralwhite", "forestgreen", "fuchsia",
    "ghostwhite", "gold", "goldenrod", "green", "greenyellow", "honeydew", "hotpink", "indianred",
    "indigo", "ivory", "khaki", "lavender", "lavenderblush", "lawngreen", "lemonchiffon",
    "lightblue", "lightcoral", "lightcyan", "lightgoldenrodyellow", "lightgreen", "lightpink",
    "lightsalmon", "lightseagreen", "lightskyblue", "lightsteelblue", "lightyellow", "lime",
    "limegreen", "linen", "magenta", "maroon", "mediumaquamarine", "mediumblue", "mediumorchid",
    "mediumpurple", "mediumseagreen", "mediumslateblue", "mediumspringgreen", "mediumturquoise",
    "mediumvioletred", "midnightblue", "mintcream", "mistyrose", "moccasin", "navajowhite", "navy",
    "oldlace", "olive", "olivedrab", "orange", "orangered", "orchid", "palegoldenrod", "palegreen",
    "paleturquoise", "palevioletred", "papayawhip", "peachpuff", "peru", "pink", "plum",
    "powderblue", "purple", "rebeccapurple", "red", "rosybrown", "royalblue", "saddlebrown",
    "salmon", "sandybrown", "seagreen", "seashell", "sienna", "skyblue", "slateblue", "snow",
    "springgreen", "steelblue", "tan", "teal", "thistle", "tomato", "turquoise", "violet", "wheat",
    "yellow", "yellowgreen",
}
DECLARATION = re.compile(r"(?<![\w-])([-\w]+)\s*:\s*([^;{}]*)")
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
        if "from var(" in m.group(2):
            # Derived from a token in the channels it leaves alone. A numeric literal in a
            # chroma or hue position is invented, whatever it was derived from.
            channels = m.group(2).split("/")[0].split()
            after = channels[2:] if len(channels) > 2 else []
            invented = [i for i in RELATIVE_CHANNELS.get(m.group(1), ())
                        if i < len(after) and re.fullmatch(r"-?[\d.]+%?", after[i])]
            if invented:
                failures.append("%s: hue outside the token layer: %s(%s) invents a channel it did "
                                "not derive" % (path.name, m.group(1), m.group(2).strip()[:40]))
            continue
        if func_has_hue(m.group(1), m.group(2)):
            failures.append("%s: hue outside the token layer: %s(%s)"
                            % (path.name, m.group(1), m.group(2).strip()[:40]))
    if path.suffix != ".css":
        raw = path.read_text(encoding="utf-8")
        for m in SVG_PAINT.finditer(raw):
            value = m.group(2).strip()
            if PAINT_OK.match(value):
                continue
            if HEX.search(value) or FUNC.search(value) or value.lower() in NAMED_HUES:
                failures.append("%s: hue outside the token layer: %s=\"%s\" on line %d — an SVG "
                                "presentation attribute is still a colour"
                                % (path.name, m.group(1), value[:24],
                                   raw.count("\n", 0, m.start()) + 1))

    for m in DECLARATION.finditer(outside):
        for token in re.split(r"[\s,()/]+", m.group(2).lower()):
            name = token.strip()
            if name in NAMED_HUES and name not in NAMED_ACHROMATIC:
                failures.append("%s: hue outside the token layer: %s on stylesheet line %d"
                                % (path.name, name, outside.count("\n", 0, m.start()) + 1))
                break


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
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
        print("style-rules: cannot read " + ", ".join(unreadable), file=sys.stderr)
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
