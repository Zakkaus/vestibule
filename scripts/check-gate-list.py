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
in both CI and the documented gate block. Go checks retain their race detector,
build tags, and tool versions. Third-party CI actions must also be documented.
"""
import re
import shlex
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONTRIBUTING = ROOT / "CONTRIBUTING.md"
WORKFLOW = ROOT / ".github" / "workflows" / "ci.yml"

# Steps whose commands are setup or reporting rather than a gate a person runs.
IGNORED_SCRIPTS = set()


def _shell_tokens(fragment: str) -> list[str]:
    """Lex one already-separated shell fragment."""
    return shlex.split(fragment, comments=False, posix=True)


def _raw_shell_fragments(line: str):
    """Split only on unquoted, unescaped shell separators and comments."""
    quote = ""
    escaped = False
    fragment = []
    for character in line:
        if escaped:
            fragment.append(character)
            escaped = False
            continue
        if character == "\\":
            fragment.append(character)
            escaped = True
            continue
        if quote:
            fragment.append(character)
            if character == quote:
                quote = ""
            continue
        if character in "'\"":
            fragment.append(character)
            quote = character
        elif character in ";&|":
            value = "".join(fragment).strip()
            if value:
                yield value
            fragment = []
        elif character == "#":
            value = "".join(fragment).strip()
            if value:
                yield value
            return
        else:
            fragment.append(character)
    value = "".join(fragment).strip()
    if value:
        yield value


def _logical_shell_lines(text: str):
    """Join shell backslash-newline continuations without parsing shell."""
    pending = ""
    for line in text.splitlines():
        pending += line
        if line.endswith("\\"):
            pending = pending[:-1]
            continue
        yield pending
        pending = ""
    if pending:
        yield pending


def _command_substitutions(fragment: str):
    """Yield the supported real assignment command substitution, if present."""
    for pattern in (
        r'\s*[A-Za-z_][A-Za-z0-9_]*="\$\(([^()]*)\)"\s*',
        r"\s*[A-Za-z_][A-Za-z0-9_]*=\$\(([^()]*)\)\s*",
    ):
        match = re.fullmatch(pattern, fragment)
        if match:
            yield match.group(1)
            return


def _shell_fragments(text: str):
    """Yield command-sized fragments using shell lexical boundaries."""
    for line in _logical_shell_lines(text):
        for fragment in _raw_shell_fragments(line):
            yield fragment
            for substitution in _command_substitutions(fragment):
                yield from _shell_fragments(substitution)


def _go_command(fragment: str):
    tokens = _shell_tokens(fragment)
    command_index = 0
    while command_index < len(tokens):
        token = tokens[command_index]
        if re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", token):
            command_index += 1
            continue
        break
    if command_index < len(tokens) and tokens[command_index] == "command":
        command_index += 1
    if command_index >= len(tokens):
        return None
    if tokens[command_index] == "gofmt":
        return "gofmt", tokens[command_index + 1:]
    if command_index + 1 >= len(tokens) or tokens[command_index] != "go":
        return None
    return tokens[command_index + 1], tokens[command_index + 2:]


def _go_flag_value(arguments: list[str], flag: str) -> str | None:
    for index, argument in enumerate(arguments):
        if argument == flag:
            if index + 1 < len(arguments):
                return arguments[index + 1]
            return None
        prefix = flag + "="
        if argument.startswith(prefix):
            return argument[len(prefix):]
    return None


def go_invocations(text: str) -> set:
    found = set()
    for fragment in _shell_fragments(text):
        command = _go_command(fragment)
        if command is None:
            continue
        tool, arguments = command
        if tool == "gofmt":
            found.add("gofmt")
            continue
        if tool not in {"vet", "build", "test", "run"}:
            continue
        if tool == "vet":
            found.add("go vet")
            continue
        tag = _go_flag_value(arguments, "-tags")
        tag_suffix = " -tags " + tag if tag else ""
        if tool == "build":
            found.add("go build" + tag_suffix)
            continue
        if tool == "test":
            race = " -race" if any(argument == "-race" for argument in arguments) else ""
            found.add("go test" + race + tag_suffix)
            continue
        module = arguments[0] if arguments else ""
        found.add("go run " + (module or "(unversioned)"))
    return found


def _workflow_run_text(text: str) -> str:
    """Return only GitHub Actions run values, not names, comments, or metadata."""
    lines = text.splitlines()
    runs = []
    index = 0
    while index < len(lines):
        match = re.match(r"^(\s*)run:\s*(.*)$", lines[index])
        if not match:
            index += 1
            continue
        indent, value = match.groups()
        if value.startswith(("|", ">")):
            parent_indent = len(indent)
            index += 1
            while index < len(lines):
                body = lines[index]
                body_indent = len(body) - len(body.lstrip())
                if body.strip() and body_indent <= parent_indent:
                    break
                runs.append(body)
                index += 1
            continue
        runs.append(value)
        index += 1
    return "\n".join(runs)


def _workflow_commands(text: str) -> str:
    """Return run commands without metadata, comments, or reporting lines."""
    commands = []
    for fragment in _shell_fragments(_workflow_run_text(text)):
        if re.match(r"^(?:echo|printf|:)(?:\s|$)", fragment):
            continue
        commands.append(fragment)
    return "\n".join(commands)


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
    found.update(go_invocations(text))
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

    commands = _workflow_commands(workflow)
    workflow_gates = invocations(commands)
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
