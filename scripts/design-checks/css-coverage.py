#!/usr/bin/env python3
"""Match the stylesheet against the pages that are supposed to demonstrate it.

Two failures, one in each direction, both seen in a real document:

  used but not defined   Two blocks carried class="seq". The stylesheet that
                         defined .seq lived in a sibling document, so every
                         step sequence collapsed into one run-on line. The page
                         still rendered, so nothing complained.

  defined but not used   .legend was defined across three rules and never
                         appeared in the markup once. A design language whose
                         examples are what people copy cannot afford a rule
                         with nothing to copy.

Usage: css-coverage.py <file.css|file.html> ...
Every argument is scanned for both definitions and uses, so a self-contained
page with an inline <style> can be checked on its own.
"""
import re
import sys
from pathlib import Path

# Utility prefixes a page may legitimately define without demonstrating each one.
IGNORE_PREFIXES = ("sr-", "u-", "is-", "has-")

# Attributes whose value is computed rather than written. The theme toggle calls
# setAttribute('data-theme', next) with a variable, so no literal appears
# anywhere for a checker to find; treating that as dead would mean deleting the
# theme mechanism. One entry, and it carries its reason.
IGNORE_ATTRS = {
    "data-theme",
    # Palette scopes are cycled by a button that holds the name in a variable,
    # so no literal reaches setAttribute either.
    "data-palette",
    # Read as content, never used as a styling hook. The palette samples carry
    # the token name and the sentence describing where it may appear, and the
    # accent picker carries its hue; a script reads all three to build the swatch.
    # Requiring a rule for them would mean writing dead CSS to satisfy a checker.
    "data-token", "data-use", "data-hue",
}

DEF = re.compile(r"\.([a-zA-Z][\w-]*)(?=[\s,:.>{\[])")
USE = re.compile(r'''class=(?:"(?P<d>[^"]*)"|'(?P<s>[^']*)')''')
# An attribute-based component system needs the same check in both directions.
# A checker written for class= alone does not see data-slot, and this library is
# mostly data-slot: textarea and select-trigger were both missing from one of the
# two stylesheets and rendered as unstyled browser defaults inside the document
# that specifies them.
# Every data-* vocabulary, not just data-slot. A demo written with
# data-variant="danger" against a library that calls it "destructive" rendered
# as an ordinary button and passed every check here, because the checker knew
# about classes and slots and nothing else. An attribute value is a name the
# stylesheet owns, so it gets the same treatment.
ATTR_DEF = re.compile(r'\[(data-[\w-]+)=["\']([\w-]+)["\']\]')
ATTR_USE = re.compile(r'(data-[\w-]+)=["\']([\w-]+)["\']')
# Set from script rather than written in markup.
ATTR_SCRIPT = re.compile(r'setAttribute\(\s*["\'](data-[\w-]+)["\']\s*,\s*["\']([\w-]+)["\']')
# A page that renders part of itself puts those class names in script, not in
# markup. .vl and .meta were reported as dead while a palette renderer was
# creating both on every load; deleting them would have broken the swatches.
# Same reason the style="" attributes had to be included: a checker blind to
# where its subject actually lives reports a pass, or a failure, it did not earn.
SPLIT = re.compile(r"[\s,'\"]+")
IDENT = re.compile(r"[A-Za-z][\w-]*")
SCRIPT_USE = re.compile(
    r"""className\s*=\s*['"]([^'"]*)['"]"""
    r"""|classList\.(?:add|remove|toggle|replace)\(([^)]*)\)"""
    r"""|setAttribute\(\s*['"]class['"]\s*,\s*['"]([^'"]*)['"]""")


def strip_comments(css: str) -> str:
    """A class name written in a comment is prose, not a definition."""
    return re.sub(r"/\*.*?\*/", " ", css, flags=re.S)


