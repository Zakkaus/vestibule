#!/usr/bin/env python3
"""Check CSS hooks and the provenance of the built frontend stylesheet.

The positional form is the repository's compatibility wrapper around the
vendored coverage check::

    check-css-coverage.py <file.css|file.html> ...

The frontend form is provenance-aware and is the only form used for the Vite
bundle::

    check-css-coverage.py --frontend web --provenance web/dist/css-provenance.json

Vite emits one manifest entry per CSS asset.  The manifest is authoritative for
whether a rule came from project CSS or from a dependency.  No filename,
selector, or comment can turn project CSS into vendor CSS.  The emitted-byte
check below is intentionally only a lexical truncation guard; it is not a full
CSS grammar or selector parser.
"""

from __future__ import annotations

import argparse
import contextlib
import importlib.util
import io
import json
import re
import sys
from pathlib import Path
from types import ModuleType
from typing import Any

from spectrum_macros import is_style_expression, macro_definitions

ROOT = Path(__file__).resolve().parent.parent
VENDORED = ROOT / "scripts" / "design-checks" / "css-coverage.py"
POLICY = ROOT / "scripts" / "css-coverage-policy.json"


def load_vendored_checker() -> ModuleType:
    spec = importlib.util.spec_from_file_location("vendored_css_coverage", VENDORED)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {VENDORED}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def prefixed_class_uses_have_definitions(
    module: ModuleType, argv: list[str]
) -> tuple[list[str], int]:
    defined = set()
    used = {}
    for arg in argv:
        css, html = module.read(Path(arg))
        defined.update(module.DEF.findall(css))
        for match in module.USE.finditer(html):
            value = match.group("d") if match.group("d") is not None else match.group("s")
            for name in value.split():
                if name.startswith(module.IGNORE_PREFIXES):
                    used.setdefault(name, arg)
        for groups in module.SCRIPT_USE.findall(html):
            for group in groups:
                for name in module.SPLIT.split(group):
                    if module.IDENT.fullmatch(name) and name.startswith(module.IGNORE_PREFIXES):
                        used.setdefault(name, arg)
    failures = [
        f".{name} is used in {used[name]} but has no CSS definition; the named utility has no effect"
        for name in sorted(used)
        if name not in defined
    ]
    return failures, len(used)


def legacy_main(argv: list[str]) -> int:
    """Run the original wrapper without changing its public CLI."""
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    try:
        module = load_vendored_checker()
    except (OSError, RuntimeError) as error:
        print(f"FAIL check-css-coverage: {error}")
        return 2

    stdout = io.StringIO()
    stderr = io.StringIO()
    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
        status = module.main(argv)
    if status:
        print(
            "FAIL check-css-coverage: stylesheet hooks and their demonstrations disagree; "
            "a console element can render unstyled or dead CSS can survive"
        )
        print(stdout.getvalue(), end="")
        print(stderr.getvalue(), end="", file=sys.stderr)
        return status

    failures, prefixed = prefixed_class_uses_have_definitions(module, argv)
    if failures:
        print("FAIL check-css-coverage:")
        for failure in failures:
            print("  " + failure)
        return 1

    print(stdout.getvalue(), end="")
    print(f"check-css-coverage: passed; {prefixed} utility-prefixed class names have definitions")
    return 0


CSS_COMMENT = re.compile(r"/\*.*?\*/", re.S)
CSS_CLASS = re.compile(r"(?<![\w-])\.([A-Za-z][\w-]*)")
CSS_ATTR = re.compile(
    r"\[\s*(data-[\w-]+)"
    r"(?:\s*=\s*([\"'])([^\"']*)\2)?\s*\]"
)
SOURCE_CLASS_ASSIGNMENT = re.compile(r"\b(?:UNSAFE_)?class(?:Name)?\s*=")
JS_STRING = re.compile(r"""'(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*"|`(?:\\.|[^`\\])*`""", re.S)
SOURCE_DATA = re.compile(
    r"\b(data-[\w-]+)"
    r"(?:\s*=\s*(?:"
    r"['\"]([A-Za-z0-9_-]*)['\"]|"
    r"\{\s*['\"]([A-Za-z0-9_-]*)['\"]\s*\}|"
    r"\{([^}]*)\}))?"
)
SET_ATTRIBUTE = re.compile(
    r"setAttribute\(\s*['\"](data-[\w-]+)['\"]\s*,\s*['\"]([A-Za-z0-9_-]+)['\"]"
)
VAR_DEF = re.compile(r"(--[\w-]+)\s*:")
VAR_USE = re.compile(r"var\(\s*(--[\w-]+)\s*\)")


