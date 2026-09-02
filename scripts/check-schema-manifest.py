#!/usr/bin/env python3
"""Verify released schema manifests match the current migrations.Table."""
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
GENERATOR = ("go", "run", "./cmd/schema-manifest")


def target_path(raw_target: str) -> Path:
    path = Path(raw_target)
    return path if path.is_absolute() else ROOT / path


def check_manifest(path: Path) -> str | None:
    try:
        result = subprocess.run(
            [*GENERATOR, "-check", str(path)],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
    except OSError as error:
        return f"cannot run schema-manifest generator: {error}"
    if result.returncode == 0:
        return None
    detail = (result.stderr or result.stdout).strip()
    return detail or f"schema-manifest generator exited {result.returncode}"


def main(argv: list[str]) -> int:
    failures: list[str] = []
    checked = 0
    for raw_target in argv:
        path = target_path(raw_target)
        if not path.is_file():
            failures.append(f"target does not exist: {raw_target}")
            continue
        checked += 1
        if error := check_manifest(path):
            failures.append(f"{raw_target}: {error}")

    if checked == 0:
        failures.append("coverage is zero: no schema manifest targets were checked")
    if failures:
        print("FAIL check-schema-manifest:")
        for failure in failures:
            print("  " + failure)
        return 1

    print(
        "check-schema-manifest: passed; "
        f"{checked} manifest target(s) match migrations.Table"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
