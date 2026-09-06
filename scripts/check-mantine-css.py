#!/usr/bin/env python3
"""Verify Mantine's stylesheet boundary and check the authored CSS it surrounds.

Mantine owns the rules inside ``@layer mantine``.  The package is not copied into
this repository: the lockfile, package version, and published stylesheet hash are
its provenance.  Everything outside that layer must be the compiled form of the
committed application stylesheets, so a generated asset cannot quietly lose or gain
project CSS.  The existing compiled-CSS design checks run against that non-library
remainder rather than against Mantine's own tokens.

Usage:
  check-mantine-css.py --mantine-css web/node_modules/@mantine/core/styles.layer.css \
    --bundle web/dist/assets/*.css --source-css web/src/styles/tokens.css \
    --source-css web/src/app/app.css --hook data-entry-page \
    --hook data-record-table
"""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
import tempfile
from collections.abc import Callable, Iterable, Sequence
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LOCKFILE = ROOT / "web" / "package-lock.json"
MANTINE_PACKAGE = "@mantine/core"
MANTINE_VERSION = "9.6.0"
MANTINE_TARBALL_INTEGRITY = (
    "sha512-WpdtSv9q2k4RrAVUxBlABMyEul4vv94EkjoVuQglBBC7jVmLhBBgI64HltE6di/"
    "l6zvj9D7k1vgkbVjyxQsn1g=="
)
# SHA-256 of @mantine/core@9.6.0/styles.layer.css as published by npm.
MANTINE_STYLES_LAYER_SHA256 = "cfeb20f197d77a6ebccd35e367eceb5facd7983f4c5943b0fb035f42e377b911"
LAYER_NAME = "mantine"

COMMENT = re.compile(r"/\*.*?\*/", re.S)
DESIGN_CHECKS = (
    "coverage-floor.py",
    "style-rules.py",
    "undefined-var.py",
    "shadowed.py",
    "theme-leak.py",
    "comment-boundaries.py",
    "percentage-min.py",
)
Canonicalizer = Callable[[Sequence[str]], list[str]]


def blank_comments(text: str) -> str:
    """Blank comments without moving line positions or changing brace balance."""
    return COMMENT.sub(lambda match: re.sub(r"[^\n]", " ", match.group(0)), text)


def normalized_css(text: str) -> str:
    """A deterministic fallback normalizer used by self-coverage fixtures.

    Production comparisons use Lightning CSS, which understands declarations and
    minifier transformations.  This intentionally modest normalizer keeps the
    parser/provenance functions independently testable without node_modules.
    """
    text = COMMENT.sub(" ", text)
    text = re.sub(r"\s+", " ", text).strip()
    text = re.sub(r"\s*([{}:;,>+~])\s*", r"\1", text)
    text = re.sub(r"(?<![\w.])0+\.(\d+)", r".\1", text)
    text = re.sub(r"\b0(?:px|em|rem|%|s|ms|deg|vh|vw|dvh|dvw)\b", "0", text)
    text = re.sub(r";}", "}", text)
    # Multiplication and division are operators only inside calc().
    text = re.sub(
        r"calc\(([^()]*)\)",
        lambda match: "calc(" + re.sub(r"\s*([*/])\s*", r"\1", match.group(1)) + ")",
        text,
    )
    return text


def _layer_spans(text: str) -> list[tuple[int, int]]:
    """Return complete ``@layer mantine { ... }`` spans in *text*.

    The scan runs over comments blanked in place, so braces in prose do not alter
    the depth while offsets still address the original text.
    """
    scrubbed = blank_comments(text)
    spans: list[tuple[int, int]] = []
    pattern = re.compile(r"@layer\s+" + re.escape(LAYER_NAME) + r"\s*\{")
    for match in pattern.finditer(scrubbed):
        opening = scrubbed.find("{", match.start(), match.end())
        depth = 1
        index = opening + 1
        quote: str | None = None
        escaped = False
        while index < len(scrubbed) and depth:
            char = scrubbed[index]
            if quote:
                if escaped:
                    escaped = False
                elif char == "\\":
                    escaped = True
                elif char == quote:
                    quote = None
            elif char in "\"'":
                quote = char
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
            index += 1
        if depth:
            # The caller reports an unclosed layer rather than treating the rest of
            # the bundle as authored CSS.
            spans.append((match.start(), len(text)))
        else:
            spans.append((match.start(), index))
    return spans