def _read_js_string(text: str, start: int, path: Path) -> tuple[str, int]:
    quote = text[start]
    escaped = False
    for index in range(start + 1, len(text)):
        char = text[index]
        if escaped:
            escaped = False
        elif char == "\\":
            escaped = True
        elif char == quote:
            return text[start + 1:index], index + 1
    raise ValueError(f"{path}: unterminated className string")


def _read_js_expression(text: str, start: int, path: Path) -> tuple[str, int]:
    depth = 0
    quote: str | None = None
    escaped = False
    for index in range(start, len(text)):
        char = text[index]
        if quote is not None:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == quote:
                quote = None
        elif char in {"'", '"', "`"}:
            quote = char
        elif char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return text[start + 1:index], index + 1
    raise ValueError(f"{path}: unterminated className expression")


def _class_expression_values(expression: str, path: Path) -> list[str]:
    literals: list[str] = []

    def record_literal(match: re.Match[str]) -> str:
        literal = match.group()
        if literal.startswith("`") and "${" in literal:
            raise ValueError(f"{path}: interpolated className is not statically verifiable")
        literals.append(literal[1:-1])
        return f"\x00{len(literals) - 1}\x00"

    tokens = JS_STRING.sub(record_literal, expression).strip()
    direct = re.fullmatch(r"\x00(\d+)\x00", tokens)
    if direct:
        return [literals[int(direct[1])]]
    conditional = re.fullmatch(r"[^?:]+\?\s*\x00(\d+)\x00\s*:\s*\x00(\d+)\x00", tokens)
    if conditional:
        return [literals[int(conditional[1])], literals[int(conditional[2])]]
    raise ValueError(f"{path}: className expression is not statically verifiable")


def _class_values(text: str, path: Path) -> list[str]:
    values: list[str] = []
    for assignment in SOURCE_CLASS_ASSIGNMENT.finditer(text):
        index = assignment.end()
        while index < len(text) and text[index].isspace():
            index += 1
        if index >= len(text):
            raise ValueError(f"{path}: className has no value")
        if text[index] in {"'", '"', "`"}:
            value, _ = _read_js_string(text, index, path)
            values.append(value)
        elif text[index] == "{":
            expression, _ = _read_js_expression(text, index, path)
            if not is_style_expression(text, expression):
                values.extend(_class_expression_values(expression, path))
        else:
            raise ValueError(f"{path}: className expression is not statically verifiable")
    return values


