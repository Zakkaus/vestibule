#!/usr/bin/env python3
"""Nothing the previous generation answered disappears before the cutover.

Each phase's plan lists behaviour that must survive the rewrite, and nothing
checks that those lists are complete. The cheapest complete list that exists is
the previous generation's command set: a name that stops being registered is a
user typing something that used to work and getting nothing.

The reference lives outside this repository, so the names are frozen in
scripts/inherited-commands.txt with the date they were read. A name leaves that
file only into its dropped section, with a reason, which makes removing a
command a decision someone wrote down rather than a diff nobody noticed.

Usage: check-inherited-commands.py [modules.go] [manifest]
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DECLARED = re.compile(r'Name: *"([a-z]+)", *Description:')


def main() -> int:
    source = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "internal/app/modules.go")
    manifest = ROOT / (sys.argv[2] if len(sys.argv) > 2 else "scripts/inherited-commands.txt")
    if not source.is_file():
        print("FAIL check-inherited-commands: %s is missing, so nothing was compared"
              % source.relative_to(ROOT) if source.is_relative_to(ROOT) else source)
        return 1
    if not manifest.is_file():
        print("FAIL check-inherited-commands: %s is missing, so nothing was compared"
              % manifest)
        return 1

    inherited, dropped = [], {}
    for line in manifest.read_text(encoding="utf-8").split("\n"):
        # Strip only the newline: stripping the whole line first turned a dropped
        # entry whose reason is empty into an ordinary inherited name, so the
        # missing reason went unreported. The tab is the field separator and has
        # to survive long enough to be seen.
        text = line.rstrip("\n").rstrip("\r")
        if not text.strip():
            continue
        if text.lstrip().startswith("#"):
            continue
        if "\t" in text:
            name, reason = text.split("\t", 1)
            dropped[name.strip()] = reason.strip()
            continue
        inherited.append(text.strip())

    if not inherited and not dropped:
        print("FAIL check-inherited-commands: the manifest names no command; an "
              "empty run would report success")
        return 1

    declared = set(DECLARED.findall(source.read_text(encoding="utf-8")))
    if not declared:
        print("FAIL check-inherited-commands: no command declaration was found in "
              "%s — has the declaration shape changed?" % source.name)
        return 1

    failures = []
    for name in inherited:
        if name in declared:
            continue
        if name in dropped:
            continue
        failures.append("/%s was answered by the previous generation and is no "
                        "longer registered; drop it in the manifest with a reason "
                        "or put it back" % name)
    for name, reason in sorted(dropped.items()):
        if not reason:
            failures.append("/%s is listed as dropped without a reason" % name)

    if failures:
        print("FAIL check-inherited-commands: the bot stopped answering something "
              "it used to")
        for failure in failures:
            print("  " + failure)
        return 1

    print("check-inherited-commands: passed; %d inherited commands still "
          "registered, %d deliberately dropped" % (len(inherited), len(dropped)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
