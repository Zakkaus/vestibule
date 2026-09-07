"""Validate source ownership of Spectrum's build-time CSS output."""
from __future__ import annotations

import re
from pathlib import Path
from typing import Any

MACRO_IMPORT = re.compile(
    r"from\s+['\"]@react-spectrum/s2/style['\"]\s+with\s*\{\s*type:\s*['\"]macro['\"]\s*\}"
)
MACRO_VARIABLE = re.compile(r"['\"](--[\w-]+)['\"]\s*:\s*\{")
STYLE_IMPORT = re.compile(
    r"import\s*\{[^}]*\bstyle\b[^}]*\}\s*from\s+['\"]@react-spectrum/s2/style['\"]"
    r"\s+with\s*\{\s*type:\s*['\"]macro['\"]\s*\}"
)


def is_style_expression(text: str, expression: str) -> bool:
    """Only a real build-time import can supply a generated class expression."""
    return bool(STYLE_IMPORT.search(text) and
                re.fullmatch(r"\s*style\s*\(\s*\{.*\}\s*\)\s*", expression, re.S))


def macro_definitions(
    frontend: Path, sources: list[Path], assets: list[dict[str, Any]], emitted: set[str]
) -> set[str]:
    declared = {
        frontend / origin["path"]
        for asset in assets
        for origin in asset["origins"]
        if origin["kind"] == "spectrum-macro"
    }
    imported = {
        path for path in sources
        if path.suffix in {".ts", ".tsx"}
        and MACRO_IMPORT.search(path.read_text(encoding="utf-8"))
    }
    missing = imported - declared
    extra = declared - imported
    if missing:
        raise ValueError("Spectrum macro sources missing from provenance: " +
                         ", ".join(str(path) for path in sorted(missing)))
    if extra:
        raise ValueError("Spectrum macro origins have no Spectrum macro import: " +
                         ", ".join(str(path) for path in sorted(extra)))
    definitions: set[str] = set()
    for path in declared:
        definitions.update(MACRO_VARIABLE.findall(path.read_text(encoding="utf-8")))
    absent = definitions - emitted
    if absent:
        raise ValueError("Spectrum macro variables missing from emitted CSS: " +
                         ", ".join(sorted(absent)))
    return definitions
