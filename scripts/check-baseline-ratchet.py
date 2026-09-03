#!/usr/bin/env python3
"""A baselined violation may shrink or disappear. It may not grow.

scripts/lint.go already refuses a baselined metric whose value rose above what
scripts/baseline.txt records. That comparison is defeated by the obvious move:
the same commit that grows the function also writes the larger number into the
baseline. Nothing in the tree is inconsistent afterwards, so the gate is silent.

It happened in phase two. gradeKernelAnswer went from 104 lines to 105 and the
baseline was updated to match, in a change whose whole point was extracting code
out of that file. One line is nothing; the hole it went through is not.

The baseline cannot simply be read-only — a phase that moves a package has to
repath every row, and a phase that pays a debt has to prune it. So this compares
the committed file against its previous committed version and allows exactly
that: rows may vanish, values may fall. A value that rises is refused by name.

Usage: check-baseline-ratchet.py [base-ref]     (default: HEAD)
"""
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BASELINE = "scripts/baseline.txt"
# Boundary violations are findings too: adding one to the baseline is new debt.


def rows(text):
    out = {}
    for line in text.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        fields = line.split("\t")
        if len(fields) < 5:
            continue
        kind, path, _line, name, value = fields[0], fields[1], fields[2], fields[3], fields[4]
        try:
            out[(kind, path, name)] = int(value)
        except ValueError:
            continue
    return out


def main():
    base = sys.argv[1] if len(sys.argv) > 1 else "HEAD"
    # An unresolvable ref must fail. A check that quietly passes when it cannot
    # find what to compare against reports success for every change after it.
    resolved = subprocess.run(
        ["git", "rev-parse", "--verify", "--quiet", f"{base}^{{commit}}"],
        cwd=ROOT, capture_output=True, text=True,
    )
    if resolved.returncode != 0:
        print(f"FAIL check-baseline-ratchet: cannot resolve {base!r}")
        print("Under CI the base branch must be fetched before this runs;")
        print("without it there is nothing to compare and nothing is checked.")
        return 1
    shown = subprocess.run(
        ["git", "show", f"{base}:{BASELINE}"],
        cwd=ROOT, capture_output=True, text=True,
    )
    if shown.returncode != 0:
        print(f"check-baseline-ratchet: {base} has no {BASELINE}; it is new here")
        return 0
    before = rows(shown.stdout)
    after = rows((ROOT / BASELINE).read_text(encoding="utf-8"))

    risen = []
    for key, value in after.items():
        was = before.get(key)
        if was is not None and value > was:
            kind, path, name = key
            risen.append(f"  {kind} {path} {name}: {was} -> {value}")

    # A row that leaves one path and appears at another with the same shape is a
    # move — a package renamed, a file split — and phase 1C did exactly that for
    # every row it owned. A row with no counterpart is new debt.
    departed = {(kind, name, value) for (kind, path, name), value in before.items()
                if (kind, path, name) not in after}
    arrived = []
    for (kind, path, name), value in sorted(after.items()):
        if (kind, path, name) in before:
            continue
        if (kind, name, value) in departed:
            continue
        arrived.append(f"  {kind} {path} {name}: {value}")

    if risen or arrived:
        if risen:
            print("FAIL check-baseline-ratchet: a held violation may not grow")
            for line in sorted(risen):
                print(line)
        if arrived:
            print("FAIL check-baseline-ratchet: new code may not be added to the baseline")
            for line in arrived:
                print(line)
        print("\nThe baseline is phase zero's snapshot of debt that already existed.")
        print("A row may leave it, by the violation being cleared or the code moving")
        print("elsewhere unchanged. Nothing joins it: new code meets the limits —")
        print("600 lines a file, 80 a function, complexity 15 — or it is not new code")
        print("that is ready.")
        return 1

    print(f"check-baseline-ratchet: passed; {len(after)} rows, none grew since {base}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
