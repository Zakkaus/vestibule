#!/usr/bin/env python3
"""A console handler acts on the chat it authorised, and on no other.

Every per-group handler takes the chat from its path, authorises the session against that
chat, and passes the same identifier on. Nothing said it had to. A handler that authorises
one chat and then reads a second identifier -- from a request body, a query parameter, or a
second path segment -- would pass every existing check: the session is real, the caller does
administer a group, and check-mutations-authorise sees an authorisation happen. It would
simply act somewhere the caller was never checked against.

This is a zero being frozen rather than a defect being fixed. All ten handlers already do
this, and the moment that is true is the cheapest moment to require it.

The comparison is structural: only known per-group service methods are inspected, and
the argument slot carrying the group is checked as a complete expression. Passing a
differently-named variable is exactly the shape being refused, while similarly named
fields or unrelated calls remain outside this bounded analysis.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
HANDLERS = ROOT / "internal" / "console" / "api"
FUNCTION_START = re.compile(r"^func \(s \*Server\) ([A-Za-z0-9]+)\(", re.M)
AUTHORISES = re.compile(r"authorizedSession\(writer, request, ([A-Za-z][A-Za-z0-9]*),")
PASSES = re.compile(r"(?<![A-Za-z])(?:GroupID|ChatID):\s*([A-Za-z][A-Za-z0-9]*)")
# The lookbehind keeps AdminLogChatID and its kind out: they name a destination for a
# notice, not the chat being acted on, and a handler holding one is not a violation.

# These are the service calls whose positional argument is the authorised group. Keeping
# the receiver and slot explicit avoids treating arbitrary variables named "group" as IDs.
GROUP_ARGUMENTS = {
    ("verification", "ConsoleAudit"): 1,
    ("verification", "ConsoleQueue"): 1,
    ("rules", "ListRules"): 1,
    ("rules", "ReplaceRules"): 1,
    ("rules", "UpdateRule"): 1,
    ("settings", "Settings"): 0,
    ("settings", "Update"): 0,
}
SERVICE_CALL = re.compile(r"\bs\.(verification|rules|settings)\.([A-Za-z][A-Za-z0-9_]*)\s*\(")
IDENTIFIER = re.compile(r"[A-Za-z_][A-Za-z0-9_]*\Z")


def call_arguments(text: str, opening: int):
    """Return top-level call arguments, or None for an incomplete call."""
    depth = 0
    start = opening + 1
    pieces = []
    quote = None
    escaped = False
    for index in range(opening, len(text)):
        char = text[index]
        if quote:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == quote:
                quote = None
            continue
        if char in ('"', "'", "`"):
            quote = char
            continue
        if char == "(":
            depth += 1
        elif char == ")":
            depth -= 1
            if depth == 0:
                pieces.append(text[start:index])
                return [piece.strip() for piece in pieces if piece.strip()]
        elif char == "," and depth == 1:
            pieces.append(text[start:index])
            start = index + 1
    return None


def handler_functions(text: str):
    """Yield complete Server handler bodies with a small brace-aware scanner."""
    for match in FUNCTION_START.finditer(text):
        opening = text.find("{", match.end())
        if opening < 0:
            continue
        depth = 0
        quote = None
        escaped = False
        line_comment = False
        block_comment = False
        index = opening
        while index < len(text):
            char = text[index]
            next_char = text[index + 1] if index + 1 < len(text) else ""
            if line_comment:
                if char == "\n":
                    line_comment = False
            elif block_comment:
                if char == "*" and next_char == "/":
                    block_comment = False
                    index += 1
            elif quote:
                if quote != "`" and escaped:
                    escaped = False
                elif quote != "`" and char == "\\":
                    escaped = True
                elif char == quote:
                    quote = None
            elif char == "/" and next_char == "/":
                line_comment = True
                index += 1
            elif char == "/" and next_char == "*":
                block_comment = True
                index += 1
            elif char in ('"', "'", "`"):
                quote = char
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    yield match.group(1), text[match.start():index + 1]
                    break
            index += 1


def main() -> int:
    if not HANDLERS.is_dir():
        print("FAIL check-handlers-act-on-what-they-authorised: %s does not exist, so this "
              "check read nothing" % HANDLERS)
        return 1

    failures = []
    checked = 0
    for path in sorted(HANDLERS.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        text = path.read_text()
        for name, body in handler_functions(text):
            authorised = AUTHORISES.search(body)
            if not authorised:
                continue
            checked += 1
            for call in SERVICE_CALL.finditer(body):
                service, method = call.group(1), call.group(2)
                slot = GROUP_ARGUMENTS.get((service, method))
                if slot is None:
                    continue
                args = call_arguments(body, call.end() - 1)
                if args is None or slot >= len(args):
                    continue
                passed = args[slot]
                if not IDENTIFIER.fullmatch(passed) or passed != authorised.group(1):
                    failures.append(
                        "%s: %s authorises %s but passes %s to s.%s.%s; a handler must "
                        "act on the chat it was checked against"
                        % (path.relative_to(ROOT), name, authorised.group(1),
                           passed, service, method))
            for passed in sorted(set(PASSES.findall(body))):
                if passed != authorised.group(1):
                    failures.append(
                        "%s: %s authorises %s and then names %s as the chat to act on; a "
                        "handler must act on the chat it was checked against"
                        % (path.relative_to(ROOT), name, authorised.group(1), passed))

    if checked == 0:
        print("FAIL check-handlers-act-on-what-they-authorised: no handler taking a chat was "
              "found, so this check read nothing")
        return 1
    for failure in failures:
        print("FAIL check-handlers-act-on-what-they-authorised: %s" % failure)
    if failures:
        return 1
    print("check-handlers-act-on-what-they-authorised: passed; %d handlers authorise a chat "
          "and act on that chat" % checked)
    return 0


if __name__ == "__main__":
    sys.exit(main())
