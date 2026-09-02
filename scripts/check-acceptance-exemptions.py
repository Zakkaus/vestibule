#!/usr/bin/env python3
"""Hold every acceptance exemption to a reason that can still be true.

check-phase-acceptance.py already refuses a completed phase with no acceptance script
and no written exemption. It reads the exemption's presence, not its content, so a
reason keeps passing long after it stops being true. Four had gone stale at once:

  - two named phase nine as the owner of the missing work, and phase nine completed.
    One of them was by then satisfied by a case phase nine's own acceptance runs, so
    the exemption was hiding a check that would have passed.
  - three said a decision was pending and cited a plan line by number. The plan grew,
    the numbers drifted onto unrelated prose, and the decisions were made.

Two rules follow from those, and each has subjects today:

  1. An exemption naming a phase as the owner of the missing work may not name a phase
     the plan marks complete. A completed phase either delivered the thing or did not,
     and either way the reason has to say something else.
  2. An exemption may not say a decision is still open without citing a row of the
     open-questions table that is still open. A bare claim of openness is the form the
     three stale reasons took.

Coverage is asserted: finding no exemption at all is a failure, because the way this
check would rot is a rename that makes every exemption invisible to it.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLAN = ROOT / "docs" / "PLAN-v5.md"

ORDINALS = {
    "zero": "零", "one": "一", "two": "二", "three": "三", "four": "四",
    "five": "五", "six": "六", "seven": "七", "eight": "八", "nine": "九",
    "ten": "十", "eleven": "十一",
}
OPEN_CLAIM = re.compile(r"\b(pending|undecided|not decided|defers?|deferred)\b", re.I)
CITES_ROW = re.compile(r"PLAN-v5\.md:(\d+)")


def phase_states(plan: str) -> dict:
    states = {}
    for row in re.finditer(r"^\| ([零一二三四五六七八九十]+) \|[^|]*\|[^|]*\| ([^|]+) \|$",
                           plan, re.M):
        states[row.group(1)] = row.group(2).strip()
    return states


def open_questions_region(plan: str) -> range:
    lines = plan.splitlines()
    start = end = None
    for number, line in enumerate(lines, 1):
        if line.startswith("## 4. 待决"):
            start = number
        elif start is not None and line.startswith("## 5."):
            end = number
            break
    if start is None:
        return range(0)
    return range(start, end or len(lines) + 1)


def exemptions(path: Path) -> list:
    """Return (line number, label, reason) for each exemption call in one script."""
    found = []
    lines = path.read_text().splitlines()
    for index, line in enumerate(lines):
        if not re.match(r"^(exempt|run_then_exempt) \"", line):
            continue
        label = re.search(r"\"([^\"]*)\"", line).group(1)
        reason = ""
        for follow in lines[index + 1:]:
            quoted = re.search(r"\"([^\"]*)\"", follow)
            if quoted:
                reason = quoted.group(1)
            break
        found.append((index + 1, label, reason))
    return found


def main() -> int:
    plan = PLAN.read_text()
    states = phase_states(plan)
    if not states:
        print("FAIL check-acceptance-exemptions: the phase table in %s parsed to nothing, "
              "so no reason can be judged against it" % PLAN)
        return 1
    still_open = open_questions_region(plan)

    failures = []
    seen = 0
    for script in sorted((ROOT / "scripts").glob("accept-phase*.sh")):
        for number, label, reason in exemptions(script):
            seen += 1
            where = "%s:%d" % (script.relative_to(ROOT), number)
            if not reason:
                failures.append("%s exempts %r with no reason" % (where, label))
                continue
            for word, digit in ORDINALS.items():
                if re.search(r"\bphase %s\b" % word, reason, re.I):
                    if digit not in states:
                        failures.append(
                            "%s exempts %r on the strength of phase %s, and the plan's "
                            "phase table has no row for it; a row this check cannot read "
                            "is a phase it silently stops judging"
                            % (where, label, word))
                        continue
                    state = states[digit]
                    if state == "完成":
                        failures.append(
                            "%s exempts %r because phase %s owns the work, and the plan "
                            "marks that phase 完成; say what is actually missing"
                            % (where, label, word))
            if OPEN_CLAIM.search(reason):
                cited = [int(n) for n in CITES_ROW.findall(reason)]
                if not any(line in still_open for line in cited):
                    failures.append(
                        "%s exempts %r by calling a decision open without citing a row of "
                        "the open-questions table; cite PLAN-v5.md:<line> inside it, or "
                        "state the blocker instead" % (where, label))

    if seen == 0:
        print("FAIL check-acceptance-exemptions: no exemption was found in any "
              "scripts/accept-phase*.sh, so this check read nothing")
        return 1
    for failure in failures:
        print("FAIL check-acceptance-exemptions: %s" % failure)
    if failures:
        return 1
    print("check-acceptance-exemptions: passed; %d exemptions, each with a reason that "
          "does not rest on a completed phase or an undecided question" % seen)
    return 0


if __name__ == "__main__":
    sys.exit(main())
