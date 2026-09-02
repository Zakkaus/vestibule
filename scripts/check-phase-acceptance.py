#!/usr/bin/env python3
"""Require acceptance coverage for every completed plan phase.

A completed phase is covered by its executable scripts/accept-phaseN.sh script,
or by one explicit reason in scripts/phase-acceptance-exemptions.txt. The latter
exists for a phase that cannot have a runnable acceptance script at all; a
clause-level exemption belongs in that phase's script and must be printed when
it runs.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_PLAN = ROOT / "docs" / "PLAN-v5.md"
DEFAULT_SCRIPTS = ROOT / "scripts"
DEFAULT_EXEMPTIONS = DEFAULT_SCRIPTS / "phase-acceptance-exemptions.txt"

PHASE_NUMBERS = {
    "零": 0,
    "一": 1,
    "二": 2,
    "三": 3,
    "四": 4,
    "五": 5,
    "六": 6,
    "七": 7,
    "八": 8,
    "九": 9,
    "十": 10,
}
PHASE_ROW = re.compile(r"^\|\s*(零|一|二|三|四|五|六|七|八|九|十)\s*\|.*\|\s*(完成|未开始)\s*\|\s*$")
EXEMPTION_ROW = re.compile(r"^(\d+)\t(.+\S)$")


def completed_phases(plan: Path) -> list[int]:
    if not plan.is_file():
        raise ValueError(f"plan target is missing: {plan}")
    phases = []
    for line in plan.read_text(encoding="utf-8").splitlines():
        match = PHASE_ROW.match(line)
        if match and match.group(2) == "完成":
            phases.append(PHASE_NUMBERS[match.group(1)])
    if not phases:
        raise ValueError(f"no completed phase rows found in: {plan}")
    return phases


def exemptions(path: Path) -> dict[int, str]:
    if not path.exists():
        return {}
    if not path.is_file():
        raise ValueError(f"exemption target is not a file: {path}")
    found = {}
    for line_number, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        match = EXEMPTION_ROW.match(line)
        if not match:
            raise ValueError(
                f"{path}:{line_number}: expected phase-number, tab, and non-empty reason"
            )
        phase, reason = int(match.group(1)), match.group(2)
        if phase in found:
            raise ValueError(f"{path}:{line_number}: phase {phase} has more than one exemption")
        found[phase] = reason
    return found


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", type=Path, default=DEFAULT_PLAN)
    parser.add_argument("--scripts-dir", type=Path, default=DEFAULT_SCRIPTS)
    parser.add_argument("--exemptions", type=Path, default=DEFAULT_EXEMPTIONS)
    args = parser.parse_args()

    try:
        phases = completed_phases(args.plan)
        exemptions_by_phase = exemptions(args.exemptions)
    except ValueError as error:
        print(f"FAIL check-phase-acceptance: {error}", file=sys.stderr)
        return 1

    if not args.scripts_dir.is_dir():
        print(
            f"FAIL check-phase-acceptance: acceptance script directory is missing: {args.scripts_dir}",
            file=sys.stderr,
        )
        return 1

    failures = []
    for phase in phases:
        script = args.scripts_dir / f"accept-phase{phase}.sh"
        relative_script = script.as_posix()
        if script.is_file():
            if script.stat().st_mode & 0o111:
                print(f"ok phase {phase}: {relative_script}")
            else:
                failures.append(f"phase {phase}: {relative_script} is not executable")
            if phase in exemptions_by_phase:
                failures.append(
                    f"phase {phase}: {relative_script} exists and an exemption is still "
                    f"written for it; the exemption describes work that is now done"
                )
            continue
        reason = exemptions_by_phase.get(phase)
        if reason:
            print(f"EXEMPT phase {phase}: {reason}")
            continue
        failures.append(
            f"phase {phase}: missing {relative_script} and no written exemption"
        )

    stale = sorted(set(exemptions_by_phase) - set(phases))
    for phase in stale:
        failures.append(
            f"phase {phase}: exemption exists but the plan does not mark it completed"
        )

    if failures:
        for failure in failures:
            print(f"FAIL check-phase-acceptance: {failure}", file=sys.stderr)
        return 1

    print(f"check-phase-acceptance: passed; {len(phases)} completed phases covered")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
