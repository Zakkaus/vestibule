#!/usr/bin/env python3
"""Check the checked-in inventory against a fresh source-backed render."""
from __future__ import annotations

import argparse
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
GENERATOR = ROOT / "scripts" / "generate-third-party-licenses.py"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--artifact", type=Path, default=ROOT / "THIRD-PARTY-LICENSES")
    args = parser.parse_args()
    result = subprocess.run(
        [sys.executable, str(GENERATOR), "--check", "--output", str(args.artifact)],
        cwd=ROOT,
        check=False,
    )
    return result.returncode


if __name__ == "__main__":
    sys.exit(main())
