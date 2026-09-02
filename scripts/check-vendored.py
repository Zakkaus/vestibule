#!/usr/bin/env python3
"""Hold the vendored design-system copies to the bytes they were copied at.

CONTRIBUTING has always said these copies must stay byte-identical to the design
system they came from, and gave the command to check it. That command lives in
the design system, not here, so CI never ran it: the rule was written down and
enforced by nobody. check-gate-list did not notice because it only asked whether
every gate CI runs is documented, never whether every documented gate is run.

This cannot compare against the source, which is not present on a runner. It
compares against the hashes recorded when the copy was taken, which catches the
failure that happens here — someone editing a copy in place — and leaves
"upstream moved" to the re-sync, where the manifest records what to re-hash.

Usage: check-vendored.py [manifest]
"""
import hashlib
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def main() -> int:
    manifest_path = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "scripts/vendored-manifest.json")
    if not manifest_path.exists():
        print("FAIL check-vendored: %s is missing, so nothing was compared"
              % manifest_path.relative_to(ROOT))
        return 1
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    entries = manifest.get("files", [])
    if not entries:
        print("FAIL check-vendored: the manifest lists no file, so an empty run "
              "would report success")
        return 1

    failures = []
    directories = set()
    for entry in entries:
        path = ROOT / entry["file"]
        directories.add(path.parent)
        if not path.exists():
            failures.append("%s is in the manifest and not in the tree" % entry["file"])
            continue
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest != entry["sha256"]:
            failures.append("%s was edited in place; it must match %s upstream"
                            % (entry["file"], entry["source"]))

    # A copy added beside the others without a manifest row would otherwise be
    # unvendored and unchecked, which reads exactly like being checked.
    listed = {ROOT / entry["file"] for entry in entries}
    for directory in sorted(directories):
        for path in sorted(directory.iterdir()):
            if path.suffix in (".css", ".py") and path not in listed:
                failures.append("%s sits with the vendored copies and no manifest "
                                "row records where it came from"
                                % path.relative_to(ROOT))

    if failures:
        print("FAIL check-vendored: the vendored copies no longer match what was copied")
        for failure in failures:
            print("  " + failure)
        return 1

    print("check-vendored: passed; %d copies match the bytes recorded for them"
          % len(entries))
    return 0


if __name__ == "__main__":
    sys.exit(main())
