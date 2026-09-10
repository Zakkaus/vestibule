#!/usr/bin/env python3
"""The built console fetches nothing from a third party.

The console is installed on other people's servers. Every remote URL the bundle
keeps is a request those servers' visitors make to someone else, and an install
without outbound access waits for it instead of failing fast: Spectrum's Adobe
Clean @font-face rules did exactly that, and the wait is what hung
document.fonts.ready in CI.

Fonts, images and any other asset must be inlined or served from the instance.
A data: URL is fine. Anything in a CSS url(...) token with a scheme is not.
When scanning JavaScript bundles, ordinary links and namespace URLs are ignored; embedded
CSS url(...) tokens are still checked.

Usage: check-no-external-assets.py <css or js file>...
"""
import re
import sys
from pathlib import Path

REMOTE = re.compile(r"""url\(\s*['"]?([a-zA-Z][a-zA-Z0-9+.-]*:)""")
ALLOWED = {"data:"}


def main(paths: list[str]) -> int:
    if not paths:
        print("check-no-external-assets: no asset file given", file=sys.stderr)
        return 2
    findings = []
    for name in paths:
        text = Path(name).read_text(encoding="utf-8")
        for match in REMOTE.finditer(text):
            scheme = match.group(1)
            if scheme in ALLOWED:
                continue
            line = text.count("\n", 0, match.start()) + 1
            findings.append(f"FAIL check-no-external-assets: {name}:{line} loads {scheme}// from outside the instance")
    for finding in sorted(set(findings)):
        print(finding)
    if findings:
        return 1
    print(f"check-no-external-assets: passed; {len(paths)} asset files, every asset is inline or local")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
