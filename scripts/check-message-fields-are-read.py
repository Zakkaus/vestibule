#!/usr/bin/env python3
"""Every message field the catalogue declares is read by something outside i18n.

A declared field costs three translations and reads as a promise that the software
says this somewhere. Five were left behind by the rewrite -- the previous generation's
inline settings panel had an edit label, a stale-index notice, a reply-to-the-prompt
notice and a control-group refusal, and the feed had its own control-group refusal.
None of them is reachable: no call site names them, and the panel they belonged to is
not the panel this generation ships. Fifteen translated strings existed for text a
reader can never see.

The catalogue's loader already refuses a JSON key with no field, which is the other
direction. This is the one it cannot see, because a field with no reader still loads
perfectly.

A field's name is how a call site names it, so this compares names literally. Finding
no field at all is a failure: a rename of the declaration shape is how it would rot.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CATALOGUE = ROOT / "internal" / "i18n"
FIELD = re.compile(r"^\t([A-Z][A-Za-z0-9]*)\s+(?:Text|Format|StringList)\b", re.M)


def main() -> int:
    if not CATALOGUE.is_dir():
        print("FAIL check-message-fields-are-read: %s does not exist, so this check read "
              "nothing" % CATALOGUE)
        return 1

    declared = {}
    for path in sorted(CATALOGUE.glob("*.go")):
        if path.name.endswith("_test.go") or path.name == "catalog.go":
            continue
        text = path.read_text()
        for match in FIELD.finditer(text):
            line = text[: match.start()].count("\n") + 1
            declared.setdefault(match.group(1), []).append(
                "%s:%d" % (path.relative_to(ROOT), line))

    if not declared:
        print("FAIL check-message-fields-are-read: no message field was found in %s, so "
              "this check read nothing" % CATALOGUE.relative_to(ROOT))
        return 1

    readers = ""
    for directory in ("internal", "cmd"):
        for path in (ROOT / directory).rglob("*.go"):
            if path.is_relative_to(CATALOGUE):
                continue
            readers += path.read_text()

    failures = []
    for name, where in sorted(declared.items()):
        if not re.search(r"\b%s\b" % re.escape(name), readers):
            failures.append("%s declares %s and nothing outside internal/i18n names it; "
                            "its translations describe text nobody can read"
                            % (", ".join(where), name))
    for failure in failures:
        print("FAIL check-message-fields-are-read: %s" % failure)
    if failures:
        return 1
    print("check-message-fields-are-read: passed; %d message fields, every one of them "
          "named by a reader outside the catalogue" % len(declared))
    return 0


if __name__ == "__main__":
    sys.exit(main())
