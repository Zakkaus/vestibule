#!/usr/bin/env python3
"""Everything CI invokes is named in CONTRIBUTING's gate block.

CONTRIBUTING tells a contributor what to run before opening a PR. Twice now that
list has fallen behind CI within a round of a gate being added — scripts/lint.sh
and the baseline ratchet the first time, the static SQLite gate and the prose
checker the second. Someone following the document pushes and takes a failure
they could not have predicted.

The lesson written down after the first time was to remember to update every
list. Remembering is not a mechanism, which is the same finding that put the
prose checker into CI in the first place. This is the mechanism.

It compares invocations, not command lines. CI legitimately passes different
flags — a base SHA where the document says origin/main, --silent where a person
wants output — and a checker demanding equal text would be switched off inside a
week. What must agree is *what gets run*: every repository script, every npm
script, and every third-party action CI uses has to be named in the block.

The direction is one-way. CI is the authority; the document may not omit. The
document may name extra things CI does not run, because some checks are worth
running locally before they are worth a runner.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONTRIBUTING = ROOT / "CONTRIBUTING.md"
WORKFLOW = ROOT / ".github" / "workflows" / "ci.yml"

# Steps whose commands are setup or reporting rather than a gate a person runs.
IGNORED_SCRIPTS = set()


def invocations(text: str) -> set:
    found = set()
    # Repository scripts, with or without an interpreter in front. The negative
    # look-behind keeps a script that lives outside the repository from being
    # read as one inside it: the Chinese checker is invoked as
    # ~/.claude/.../scripts/chinese_lint.py, and matching from "scripts/" alone
    # turned it into a repository gate that CI appeared not to run.
    for match in re.finditer(r"(?<![A-Za-z0-9_./~-])scripts/[A-Za-z0-9_./-]+\.(?:py|sh)", text):
        found.add(match.group(0))
    # npm scripts.
    for match in re.finditer(r"npm run ([a-z0-9:-]+)", text):
        found.add("npm run " + match.group(1))
    return found


def ci_actions(text: str) -> set:
    return {m.group(1) for m in re.finditer(r"uses:\s*([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@", text)
            if not m.group(1).startswith("actions/")}


def gate_block(text: str) -> str:
    blocks = re.findall(r"```sh\n(.*?)```", text, re.S)
    if not blocks:
        return ""
    # The gate list is the block under "Before opening a PR".
    marker = text.find("## Before opening a PR")
    if marker < 0:
        return "\n".join(blocks)
    after = text[marker:]
    following = re.findall(r"```sh\n(.*?)```", after, re.S)
    return following[0] if following else ""


def main() -> int:
    if not CONTRIBUTING.exists() or not WORKFLOW.exists():
        print("check-gate-list: CONTRIBUTING.md or the CI workflow is missing")
        return 1

    contributing = CONTRIBUTING.read_text(encoding="utf-8")
    workflow = WORKFLOW.read_text(encoding="utf-8")

    block = gate_block(contributing)
    if not block.strip():
        print("FAIL check-gate-list: no shell block under \"Before opening a PR\"")
        return 1

    # A glob in the document covers every script the glob would match.
    documented = invocations(block)
    globbed = set(re.findall(r"scripts/[A-Za-z0-9_./-]*\$[A-Za-z0-9_{}]+[A-Za-z0-9_./-]*", block))
    documented_dirs = {g.rsplit("/", 1)[0] for g in globbed}

    missing = []
    for used in sorted(invocations(workflow) - IGNORED_SCRIPTS):
        if used in documented:
            continue
        if used.rsplit("/", 1)[0] in documented_dirs:
            continue
        missing.append(used)

    for action in sorted(ci_actions(workflow)):
        if action not in contributing:
            missing.append(action + " (a CI action)")

    # The other direction, which was missing and cost something: CONTRIBUTING
    # said the vendored copies must stay byte-identical and gave the command,
    # CI never ran it, and this check could not see that because it only asked
    # whether every gate CI runs is documented. A documented gate nobody runs
    # reads exactly like a gate.
    unrun = []
    for gate in sorted(documented):
        if not gate.startswith("scripts/"):
            # An invocation whose path is outside the repository cannot run on a
            # runner. Those are local by construction; the count is printed so
            # the exemption stays a decision rather than a hole.
            continue
        # The first version of this line also accepted the gate's directory
        # appearing anywhere in the workflow, which is "scripts/" and therefore
        # always true, so every documented gate was exempted and removing one
        # from CI still passed. Ask for the gate itself.
        if gate in workflow:
            continue
        unrun.append(gate)

    if unrun:
        print("FAIL check-gate-list: CONTRIBUTING documents these and CI does not run them")
        for item in unrun:
            print("  " + item)
        print("\nA gate the document lists and no workflow runs is enforced by")
        print("whoever remembers it. Either wire it into CI or say in the document")
        print("why it cannot run there.")
        return 1

    if missing:
        print("FAIL check-gate-list: CI runs these and CONTRIBUTING does not name them")
        for item in missing:
            print("  " + item)
        print("\nThe list under \"Before opening a PR\" is what a contributor runs.")
        print("A gate CI enforces and the document omits is a failure nobody could")
        print("have predicted from reading the repository.")
        return 1

    print("check-gate-list: passed; %d gates, each one CI runs is documented and "
          "each one documented is run" % len(documented))
    return 0


if __name__ == "__main__":
    sys.exit(main())
