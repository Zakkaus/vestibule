#!/usr/bin/env python3
"""Nothing a viewer reads is written in a component.

The plan requires every word on screen to come from the locale tables, with no
literals left in the code, and phase seven adds twelve more screens. Constants
get inlined while a screen is being built and nobody goes back for them, so this
freezes the rule while it still holds: three screens, zero violations.

What it reads, and why only these. A bare string in aria-label, title,
placeholder or alt is read aloud or shown on hover, and never reaches a
translator. A bare word between JSX tags is on the screen. Both are unambiguous.
Everything else a component might hold — a class name, a test id, a route — is a
string that no viewer sees, and demanding t() around those would make the check
noise and then make it dead.

Usage: check-console-copy.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
COMPONENTS = ROOT / "web" / "src"
LITERAL_ATTRIBUTE = re.compile(
    r"""\b(?P<attribute>aria-label|title|placeholder|alt)\s*=\s*
        (?:\{\s*)?(?P<quote>["'`])
        (?P<value>(?:\\.|(?! (?P=quote) ).)*)
        (?P=quote)(?:\s*\})?""",
    re.S | re.X,
)
JSX_TEXT = re.compile(
    r"""<(?P<tag>[A-Za-z][A-Za-z0-9._:-]*)\b[^<>]*>
        (?P<text>[^<>{}]*[A-Za-z一-鿿][^<>{}]*)
        </(?P=tag)\s*>""",
    re.S | re.X,
)


def line_number(source: str, offset: int) -> int:
    return source.count("\n", 0, offset) + 1


def main() -> int:
    if not COMPONENTS.exists():
        print("FAIL check-console-copy: web/src is not there")
        print("The console lives in this repository. A check that shrugs when it")
        print("cannot find what it checks reports success for everything after.")
        return 1

    findings = []
    scanned = 0
    for path in sorted(COMPONENTS.rglob("*.tsx")):
        scanned += 1
        name = path.relative_to(ROOT)
        source = path.read_text(encoding="utf-8")
        for match in LITERAL_ATTRIBUTE.finditer(source):
            quote = match.group("quote")
            value = match.group("value")
            if quote == "`" and "${" in value:
                continue
            findings.append(
                "  %s:%d %s=%s%s%s is not from the locale table"
                % (
                    name,
                    line_number(source, match.start()),
                    match.group("attribute"),
                    quote,
                    value,
                    quote,
                )
            )
        for match in JSX_TEXT.finditer(source):
            text = match.group("text").strip()
            if not re.search(r"[A-Za-z一-鿿]", text):
                continue
            findings.append(
                "  %s:%d the words %r are on screen and not from the "
                "locale table"
                % (name, line_number(source, match.start()), text[:50])
            )

    if not scanned:
        print("FAIL check-console-copy: no .tsx files found — has the console moved?")
        return 1

    if findings:
        print("FAIL check-console-copy: a viewer reads something a component wrote")
        for line in findings:
            print(line)
        print("\nEvery word on screen comes from the locale tables. A literal here")
        print("never reaches a translator, so it stays in one language for good.")
        return 1

    print("check-console-copy: passed; %d components, nothing a viewer reads is "
          "written in one" % scanned)
    return 0


if __name__ == "__main__":
    sys.exit(main())
