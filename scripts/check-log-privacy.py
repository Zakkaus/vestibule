#!/usr/bin/env python3
"""What a person typed does not reach the log.

docs/PRIVACY.md and its Chinese twin promise that the logs carry user IDs and
usernames and deliberately not message bodies or challenge answers. Under the
kernel question that promise matters more than it sounds: the answer is the
output of uname -r, which describes somebody's machine.

The promise holds today and nothing was holding it. This does, for the two
packages a reply passes through — verification, where the answer arrives, and
rules, where it is judged. Everywhere else composes its own text.

This is a name heuristic and says so: it reports a log call that interpolates a
variable named for user content. That is a smoke detector, not a proof — it
cannot see a reply carried in a differently named variable. It is worth having
because the shape it catches is the one that actually happens: someone adds
log.Printf("answer %q", text) while debugging and leaves it.

Usage: check-log-privacy.py [package ...]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT = ("internal/verification", "internal/rules")
LOG_CALL = re.compile(r"\blog\.(?:Printf|Println|Print|Fatalf)\(")
# Names a reply is carried in along those two packages' call paths.
USER_CONTENT = re.compile(r"\b(text|answer|reply|body|content|caption|input)\b")
# The format string is prose about the fields, not a field. Removing it first is
# what separates "logs the reply" from "says the word reply".
FORMAT_STRING = re.compile(r'"(?:[^"\\]|\\.)*"')
# An exact source line may be listed here with the reason it is not user content.
ALLOWED: dict[str, str] = {}


def main() -> int:
    packages = [ROOT / name for name in (sys.argv[1:] or DEFAULT)]
    scanned = 0
    calls = 0
    failures = []
    seen: set[str] = set()
    for package in packages:
        if not package.is_dir():
            print("FAIL check-log-privacy: %s is not a directory, so nothing was read"
                  % package)
            return 1
        for path in sorted(package.glob("*.go")):
            if path.name.endswith("_test.go"):
                continue
            scanned += 1
            for number, line in enumerate(path.read_text(encoding="utf-8").split("\n"), 1):
                if not LOG_CALL.search(line):
                    continue
                calls += 1
                stripped = line.strip()
                if stripped in ALLOWED:
                    seen.add(stripped)
                    continue
                # Only the arguments after the format string matter. The first
                # version searched from the opening parenthesis and reported five
                # lines whose format prose happens to contain "reply" or
                # "answer" — the words were in the sentence, not in a variable.
                arguments = FORMAT_STRING.sub("", line, count=1)
                found = USER_CONTENT.search(arguments)
                if found:
                    failures.append("%s:%d logs a value named %s; PRIVACY says a "
                                    "message body and a challenge answer stay out "
                                    "of the log"
                                    % (path.relative_to(ROOT), number, found.group(1)))

    if scanned == 0:
        print("FAIL check-log-privacy: no Go source was read from %s"
              % ", ".join(str(p) for p in packages))
        return 1
    if calls == 0:
        print("FAIL check-log-privacy: no log call was found at all — has the "
              "logging shape changed?")
        return 1

    if failures:
        print("FAIL check-log-privacy: something a person typed may reach the log")
        for failure in failures:
            print("  " + failure)
        print("\nLog the decision and the identifiers, not the text. If the value is "
              "not user content, add its exact line to ALLOWED with the reason.")
        return 1

    # ALLOWED is empty today. The assertion goes in while that is true, because an
    # exception nobody re-reads is how a rule stops applying: every entry must still be
    # a line this package contains.
    for line, reason in sorted(ALLOWED.items()):
        if line not in seen:
            failures.append("the allowed line %r is not here any more (%s)" % (line, reason))
    if failures:
        print("FAIL check-log-privacy: an exception outlived the line it was written for")
        for failure in failures:
            print("  " + failure)
        return 1

    print("check-log-privacy: passed; %d log calls across %d files carry no value "
          "named for user content, %d listed exceptions"
          % (calls, scanned, len(ALLOWED)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
