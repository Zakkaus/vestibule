"""Read gate identities from executable shell commands and workflow run values."""
import re
import shlex


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


def _shell_commands(text: str):
    for fragment in _shell_fragments(text):
        tokens = _shell_tokens(fragment.lstrip(" ({"))
        while tokens and (
            re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", tokens[0])
            or tokens[0] in {"command", "do", "then"}
        ):
            tokens = tokens[1:]
        if tokens:
            yield tokens


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


def _go_mod_gate(arguments: list[str]) -> str | None:
    if not arguments:
        return None
    if arguments[0] == "tidy":
        return "go mod tidy" + (" -diff" if "-diff" in arguments[1:] else "")
    if arguments[0] == "verify":
        return "go mod verify"
    return None


def _go_gate(tokens: list[str]) -> str | None:
    if tokens[0] == "gofmt":
        return "gofmt"
    if tokens[0] != "go" or len(tokens) < 2:
        return None
    tool, arguments = tokens[1], tokens[2:]
    if tool == "mod":
        return _go_mod_gate(arguments)
    if tool == "vet":
        return "go vet"
    tag = _go_flag_value(arguments, "-tags")
    tag_suffix = " -tags " + tag if tag else ""
    if tool == "build":
        return "go build" + tag_suffix
    if tool == "test":
        race = " -race" if "-race" in arguments else ""
        shuffle_value = _go_flag_value(arguments, "-shuffle")
        shuffle = " -shuffle=" + shuffle_value if shuffle_value else ""
        return "go test" + race + shuffle + tag_suffix
    if tool == "run":
        return "go run " + (arguments[0] if arguments else "(unversioned)")
    return None


def _repository_script(tokens: list[str]) -> str | None:
    if tokens[0] in {"python", "python3", "bash", "sh"}:
        tokens = tokens[1:]
    if tokens and re.fullmatch(r"scripts/[A-Za-z0-9_./${}-]+\.(?:py|sh)", tokens[0]):
        return tokens[0]
    return None


def invocations(text: str) -> set[str]:
    """Match gates in command position, never in reporting arguments."""
    found = set()
    for tokens in _shell_commands(text):
        script = _repository_script(tokens)
        if script and "$" not in script:
            found.add(script)
        if len(tokens) >= 3 and tokens[:2] == ["npm", "run"]:
            if re.fullmatch(r"[a-z0-9:-]+", tokens[2]):
                found.add("npm run " + tokens[2])
        go_gate = _go_gate(tokens)
        if go_gate:
            found.add(go_gate)
    return found


def globbed_directories(text: str) -> set[str]:
    """Read directories covered by executed variable-driven script loops."""
    found = set()
    for tokens in _shell_commands(text):
        script = _repository_script(tokens)
        if script and "$" in script:
            found.add(script.rsplit("/", 1)[0])
    return found


def ci_actions(text: str, ignored: tuple[str, ...] = ("actions/",)) -> set[str]:
    return {
        match.group(1)
        for match in re.finditer(
            r"(?m)^\s*(?:-\s*)?uses:\s*([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@", text
        )
        if not match.group(1).startswith(ignored)
    }


def workflow_run_text(text: str) -> str:
    """Return only GitHub Actions run values, not names, comments, or metadata."""
    lines = text.splitlines()
    runs = []
    index = 0
    while index < len(lines):
        match = re.match(r"^(\s*(?:-\s+)?)run:\s*(.*)$", lines[index])
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


