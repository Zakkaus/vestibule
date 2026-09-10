#!/usr/bin/env python3
"""Reject deployment identities in shipped code and active design examples.

This repository is a general-purpose deployment: every instance runs its own
bot, in its own groups, for its own community. A handle, supergroup ID,
deployment domain, or known community name written into the bundle names somebody else's deployment,
and it does so in the copy a visitor reads before anything else exists.

Test files are excluded. UI fixture modules enter the production bundle, so
they must also avoid deployment identities. Upstream service and optional-module
domains remain allowed; this gate only rejects the known deployment domain
from this source tree.
"""

from __future__ import annotations

import pathlib
import re
import sys

HANDLE = re.compile(r"@([A-Za-z][A-Za-z0-9_]{3,30}[Bb]ot)\b")
SUPERGROUP_ID = re.compile(r"(?<![0-9])-100[0-9]{9,10}(?![0-9])")
DOMAIN = re.compile(
    r"(?<![A-Za-z0-9-])(?:[A-Za-z0-9-]+\.)+[A-Za-z]{2,63}(?![A-Za-z0-9-])",
    re.IGNORECASE,
)
COMMUNITY = re.compile(
    r"Gentoo(?:[- ]zh| Chinese) Community|Gentoo 中文社[区群]|"
    r"Arch Linux (?:Chinese Community|中文社[区群])|\bOld OT\b|老 OT",
    re.IGNORECASE,
)
UNIVERSAL = {"BotFather", "botfather"}
RESERVED_SYNTHETIC_PREFIX = "-1009"
# This was the old public deployment's domain. It appears in compatibility
# fixtures, but it is not an upstream dependency or a required service.
DEPLOYMENT_DOMAINS = {"gentoozh.org"}
SEARCHED = (
    ("web/src", (".ts", ".tsx", ".json")),
    ("internal", (".go", ".json", ".yaml")),
    ("cmd", (".go",)),
)
EXEMPT_SUFFIXES = (".spec.ts", "_test.go", ".test.ts", ".test.tsx")


def searched_files(root: pathlib.Path) -> list[pathlib.Path]:
    found: list[pathlib.Path] = []
    for directory, suffixes in SEARCHED:
        base = root / directory
        if not base.is_dir():
            print(f"FAIL check-no-baked-identity: {directory} is missing, so nothing was searched")
            raise SystemExit(1)
        for path in sorted(base.rglob("*")):
            if not path.is_file() or path.suffix not in suffixes:
                continue
            if path.name.endswith(EXEMPT_SUFFIXES):
                continue
            found.append(path)
    design = root / "web/design.html"
    if not design.is_file():
        print("FAIL check-no-baked-identity: web/design.html is missing")
        raise SystemExit(1)
    found.append(design)
    return found


def is_deployment_domain(domain: str) -> bool:
    candidate = domain.rstrip(".").lower()
    return any(
        candidate == deployed or candidate.endswith("." + deployed)
        for deployed in DEPLOYMENT_DOMAINS
    )


def scan_line(path: pathlib.Path, number: int, line: str) -> list[str]:
    findings: list[str] = []
    for match in HANDLE.finditer(line):
        handle = match.group(1)
        if handle in UNIVERSAL:
            continue
        findings.append(
            f"{path}:{number} names @{handle}; every instance runs its own bot, so this "
            "handle has to come from the instance rather than from the build"
        )
    for match in SUPERGROUP_ID.finditer(line):
        identifier = match.group()
        if identifier.startswith(RESERVED_SYNTHETIC_PREFIX):
            continue
        findings.append(
            f"{path}:{number} names Telegram supergroup {identifier}; every instance "
            "chooses its own groups, so this identifier has to come from the instance "
            "rather than from the build"
        )
    for match in DOMAIN.finditer(line):
        domain = match.group()
        if not is_deployment_domain(domain):
            continue
        findings.append(
            f"{path}:{number} names deployment domain {domain}; every instance chooses "
            "its own service address, so this domain has to come from the instance rather "
            "than from the build"
        )
    for match in COMMUNITY.finditer(line):
        findings.append(
            f"{path}:{number} names known deployment community {match.group()}; "
            "the community name has to come from the instance rather than from the build"
        )
    return findings


def main() -> int:
    root = pathlib.Path(__file__).resolve().parent.parent
    files = searched_files(root)
    if not files:
        print("FAIL check-no-baked-identity: no file matched the search, so nothing was checked")
        return 1

    problems = []
    for path in files:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
        for number, line in enumerate(lines, start=1):
            problems.extend(scan_line(path.relative_to(root), number, line))

    for problem in problems:
        print(f"FAIL check-no-baked-identity: {problem}")
    if problems:
        return 1
    print(
        f"check-no-baked-identity: passed; {len(files)} code and design files, no deployment "
        "bot handle, deployed supergroup ID, or known deployment domain/community name"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
