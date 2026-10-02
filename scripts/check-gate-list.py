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
week. Every repository script, npm script, Go check, and analysis tool must appear
in both CI and the documented gate block. Go checks retain their matrix variants,
race detector, shuffle mode, build tags, and tool versions. Third-party CI actions
must also be documented.
"""
import re
import sys
from pathlib import Path

from gate_invocations import ci_actions, globbed_directories, invocations, workflow_run_text

ROOT = Path(__file__).resolve().parent.parent
CONTRIBUTING = ROOT / "CONTRIBUTING.md"
WORKFLOW = ROOT / ".github" / "workflows" / "ci.yml"

# Steps whose commands are setup or reporting rather than a gate a person runs.
IGNORED_SCRIPTS = set()


def go_matrix_tags(text: str) -> set[str]:
    job = re.search(
        r"(?ms)^  go:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)",
        text,
    )
    if job is None:
        return set()
    entries = re.findall(
        r'(?m)^          - tags:\s*(?:"([^"]*)"|\'([^\']*)\'|([A-Za-z0-9_-]+))\s*$',
        job.group("body"),
    )
    return {double or single or bare for double, single, bare in entries}


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

    matrix_tags = go_matrix_tags(workflow)
    missing_matrix_entries = []
    if "" not in matrix_tags:
        missing_matrix_entries.append("default Go matrix entry")
    if "gentoo" not in matrix_tags:
        missing_matrix_entries.append("gentoo Go matrix entry")
    if missing_matrix_entries:
        print("FAIL check-gate-list: the Go matrix lost required compatibility entries")
        for item in missing_matrix_entries:
            print("  " + item)
        return 1

    block = gate_block(contributing)
    if not block.strip():
        print("FAIL check-gate-list: no shell block under \"Before opening a PR\"")
        return 1

    # A glob in the document covers every script the glob would match.
    documented = invocations(block)
    documented_dirs = globbed_directories(block)

    workflow_gates = invocations(workflow_run_text(workflow))
    missing = []
    for used in sorted(workflow_gates - IGNORED_SCRIPTS):
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
        # The first version of this line accepted any occurrence in the
        # workflow, including a step name or comment. Compare parsed run
        # invocations instead, so a removed command cannot hide behind prose.
        if gate in workflow_gates:
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
