#!/usr/bin/env python3
"""A screen may only map error codes the API can actually answer with.

Each screen turns an API error code into a message key through its own small table.
A code the API never sends is a dead row: it looks like the failure is handled, the
copy exists and reads correctly, and the branch can never run. Its cost is the real
code sitting beside it unhandled, because whoever wrote the table believed it was
covered.

The moderation screen held two: authentication_required and session_expired, neither
of which appears in any writeError call. The codes the session layer does send,
authentication_expired and authentication_invalid, were absent, so an expired session
fell through to "moderation settings cannot be saved right now" -- a claim about the
settings service, from a failure in the session.

This checks one direction only, that every mapped code exists on the Go side. The
other direction, that every code an endpoint can answer with is mapped, needs to know
which endpoints a screen calls and is not attempted here; a screen is allowed to let
a code fall through to its generic message deliberately.

The names of both sides are the vocabulary, so this compares them literally. Finding
no mapping at all is a failure: that is how it would rot.
"""
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
FEATURES = ROOT / "web" / "src" / "features"
EMIT = re.compile(r'writeError\([^,]+,\s*[^,]+,\s*"([a-z_]+)"')
PRESENTER = re.compile(r"function [A-Za-z]*[Ee]rror(?:MessageKey|Presentation)\(.*?\n\}", re.S)
ERROR_CODE_SWITCH = re.compile(r"\bswitch\s*\(\s*error\.code\s*\)\s*\{")
TABLE = re.compile(r"(?:errorMessageKeys|keyByCode)[^=]*=\s*\{(.*?)\n\s*\};", re.S)
ROW = re.compile(r"^\s*([a-z_]{4,}):", re.M)

def error_code_switches(component: str):
    """Return switch bodies and whether any switch had an unbalanced brace."""
    bodies = []
    unbalanced = 0
    for opener in ERROR_CODE_SWITCH.finditer(component):
        depth = 1
        quote = None
        escaped = False
        comment = None
        index = opener.end()
        while index < len(component):
            char = component[index]
            next_char = component[index + 1] if index + 1 < len(component) else ""
            if comment == "line":
                if char == "\n":
                    comment = None
            elif comment == "block":
                if char == "*" and next_char == "/":
                    comment = None
                    index += 1
            elif quote is not None:
                if escaped:
                    escaped = False
                elif char == "\\":
                    escaped = True
                elif char == quote:
                    quote = None
            elif char == "/" and next_char == "/":
                comment = "line"
                index += 1
            elif char == "/" and next_char == "*":
                comment = "block"
                index += 1
            elif char in "\"'`":
                quote = char
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    bodies.append(component[opener.end():index])
                    break
            index += 1
        else:
            unbalanced += 1
    return bodies, unbalanced


def emitted_codes() -> set:
    listing = subprocess.run(
        ["grep", "-rl", "writeError", "internal", "--include=*.go"],
        cwd=ROOT, capture_output=True, text=True).stdout.split()
    codes = set()
    for name in listing:
        if name.endswith("_test.go"):
            continue
        codes |= set(EMIT.findall((ROOT / name).read_text()))
    return codes


def mapped_codes(component: str) -> set:
    codes = set()
    switch_bodies, _ = error_code_switches(component)
    for body in switch_bodies:
        codes |= set(re.findall(r'case "([a-z_]+)"', body))
        for table in TABLE.findall(body):
            codes |= set(ROW.findall(table))
    for body in PRESENTER.findall(component):
        for table in TABLE.findall(body):
            codes |= set(ROW.findall(table))
    for table in TABLE.findall(component):
        codes |= set(ROW.findall(table))
    return codes


def main() -> int:
    emitted = emitted_codes()
    if not emitted:
        print("FAIL check-error-maps-hold-real-codes: no writeError call was found in "
              "internal/, so no mapping can be judged against it")
        return 1

    failures = []
    rows = 0
    presenters = 0
    for screen in sorted(p for p in FEATURES.iterdir() if p.is_dir()):
        component = "".join(path.read_text() for path in sorted(screen.glob("*.tsx")))
        switch_bodies, unbalanced_switches = error_code_switches(component)
        switch_codes = set()
        for body in switch_bodies:
            switch_codes |= set(re.findall(r'case "([a-z_]+)"', body))
        codes = mapped_codes(component)
        named_presenter = PRESENTER.search(component) is not None
        has_error_switch = bool(switch_bodies) or unbalanced_switches > 0
        if named_presenter or has_error_switch:
            presenters += 1
            if not codes or unbalanced_switches or (
                has_error_switch and not switch_codes
            ):
                # A screen that presents errors and yields no code means the shapes this
                # reads for have moved. Zero rows overall would be caught below; this
                # catches losing most of them, which otherwise still reports a pass.
                failures.append(
                    "%s presents API errors and yields no mapped code, so the table shape "
                    "this reads has moved and most of the console is going unchecked"
                    % screen.relative_to(ROOT))
        rows += len(codes)
        for code in sorted(codes - emitted):
            failures.append(
                "%s maps %s, which no writeError call sends, so that branch cannot run "
                "and whatever the API does send falls through"
                % (screen.relative_to(ROOT), code))

    if rows == 0:
        print("FAIL check-error-maps-hold-real-codes: no screen was found mapping any "
              "error code, so this check read nothing")
        return 1
    for failure in failures:
        print("FAIL check-error-maps-hold-real-codes: %s" % failure)
    if failures:
        return 1
    print("check-error-maps-hold-real-codes: passed; %d mapped codes across %d screens "
          "that present errors, every one of them a code %d writeError calls can send"
          % (rows, presenters, len(emitted)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