def read(path: Path) -> tuple[str, str]:
    """Return (stylesheet text, markup text) for a .css or .html file."""
    text = path.read_text(encoding="utf-8")
    if path.suffix == ".css":
        return strip_comments(text), ""
    styles = "\n".join(re.findall(r"<style[^>]*>(.*?)</style>", text, re.S))
    markup = re.sub(r"<style[^>]*>.*?</style>", "", text, flags=re.S)
    markup = re.sub(r"<!--.*?-->", " ", markup, flags=re.S)
    # A class or data attribute quoted inside <code> or <pre> is prose about some
    # other project, not a use on this page. Counting it hides a genuinely
    # undefined name behind one the document merely mentions.
    #
    # Blank the text and keep the tags: a highlighted code block is real markup
    # that this page styles — <pre><span class="k-val"> is a use, while
    # <code>class="k-val"</code> is prose. Removing the whole subtree could not
    # tell them apart and reported twenty-six live spans as dead CSS.
    def _text_only(m: "re.Match") -> str:
        inner = re.sub(r">[^<]*<", "><", m.group(2))
        return m.group(1) + inner + m.group(3)

    markup = re.sub(r"(<(code|pre)\b[^>]*>)(.*?)(</\2>)",
                    lambda m: m.group(1) + re.sub(r">[^<]*<", "><", m.group(3)) + m.group(4),
                    markup, flags=re.S)
    return strip_comments(styles), markup


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2

    defined: dict[str, str] = {}
    used: dict[str, str] = {}
    slot_defined: dict[tuple[str, str], str] = {}
    slot_used: dict[tuple[str, str], str] = {}
    for arg in argv:
        path = Path(arg)
        css, html = read(path)
        for name in DEF.findall(css):
            defined.setdefault(name, arg)
        for m_ in USE.finditer(html):
            attr = m_.group('d') if m_.group('d') is not None else m_.group('s')
            for name in attr.split():
                used.setdefault(name, arg)
        for pair in ATTR_DEF.findall(css):
            slot_defined.setdefault(pair, arg)
        for pair in ATTR_USE.findall(html):
            slot_used.setdefault(pair, arg)
        for pair in ATTR_SCRIPT.findall(html):
            slot_used.setdefault(pair, arg)
        for groups in SCRIPT_USE.findall(html):
            for group in groups:
                for name in SPLIT.split(group):
                    # Only accept things shaped like a class name. A variable
                    # passed to classList.add, or a selector string, is not one,
                    # and counting it invents a use that does not exist.
                    if IDENT.fullmatch(name):
                        used.setdefault(name, arg)

    problems = 0
    for name, where in sorted(used.items()):
        if name not in defined and not name.startswith(IGNORE_PREFIXES):
            print("FAIL used-but-not-defined: .%s (in %s)" % (name, where))
            problems += 1
    for name, where in sorted(defined.items()):
        if name not in used and not name.startswith(IGNORE_PREFIXES):
            print("FAIL defined-but-not-used: .%s (in %s)" % (name, where))
            problems += 1

    for (attr, val), where in sorted(slot_used.items()):
        # The ignore list has to hold in both directions. It held only for
        # defined-but-not-used, so an attribute declared as content still had to
        # have a rule written for it, which is the dead CSS the check exists to
        # prevent.
        if attr in IGNORE_ATTRS:
            continue
        if (attr, val) not in slot_defined:
            print('FAIL used-but-not-defined: [%s="%s"] (in %s)' % (attr, val, where))
            problems += 1
    for (attr, val), where in sorted(slot_defined.items()):
        if attr in IGNORE_ATTRS:
            continue
        if (attr, val) not in slot_used:
            print('FAIL defined-but-not-used: [%s="%s"] (in %s)' % (attr, val, where))
            problems += 1

    if problems:
        print("css-coverage: %d problem(s); %d defined, %d used"
              % (problems, len(defined), len(used)), file=sys.stderr)
        return 1
    print("css-coverage: passed; %d classes and %d data-* values, each defined and demonstrated"
          % (len(defined), len(slot_defined)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
