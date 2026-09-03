#!/usr/bin/env python3
"""The console entry document and CSS fixture remain valid, exercised HTML inputs.

Vite consumes web/index.html and React requires exactly one #root mount point. The app CSS
fixture is the executable inventory paired with app.css. Both files used to sit outside every
HTML check, so malformed markup could recover differently in browsers or make the console blank.

Usage: check-console-html.py
"""
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
INDEX = ROOT / "web" / "index.html"
FIXTURE = ROOT / "web" / "src" / "app" / "app.css.fixture.html"
STRUCTURE_CHECK = ROOT / "scripts" / "design-checks" / "html-structure.py"


def console_entry_has_one_mount_point() -> list[str]:
    try:
        text = INDEX.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        return [f"cannot read web/index.html ({getattr(error, 'strerror', None) or error.__class__.__name__})"]
    failures = []
    roots = len(re.findall(r'''\bid=(?:"root"|'root')''', text))
    if roots != 1:
        failures.append(
            f"web/index.html defines {roots} #root mount points instead of one; React can leave the console blank"
        )
    entries = len(re.findall(r'''<script\b[^>]*\bsrc=(?:"/src/main\.tsx"|'/src/main\.tsx')''', text))
    if entries != 1:
        failures.append(
            f"web/index.html loads /src/main.tsx {entries} times instead of once; the console cannot start reliably"
        )
    return failures


def checked_inputs_are_structurally_valid() -> list[str]:
    command = [sys.executable, str(STRUCTURE_CHECK), str(INDEX), str(FIXTURE)]
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, check=False)
    if result.returncode == 0:
        return []
    detail = (result.stdout + result.stderr).strip().replace("\n", "\n    ")
    return [
        "web/index.html or app.css.fixture.html is malformed; browser recovery can move "
        f"content under the wrong parent:\n    {detail}"
    ]


def main() -> int:
    failures = console_entry_has_one_mount_point()
    failures.extend(checked_inputs_are_structurally_valid())
    if failures:
        print("FAIL check-console-html:")
        for failure in failures:
            print("  " + failure)
        return 1
    print("check-console-html: passed; Vite entry and app CSS fixture are structurally valid")
    return 0


if __name__ == "__main__":
    sys.exit(main())