def _read_policy() -> tuple[set[str], set[str], dict[str, dict[str, str]]]:
    try:
        value = json.loads(POLICY.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot read {POLICY}: {error}") from error
    if not isinstance(value, dict) or value.get("version") != 1:
        raise ValueError(f"{POLICY}: unsupported version")
    attrs = value.get("non_styling_attributes")
    classes = value.get("non_styling_classes")
    runtime = value.get("runtime_custom_properties")
    if not isinstance(attrs, dict) or not isinstance(classes, dict) or not isinstance(runtime, dict):
        raise ValueError(f"{POLICY}: hook and runtime exemptions must be objects with reasons")
    if any(not isinstance(key, str) or not isinstance(reason, str) or not reason.strip()
           for key, reason in (*attrs.items(), *classes.items())):
        raise ValueError(f"{POLICY}: every non-styling hook needs a non-empty reason")
    runtime_sources: dict[str, dict[str, str]] = {}
    for name, entry in runtime.items():
        if not re.fullmatch(r"--[\w-]+", name) or not isinstance(entry, dict):
            raise ValueError(f"{POLICY}: runtime custom properties need structured entries")
        source = entry.get("source")
        setter = entry.get("setter")
        reason = entry.get("reason")
        if (
            not isinstance(source, str)
            or not source.startswith("node_modules/")
            or not source.endswith(".css")
            or "/" not in source
            or not isinstance(setter, str)
            or not setter.startswith("node_modules/")
            or not setter.endswith((".js", ".mjs", ".cjs"))
            or "/" not in setter
            or not isinstance(reason, str)
            or not reason.strip()
        ):
            raise ValueError(
                f"{POLICY}: runtime custom property {name} needs an exact "
                "dependency stylesheet, JS setter, and reason"
            )
        _safe_relative(source, f"{POLICY} runtime stylesheet")
        _safe_relative(setter, f"{POLICY} runtime setter")
        runtime_sources[name] = {"source": source, "setter": setter}
    return set(attrs), set(classes), runtime_sources


def _normalise_repo_path(path: str) -> str:
    value = path.replace("\\", "/")
    while value.startswith("./"):
        value = value[2:]
    if value.startswith("web/"):
        value = value[4:]
    return value


def _safe_relative(path: str, label: str) -> str:
    value = path.replace("\\", "/")
    candidate = Path(value)
    if candidate.is_absolute() or ".." in candidate.parts:
        raise ValueError(f"{label} must be a relative path inside the build: {path}")
    if not value or value.endswith("/"):
        raise ValueError(f"{label} is not a file path: {path}")
    return value


def _load_manifest(frontend: Path, dist: Path, manifest_path: Path) -> tuple[list[dict[str, Any]], list[str]]:
    try:
        raw = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as error:
        raise ValueError(f"cannot read CSS provenance manifest {manifest_path}: {error}") from error
    if not isinstance(raw, dict) or raw.get("version") != 1 or not isinstance(raw.get("assets"), list):
        raise ValueError("CSS provenance manifest must have version 1 and an assets list")
    if not raw["assets"]:
        raise ValueError("CSS provenance manifest contains no CSS assets")

    assets: list[dict[str, Any]] = []
    seen: set[str] = set()
    for index, entry in enumerate(raw["assets"]):
        if not isinstance(entry, dict):
            raise ValueError(f"CSS provenance asset {index} is not an object")
        relative = _safe_relative(str(entry.get("file", "")), f"asset {index}")
        if not relative.lower().endswith(".css"):
            raise ValueError(f"CSS provenance asset {relative} is not a CSS file")
        if relative in seen:
            raise ValueError(f"CSS provenance lists {relative} more than once")
        seen.add(relative)
        origins = entry.get("origins")
        if not isinstance(origins, list) or not origins:
            raise ValueError(f"CSS provenance asset {relative} has no origins")
        normalised_origins: list[dict[str, str]] = []
        for origin in origins:
            if not isinstance(origin, dict):
                raise ValueError(f"CSS provenance asset {relative} has a malformed origin")
            origin_path = _safe_relative(
                _normalise_repo_path(str(origin.get("path", ""))),
                f"asset {relative} origin",
            )
            kind = origin.get("kind")
            if not isinstance(kind, str) or kind not in {"project", "vendor", "spectrum-macro"}:
                raise ValueError(f"CSS provenance asset {relative} has an invalid origin kind")
            if kind == "project":
                if not origin_path.startswith("src/") or not origin_path.endswith(".css"):
                    raise ValueError(
                        f"CSS provenance marks non-project CSS as project: {origin_path}"
                    )
            elif kind == "spectrum-macro":
                if not origin_path.startswith("src/") or not origin_path.endswith((".ts", ".tsx")):
                    raise ValueError(f"CSS provenance marks a non-source path as a Spectrum macro: {origin_path}")
            elif not origin_path.startswith("node_modules/"):
                raise ValueError(
                    f"CSS provenance marks a non-dependency path as vendor: {origin_path}"
                )
            normalised_origins.append({"path": origin_path, "kind": kind})
        assets.append({"file": relative, "origins": normalised_origins})

    built_files = {
        path.relative_to(dist).as_posix()
        for path in dist.rglob("*.css")
        if path.is_file()
    } if dist.is_dir() else set()
    missing = sorted(built_files - seen)
    extra = sorted(seen - built_files)
    if missing:
        raise ValueError(
            "emitted CSS assets missing from provenance: " + ", ".join(missing)
        )
    if extra:
        raise ValueError(
            "provenance names CSS assets that were not emitted: " + ", ".join(extra)
        )
    if not built_files:
        raise ValueError(f"no emitted CSS assets found under {dist}")

    for asset in assets:
        path = dist / asset["file"]
        if not path.is_file():
            raise ValueError(f"provenance asset is not a readable file: {path}")
    return assets, sorted(built_files)


def _strip_css_comments(text: str) -> str:
    return CSS_COMMENT.sub("", text)
def _validate_css_asset(path: Path) -> tuple[str, set[str], set[str]]:
    try:
        raw = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise ValueError(f"cannot read emitted CSS {path}: {error}") from error
    css = _strip_css_comments(raw)
    if not css.strip():
        raise ValueError(f"emitted CSS asset is empty: {path}")
    if "/*" in css:
        raise ValueError(f"emitted CSS asset has an unterminated comment: {path}")

    stack: list[tuple[str, int]] = []
    quote: str | None = None
    escaped = False
    line = 1
    for char in css:
        if quote is not None:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == quote:
                quote = None
        elif char in {"'", '"'}:
            quote = char
        elif char in "{[(":
            stack.append((char, line))
        elif char in "}])":
            expected = {"}": "{", "]": "[", ")": "("}[char]
            if not stack or stack[-1][0] != expected:
                raise ValueError(f"emitted CSS asset has an unmatched {char}: {path}:{line}")
            stack.pop()
        if char == "\n":
            line += 1
    if quote is not None:
        raise ValueError(f"emitted CSS asset has an unterminated string: {path}")
    if stack:
        symbol, line = stack[-1]
        raise ValueError(f"emitted CSS asset has an unmatched {symbol}: {path}:{line}")
    if "{" not in css or ":" not in css:
        raise ValueError(f"emitted CSS asset contains no declaration block: {path}")
    return css, set(VAR_DEF.findall(css)), set(VAR_USE.findall(css))


def _source_files(source: Path) -> list[Path]:
    return sorted(
        path for path in source.rglob("*")
        if path.is_file()
        and path.suffix in {".tsx", ".ts", ".html"}
        and not path.name.endswith(".fixture.html")
    )


def _source_css_files(source: Path) -> list[Path]:
    return sorted(path for path in source.rglob("*.css") if path.is_file())


def _hooks_from_css(css_files: list[Path]) -> tuple[set[str], dict[str, set[str]]]:
    classes: set[str] = set()
    attrs: dict[str, set[str]] = {}
    for path in css_files:
        css = _strip_css_comments(path.read_text(encoding="utf-8"))
        classes.update(CSS_CLASS.findall(css))
        for match in CSS_ATTR.finditer(css):
            name, quote, value = match.groups()
            attrs.setdefault(name, set()).add(value if quote is not None else "")
    return classes, attrs


def _hooks_from_sources(files: list[Path]) -> tuple[set[str], dict[str, set[str]]]:
    classes: set[str] = set()
    attrs: dict[str, set[str]] = {}
    for path in files:
        text = path.read_text(encoding="utf-8")
        text = re.sub(r"/\*.*?\*/|//[^\n]*", " ", text, flags=re.S)
        for value in _class_values(text, path):
            classes.update(re.findall(r"[A-Za-z][\w-]*", value))
        for match in SOURCE_DATA.finditer(text):
            name = match.group(1)
            quoted_value = match.group(2) or match.group(3)
            expression = match.group(4)
            if expression is not None or (quoted_value is None and "=" in match.group(0)):
                # Runtime values demonstrate the attribute name, but not one
                # particular state selector.
                attrs.setdefault(name, set()).add("")
            elif quoted_value is None:
                attrs.setdefault(name, set()).add("")
            else:
                attrs.setdefault(name, set()).add(quoted_value)
        for name, value in SET_ATTRIBUTE.findall(text):
            attrs.setdefault(name, set()).add(value)
    return classes, attrs


def _coverage_failures(
    source: Path, project_css: list[Path], owned_css: list[Path]
) -> tuple[list[str], int, int]:
    ignored_attrs, ignored_classes, _ = _read_policy()
    defined_classes, defined_attrs = _hooks_from_css(project_css)
    owned_classes, owned_attrs = _hooks_from_css(owned_css)
    used_classes, used_attrs = _hooks_from_sources(_source_files(source))
    failures: list[str] = []

    for name in sorted(used_classes - defined_classes - ignored_classes):
        failures.append(f".{name} is used in TSX but has no project CSS definition")
    for name in sorted(owned_classes - used_classes - ignored_classes):
        failures.append(f".{name} is defined by project CSS but has no TSX use")

    for name, values in sorted(used_attrs.items()):
        if name in ignored_attrs:
            continue
        if name not in defined_attrs:
            failures.append(f"[{name}] is used in TSX but has no project CSS definition")
            continue
        defined_values = defined_attrs[name]
        known_values = values - {""}
        if known_values and "" not in defined_values and not (known_values & defined_values):
            failures.append(
                f"[{name}] values {', '.join(sorted(known_values))} have no project CSS definition"
            )
    for name, values in sorted(owned_attrs.items()):
        if name in ignored_attrs:
            continue
        if name not in used_attrs:
            failures.append(f"[{name}] is defined by project CSS but has no TSX use")
            continue
        if values and "" not in values and "" not in used_attrs[name]:
            unused_values = values - used_attrs[name]
            if unused_values:
                failures.append(
                    f"[{name}] values {', '.join(sorted(unused_values))} are defined by "
                    "project CSS but never used in TSX"
                )
    return failures, len(defined_classes) + len(defined_attrs), len(used_classes) + len(used_attrs)


def _nonlegacy_project_css(frontend: Path, project_sources: list[Path]) -> list[Path]:
    # These imported sheets are the previous console/design-system surface. Their
    # dormant examples remain available to legacy routes, but new authored sheets
    # must demonstrate every selector and data hook they add.
    legacy = {
        frontend / "src/app/app.css",
        frontend / "src/styles/tokens.css",
        frontend / "src/styles/components.css",
        frontend / "src/styles/shell.css",
    }
    return sorted(path for path in project_sources if path not in legacy)


def _project_custom_properties(project_sources: list[Path]) -> tuple[set[str], set[str]]:
    definitions: set[str] = set()
    references: set[str] = set()
    for path in project_sources:
        css = _strip_css_comments(path.read_text(encoding="utf-8"))
        definitions.update(VAR_DEF.findall(css))
        references.update(VAR_USE.findall(css))
    return definitions, references


def _runtime_source_is_verified(
    frontend: Path, asset: dict[str, Any], name: str, entry: dict[str, str]
) -> bool:
    stylesheet = entry["source"]
    if not any(
        origin["kind"] == "vendor" and origin["path"] == stylesheet
        for origin in asset["origins"]
    ):
        return False
    stylesheet_path = frontend / stylesheet
    setter_path = frontend / entry["setter"]
    if not stylesheet_path.is_file() or not setter_path.is_file():
        return False
    stylesheet_text = _strip_css_comments(stylesheet_path.read_text(encoding="utf-8"))
    setter_text = setter_path.read_text(encoding="utf-8")
    if name not in VAR_USE.findall(stylesheet_text):
        return False
    escaped_name = re.escape(name)
    return bool(
        re.search(rf"setProperty\(\s*['\"]{escaped_name}['\"]", setter_text)
        or re.search(rf"['\"]{escaped_name}['\"]\s*:", setter_text)
    )


def frontend_main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--frontend", required=True, type=Path)
    parser.add_argument("--source", type=Path)
    parser.add_argument("--dist", type=Path)
    parser.add_argument("--provenance", required=True, type=Path)
    parser.add_argument("--print-project-assets", action="store_true")
    parser.add_argument("--print-project-sources", action="store_true")
    parser.add_argument("--print-emitted-assets", action="store_true")
    args = parser.parse_args(argv)
    if sum(
        bool(value)
        for value in (
            args.print_project_assets,
            args.print_project_sources,
            args.print_emitted_assets,
        )
    ) > 1:
        parser.error("choose one --print-* output mode")
    frontend = args.frontend.resolve()
    source = (args.source or frontend / "src").resolve()
    dist = (args.dist or frontend / "dist").resolve()
    manifest = args.provenance.resolve()
    try:
        assets, emitted_asset_paths = _load_manifest(frontend, dist, manifest)
        emitted_css: dict[str, tuple[str, set[str], set[str]]] = {}
        for asset in assets:
            emitted_css[asset["file"]] = _validate_css_asset(dist / asset["file"])

        project_assets = [
            asset["file"] for asset in assets
            if any(origin["kind"] == "project" for origin in asset["origins"])
        ]
        project_sources = sorted({
            frontend / origin["path"]
            for asset in assets
            for origin in asset["origins"]
            if origin["kind"] == "project"
        })
        if not project_sources:
            raise ValueError("CSS provenance contains no project-owned source CSS")
        missing_sources = [path for path in project_sources if not path.is_file()]
        if missing_sources:
            raise ValueError(
                "project CSS provenance points at missing sources: "
                + ", ".join(str(path) for path in missing_sources)
            )
        authored_css = _source_css_files(source)
        if not authored_css:
            raise ValueError(f"no authored CSS files found under {source}")
        unowned_css = sorted(set(authored_css) - set(project_sources))
        missing_authored = sorted(set(project_sources) - set(authored_css))
        if unowned_css:
            raise ValueError(
                "authored CSS files missing from provenance: "
                + ", ".join(str(path) for path in unowned_css)
            )
        if missing_authored:
            raise ValueError(
                "project CSS provenance sources are outside the authored source tree: "
                + ", ".join(str(path) for path in missing_authored)
            )

        _, _, runtime_sources = _read_policy()
        project_definitions, project_references = _project_custom_properties(project_sources)
        definitions = set().union(*(item[1] for item in emitted_css.values()))
        project_definitions.update(macro_definitions(frontend, _source_files(source), assets, definitions))
        unresolved: list[str] = [
            f"{name} (project source has no project definition)"
            for name in sorted(project_references - project_definitions)
        ]
        for asset in assets:
            references = emitted_css[asset["file"]][2]
            for name in sorted(references - definitions):
                entry = runtime_sources.get(name)
                if name in project_references:
                    unresolved.append(f"{name} (project reference has no emitted definition)")
                    continue
                if entry is None or not _runtime_source_is_verified(frontend, asset, name, entry):
                    unresolved.append(
                        f"{name} (runtime ownership is not verified in {asset['file']})"
                    )
        if unresolved:
            raise ValueError(
                "emitted CSS has unresolved custom properties: " + ", ".join(unresolved)
            )

        owned_css = _nonlegacy_project_css(frontend, project_sources)
        if not owned_css:
            raise ValueError("CSS provenance contains no nonlegacy project-owned CSS")
        failures, defined_count, used_count = _coverage_failures(
            source, project_sources, owned_css
        )
        if failures:
            print("FAIL check-css-coverage:")
            for failure in failures:
                print("  " + failure)
            print(
                f"  evidence: {defined_count} project CSS hooks defined; "
                f"{used_count} TSX hooks observed"
            )
            return 1
    except (OSError, UnicodeDecodeError, ValueError) as error:
        print(f"FAIL check-css-coverage: {error}")
        return 1

    if args.print_emitted_assets:
        print("\n".join(str(dist / path) for path in emitted_asset_paths))
        return 0

    if args.print_project_assets:
        print("\n".join(project_assets))
        return 0
    if args.print_project_sources:
        print("\n".join(str(path) for path in project_sources))
        return 0
    print(
        "check-css-coverage: passed; "
        f"{len(emitted_css)} emitted CSS asset(s), "
        f"{len(project_assets)} project asset(s), "
        f"{defined_count} project hooks defined and {used_count} source hooks observed"
    )
    return 0


def main(argv: list[str]) -> int:
    if "--frontend" in argv or "--provenance" in argv:
        return frontend_main(argv)
    return legacy_main(argv)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