def remove_spans(text: str, spans: Iterable[tuple[int, int]]) -> str:
    pieces: list[str] = []
    end = 0
    for start, stop in spans:
        pieces.append(text[end:start])
        end = stop
    pieces.append(text[end:])
    return "".join(pieces)


def _lightning_css(texts: Sequence[str]) -> list[str]:
    """Canonicalize CSS through the compiler Vite installs for its CSS pipeline."""
    payload = json.dumps(list(texts))
    script = (
        "const fs = require('fs');"
        "const { transform } = require('lightningcss');"
        "const input = JSON.parse(fs.readFileSync(0, 'utf8'));"
        "const output = input.map((css) => transform({code: Buffer.from(css), minify: true}).code.toString());"
        "process.stdout.write(JSON.stringify(output));"
    )
    result = subprocess.run(
        ["node", "-e", script],
        cwd=ROOT / "web",
        input=payload,
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode:
        detail = (result.stderr or result.stdout).strip()
        raise RuntimeError("Lightning CSS could not canonicalize the bundle: " + detail)
    try:
        output = json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise RuntimeError("Lightning CSS returned invalid JSON") from error
    if not isinstance(output, list) or len(output) != len(texts) or not all(
        isinstance(item, str) for item in output
    ):
        raise RuntimeError("Lightning CSS returned an unexpected result")
    return output


def verify_mantine_source(
    path: Path, *, reference_bytes: bytes | None = None
) -> list[str]:
    """Verify package provenance; ``reference_bytes`` is private self-coverage injection."""
    failures: list[str] = []
    if not path.is_file():
        return [f"{path} is missing; the Mantine stylesheet was not installed"]
    if (
        path.name != "styles.layer.css"
        or path.parent.name != "core"
        or path.parent.parent.name != "@mantine"
    ):
        failures.append(
            f"{path} is not @mantine/core/styles.layer.css; the library ownership boundary is ambiguous"
        )
    package_json = path.parent / "package.json"
    try:
        package = json.loads(package_json.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as error:
        failures.append(f"{package_json} cannot prove Mantine package provenance: {error}")
        package = {}
    if package.get("name") != MANTINE_PACKAGE or package.get("version") != MANTINE_VERSION:
        failures.append(
            f"{package_json} is not {MANTINE_PACKAGE}@{MANTINE_VERSION}; got "
            f"{package.get('name', '<missing>')}@{package.get('version', '<missing>')}"
        )
    try:
        lock = json.loads(LOCKFILE.read_text(encoding="utf-8"))
        locked = lock["packages"]["node_modules/" + MANTINE_PACKAGE]
    except (OSError, KeyError, TypeError, json.JSONDecodeError) as error:
        failures.append(f"{LOCKFILE} does not prove the locked Mantine package: {error}")
        locked = {}
    if locked.get("version") != MANTINE_VERSION or locked.get("integrity") != MANTINE_TARBALL_INTEGRITY:
        failures.append(
            f"{LOCKFILE} does not lock {MANTINE_PACKAGE}@{MANTINE_VERSION} at the published tarball"
        )
    expected_hash = hashlib.sha256(reference_bytes).hexdigest() if reference_bytes is not None else MANTINE_STYLES_LAYER_SHA256
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if digest != expected_hash:
        failures.append(
            f"{path} was changed from the published Mantine stylesheet; expected SHA-256 {expected_hash}"
        )
    return failures


def check_ownership_texts(
    bundle_texts: Sequence[str],
    library_text: str,
    source_texts: Sequence[str],
    hooks: Sequence[str],
    *,
    canonicalizer: Canonicalizer = _lightning_css,
) -> list[str]:
    """Return ownership failures for texts; the canonicalizer is private-test injectable."""
    failures: list[str] = []
    if not bundle_texts:
        return ["no compiled CSS bundle was supplied"]
    if not source_texts:
        return ["no authored application stylesheet was supplied"]

    bundle = "\n".join(bundle_texts)
    spans = _layer_spans(bundle)
    if not spans:
        failures.append(
            "the compiled bundle has no @layer mantine block; import @mantine/core/styles.layer.css"
        )
        return failures
    if any(stop == len(bundle) and not blank_comments(bundle[start:]).rstrip().endswith("}") for start, stop in spans):
        failures.append("the compiled @layer mantine block is unclosed")
        return failures
    library_spans = _layer_spans(library_text)
    if not library_spans:
        failures.append("the installed Mantine source has no complete @layer mantine block")
        return failures

    emitted_library = "\n".join(bundle[start:stop] for start, stop in spans)
    emitted_app = remove_spans(bundle, spans)
    source = "\n".join(source_texts)
    try:
        canonical_library = canonicalizer([library_text])[0]
        canonical_emitted_library = canonicalizer([emitted_library])[0]
        canonical_source = canonicalizer([source])[0]
        canonical_emitted_app = canonicalizer([emitted_app])[0]
    except (OSError, RuntimeError, subprocess.SubprocessError) as error:
        return [str(error)]
    if canonical_emitted_library != canonical_library:
        failures.append(
            "the emitted @layer mantine CSS differs from the locked package stylesheet; "
            "do not edit or copy library rules into the application"
        )
    if not canonical_source.strip():
        failures.append("the authored application stylesheets contain no CSS rules")
    if canonical_emitted_app != canonical_source:
        failures.append(
            "compiled CSS outside @layer mantine is not exactly the canonical authored CSS; "
            "project styles may be missing or unowned rules may be present"
        )
    scrubbed_source = blank_comments(source)
    scrubbed_app = blank_comments(emitted_app)
    for hook in hooks:
        if not hook or hook not in scrubbed_source:
            failures.append(f"authored CSS does not declare required hook {hook!r}")
        if hook not in scrubbed_app:
            failures.append(f"compiled authored CSS does not retain required hook {hook!r}")
    return failures


def run_compiled_design_checks(css: str) -> list[str]:
    """Run the existing vendored checks on the non-library compiled remainder."""
    failures: list[str] = []
    with tempfile.NamedTemporaryFile("w", suffix=".css", encoding="utf-8") as output:
        output.write(css)
        output.flush()
        for name in DESIGN_CHECKS:
            checker = ROOT / "scripts" / "design-checks" / name
            result = subprocess.run(
                [sys.executable, str(checker), output.name],
                cwd=ROOT,
                capture_output=True,
                text=True,
                check=False,
            )
            if result.returncode:
                detail = (result.stdout + result.stderr).strip()
                failures.append(f"compiled authored CSS failed {name}:\n{detail}")
    return failures


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bundle", action="append", nargs="+", required=True, help="compiled CSS asset")
    parser.add_argument("--mantine-css", required=True, help="installed Mantine styles.layer.css")
    parser.add_argument(
        "--source-css", action="append", nargs="+", required=True, help="authored CSS input"
    )
    parser.add_argument("--hook", action="append", default=[], help="required authored CSS hook")
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    bundles = [Path(path) for group in args.bundle for path in group]
    sources = [Path(path) for group in args.source_css for path in group]
    library = Path(args.mantine_css)
    missing = [str(path) for path in [*bundles, *sources, library] if not path.is_file()]
    if missing:
        print("FAIL check-mantine-css: missing input: " + ", ".join(missing))
        return 1
    provenance = verify_mantine_source(library)
    if provenance:
        print("FAIL check-mantine-css: Mantine provenance is not trusted")
        for failure in provenance:
            print("  " + failure)
        return 1
    try:
        bundle_texts = [path.read_text(encoding="utf-8") for path in bundles]
        library_text = library.read_text(encoding="utf-8")
        source_texts = [path.read_text(encoding="utf-8") for path in sources]
    except OSError as error:
        print(f"FAIL check-mantine-css: {error}")
        return 1
    failures = check_ownership_texts(bundle_texts, library_text, source_texts, args.hook)
    if failures:
        print("FAIL check-mantine-css: CSS ownership or authored-hook coverage failed")
        for failure in failures:
            print("  " + failure)
        return 1
    app_css = remove_spans("\n".join(bundle_texts), _layer_spans("\n".join(bundle_texts)))
    failures = run_compiled_design_checks(app_css)
    if failures:
        print("FAIL check-mantine-css: compiled authored CSS checks failed")
        for failure in failures:
            print("  " + failure)
        return 1
    print(
        "check-mantine-css: passed; Mantine 9.6.0 styles.layer.css is provenance-verified "
        f"({library.stat().st_size} bytes), and {len(sources)} authored stylesheet(s) "
        "are present in the compiled bundle and passed the existing design checks"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
