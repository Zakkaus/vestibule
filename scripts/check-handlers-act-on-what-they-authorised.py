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

The comparison is by identifier name, because the identifier is how the handler refers to the
chat it was allowed to touch. Passing a differently-named variable is exactly the shape being
refused, so a rename that means to keep the property renames both.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
HANDLERS = ROOT / "internal" / "console" / "api"
FUNCTION = re.compile(r"^func \(s \*Server\) ([A-Za-z0-9]+)\(.*?\n\}", re.S | re.M)
AUTHORISES = re.compile(r"authorizedSession\(writer, request, ([A-Za-z][A-Za-z0-9]*),")
PASSES = re.compile(r"(?<![A-Za-z])(?:GroupID|ChatID):\s*([A-Za-z][A-Za-z0-9]*)")
# The lookbehind keeps AdminLogChatID and its kind out: they name a destination for a
# notice, not the chat being acted on, and a handler holding one is not a violation.


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
        for match in FUNCTION.finditer(text):
            body = match.group(0)
            authorised = AUTHORISES.search(body)
            if not authorised:
                continue
            checked += 1
            for passed in sorted(set(PASSES.findall(body))):
                if passed != authorised.group(1):
                    failures.append(
                        "%s: %s authorises %s and then names %s as the chat to act on; a "
                        "handler must act on the chat it was checked against"
                        % (path.relative_to(ROOT), match.group(1), authorised.group(1), passed))

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
