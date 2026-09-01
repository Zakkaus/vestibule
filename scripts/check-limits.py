#!/usr/bin/env python3
"""The limits the documents state are the limits the linter enforces.

Three numbers — 600 lines a file, 80 a function, complexity 15 — are defined in
scripts/lint.go and repeated in five places across three documents. That is the
shape where one moves and the rest quietly do not, and a contributor reads
whichever they opened.

They all agree today, which is the cheapest moment there will be to freeze it.

Only anchored phrasings are read, never bare numbers: "12 files, 4,600 lines" in
a plan is not a claim about the file limit, and a checker that says it is gets
switched off.

Usage: check-limits.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LINT = ROOT / "scripts" / "lint.go"
DOCUMENTS = ["CONTRIBUTING.md", "docs/PLAN-v5.md", "docs/ARCHITECTURE.md"]

# (constant in lint.go, human name, patterns whose capture group is the number)
LIMITS = [
    ("maxFileLines", "a file's lines", [
        r"单个文件 \| (\d+) 行",
        r"文件 (\d+) 行[、，]",
        r"\| 文件行数 \| 超过 (\d+) 行即失败",
        r"One file \| (\d+) lines",
        r"(\d+) lines a file",
    ]),
    ("maxFunctionLines", "a function's lines", [
        r"函数 (\d+) 行[、，]",
        r"单个函数 \| (\d+) 行",
        r"\| 函数行数 \| 超过 (\d+) 行即失败",
        r"One function \| (\d+) lines",
        r"(\d+) a function[,.]",
    ]),
    ("maxComplexity", "cyclomatic complexity", [
        r"圈复杂度 (\d+)",
        r"\| 圈复杂度 \| 超过 (\d+) 即失败",
        r"复杂度 (\d+)[。，]",
        r"complexity (\d+)[,.]",
    ]),
]


def main() -> int:
    if not LINT.exists():
        print("FAIL check-limits: scripts/lint.go is missing")
        return 1
    source = LINT.read_text(encoding="utf-8")

    enforced = {}
    for constant, _, _ in LIMITS:
        match = re.search(r"%s\s*=\s*(\d+)" % constant, source)
        if not match:
            print("FAIL check-limits: scripts/lint.go no longer defines %s" % constant)
            return 1
        enforced[constant] = match.group(1)

    failures = []
    stated = 0
    counted = {constant: 0 for constant, _, _ in LIMITS}
    per_document = {name: 0 for name in DOCUMENTS}
    for name in DOCUMENTS:
        path = ROOT / name
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        for constant, human, patterns in LIMITS:
            for pattern in patterns:
                for match in re.finditer(pattern, text):
                    stated += 1
                    counted[constant] += 1
                    per_document[name] += 1
                    if match.group(1) != enforced[constant]:
                        line = text[:match.start()].count("\n") + 1
                        failures.append(
                            "  %s:%d says %s is %s; scripts/lint.go enforces %s"
                            % (name, line, human, match.group(1), enforced[constant]))

    if failures:
        print("FAIL check-limits: a document states a limit the linter does not enforce")
        for line in sorted(set(failures)):
            print(line)
        print("\nA contributor reads whichever document they opened. Change the")
        print("number in scripts/lint.go and every place that repeats it, or not at all.")
        return 1

    # Failing only at zero lets phrasing drift quietly shrink what is covered.
    # Every limit must be stated somewhere, and every document must state one.
    uncovered = [human for constant, human, _ in LIMITS if not counted[constant]]
    if uncovered:
        print("FAIL check-limits: no document states %s any more"
              % ", or ".join(uncovered))
        print("Either the limit stopped being documented, or the phrasing moved and")
        print("this check silently stopped reading it. Both need a person.")
        return 1
    silent = [name for name in DOCUMENTS if (ROOT / name).exists() and not per_document[name]]
    if silent:
        print("FAIL check-limits: %s states no limit at all any more"
              % ", ".join(silent))
        return 1

    print("check-limits: passed; %d statements across %d documents, all matching "
          "scripts/lint.go" % (stated, len(DOCUMENTS)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
