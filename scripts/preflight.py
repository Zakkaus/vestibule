#!/usr/bin/env python3
"""Check a dispatch specification against the working copy it will run in.

Four rounds were spent on specifications that were wrong rather than hard: a
section cited in the wrong document, a branch name carried over from the slice
it was copied from, a path that had been renamed, and a line number that had
moved a phase earlier. The agent stopped each time and was right each time, and
each stop cost a whole round.

The workflow notes have told me to run this for a while. It did not exist here,
which is its own lesson: a tool named in the instructions and absent from the
repository is a step nobody performs.

Usage: preflight.py <spec.md> <worktree>
"""
import re
import subprocess
import sys
from pathlib import Path

SUFFIXES = "go|ts|tsx|py|sh|md|html|json|ya?ml"
CITATION = re.compile(r"`([A-Za-z0-9_./-]+\.(?:" + SUFFIXES + r"))(?::(\d+)(?:-(\d+))?)?`")
BRANCH = re.compile(r"分支\s*`([^`]+)`")
SECTION = re.compile(r"「([^」]{2,40})」\s*(?:那一节|这一节|整节|一节)")
# A citation on a line that names the previous generation points at ~/code/refs,
# not at this repository, and is checked by reading it rather than by this script.
EXTERNAL = re.compile(r"上一代|refs/")

failures: list[str] = []


def check_branch(spec: str, worktree: Path) -> None:
    named = BRANCH.search(spec)
    if not named:
        failures.append("the specification does not name a branch")
        return
    actual = subprocess.run(["git", "-C", str(worktree), "rev-parse", "--abbrev-ref", "HEAD"],
                            capture_output=True, text=True).stdout.strip()
    if actual != named.group(1):
        failures.append("the specification says branch %s, the working copy is on %s"
                        % (named.group(1), actual))


def check_citations(spec: str, worktree: Path) -> list[Path]:
    cited: list[Path] = []
    for line in spec.split("\n"):
        if EXTERNAL.search(line):
            continue
        for path_text, start, end in CITATION.findall(line):
            if "/" not in path_text:
                # A bare basename is a mention, not a citation — except with a
                # line number, which claims a specific place and has to say
                # where. Two prompts this round wrote `server.go:274` after
                # naming the full path once, which reads fine and cannot be
                # checked.
                if start:
                    failures.append("%s:%s cites a line without a path — write the "
                                    "full path so it can be checked" % (path_text, start))
                continue
            path = worktree / path_text
            if not path.exists():
                failures.append("%s does not exist in the working copy" % path_text)
                continue
            cited.append(path)
            if not start:
                continue
            lines = path.read_text(encoding="utf-8", errors="replace").split("\n")
            for number in {int(start), int(end or start)}:
                if number > len(lines):
                    failures.append("%s:%d is past the end of the file (%d lines)"
                                    % (path_text, number, len(lines)))
                elif not lines[number - 1].strip(" \t{}()[];,"):
                    failures.append("%s:%d is blank or only punctuation — the line moved"
                                    % (path_text, number))
    if not cited:
        failures.append("the specification cites no file in this repository at all")
    return cited


def check_sections(spec: str, cited: list[Path]) -> None:
    texts = {path: path.read_text(encoding="utf-8", errors="replace") for path in set(cited)}
    for heading in SECTION.findall(spec):
        if not any(heading in text for text in texts.values()):
            failures.append("no cited file contains the section 「%s」" % heading)


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    spec_path, worktree = Path(sys.argv[1]), Path(sys.argv[2])
    if not spec_path.is_file():
        print("FAIL preflight: %s is not a file" % spec_path, file=sys.stderr)
        return 1
    if not (worktree / ".git").exists():
        print("FAIL preflight: %s is not a working copy" % worktree, file=sys.stderr)
        return 1
    spec = spec_path.read_text(encoding="utf-8")
    check_branch(spec, worktree)
    cited = check_citations(spec, worktree)
    check_sections(spec, cited)
    if failures:
        for failure in failures:
            print("FAIL preflight: " + failure, file=sys.stderr)
        return 1
    print("preflight: passed; %d citations resolve, branch matches" % len(cited))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
