#!/usr/bin/env python3
"""Reject a documented file:line citation that does not point at what it names.

Two rules, because a citation goes wrong in two ways.

A citation whose path this repository has must point at a line the file still holds, and when the
sentence around it names a code identifier, that identifier has to be at that line. Line numbers
drift with every edit to the file they point into, and a drifted number is worse than none: it
looks precise, so a reader follows it, lands on something unrelated, and doubts themselves.

A citation whose path this repository does not have is somebody else's -- the previous generation,
the tree before the rewrite, a Go module. Those are legitimate and deliberate, so each is declared
here by prefix with the reason it does not resolve. A prefix that stops matching anything is
removed rather than left behind, so the list cannot fill up with paths nobody cites any more.
"""

import re
import sys
from pathlib import Path

DOCUMENTS = [
    "docs/PLAN-v5.md",
    "docs/INVENTORY.md",
    "docs/ARCHITECTURE.md",
    "docs/README.md",
    "README.md",
    "README.zh-CN.md",
    "CONTRIBUTING.md",
    "web/architecture.html",
    "web/design.html",
]

# Paths that do not resolve here, each with the reason it does not. Remove a prefix when nothing
# cites it any more; the checker fails on a prefix that has stopped matching.
FOREIGN_PREFIXES = {
    "internal/verify/": "the previous generation's verification package",
    "verify/": "the previous generation, cited without its internal/ prefix",
    "cmd/gentoo-zh-verify-bot/": "the previous generation's command",
    "tg/": "the previous generation's Telegram package",
    "feed/": "the previous generation's feed package",
    "lookup/": "the previous generation's lookup package",
    "moderate/": "the previous generation's moderation package",
    "panel/": "the previous generation's panel package",
    "internal/store/": "this tree before the rewrite moved settings into internal/settings",
    "~/code/refs/": "an absolute path into the reference checkout",
    "telego@": "a package in the Go module cache",
    "state.go:": "a bare filename, quoted from a claim the plan goes on to reject",
    "server.go:": "a bare filename in a sentence that names the package around it",
}

# A citation's line may move by a few lines as its neighbourhood is edited. The window is what a
# reader would still accept as "there"; wider than this and the citation is not doing its job.
WINDOW_BEFORE = 8
WINDOW_AFTER = 6

# Code font is not required: a citation is a citation either way, and the ones written bare are
# exactly the ones nobody re-reads.
CITATION = re.compile(
    r"([A-Za-z0-9_./@~-]+\.(?:go|ts|tsx|sh|py|json|css|html|sql)):(\d+)(?:[–-]\d+)?"
)
# A continuation: the plan writes `import.go:194`, `:230` when both point into one file. It may sit
# on the next line, because prose wraps, so the last full citation carries across a paragraph.
CONTINUATION = re.compile(r"`:(\d+)(?:[–-]\d+)?`")
# Backticked identifiers. An identifier carries an uppercase letter or an underscore; without one
# it is an English word in code font ("error", "pending") and says nothing about the line.
IDENTIFIER = re.compile(r"`([A-Za-z_][A-Za-z0-9_]*)`")


def identifiers(line: str) -> list[str]:
    found = []
    for name in IDENTIFIER.findall(line):
        if len(name) < 4:
            continue
        if not any(character.isupper() or character == "_" for character in name):
            continue
        found.append(name)
    return found


def main() -> int:
    failures: list[str] = []
    resolved = 0
    foreign_used: set[str] = set()
    documents_with_citations = 0

    for name in DOCUMENTS:
        document = Path(name)
        if not document.exists():
            failures.append(f"{name}: listed for checking but not present")
            continue
        lines = document.read_text(encoding="utf-8").split("\n")
        seen_here = False
        carried = None
        for number, line in enumerate(lines, 1):
            if not line.strip():
                carried = None
            citations = [(m.group(1), int(m.group(2))) for m in CITATION.finditer(line)]
            if citations:
                carried = citations[0][0]
            for match in CONTINUATION.finditer(line):
                if carried is not None:
                    citations.append((carried, int(match.group(1))))
            for path_text, cited_line in citations:
                seen_here = True
                target = Path(path_text)
                if not target.exists():
                    prefix = next(
                        (p for p in FOREIGN_PREFIXES if path_text.startswith(p) or f"{path_text}:".startswith(p)),
                        None,
                    )
                    if prefix is None:
                        failures.append(
                            f"{name}:{number}: {path_text}:{cited_line} names a path this "
                            f"repository does not have, and no declared prefix covers it"
                        )
                    else:
                        foreign_used.add(prefix)
                    continue
                resolved += 1
                body = target.read_text(encoding="utf-8").split("\n")
                if cited_line > len(body):
                    failures.append(
                        f"{name}:{number}: {path_text}:{cited_line} is past the end of a "
                        f"{len(body)}-line file"
                    )
                    continue
                names = identifiers(line)
                if not names:
                    continue
                window = "\n".join(
                    body[max(0, cited_line - 1 - WINDOW_BEFORE) : min(len(body), cited_line + WINDOW_AFTER)]
                )
                if not any(identifier in window for identifier in names):
                    failures.append(
                        f"{name}:{number}: {path_text}:{cited_line} names "
                        f"{', '.join(names)}, and none of them is within "
                        f"{WINDOW_BEFORE} lines before or {WINDOW_AFTER} after"
                    )
        if seen_here:
            documents_with_citations += 1

    # A checker that finds nothing passes for the wrong reason. Both halves have to have read
    # something: citations that resolve, and documents that carry any.
    if resolved == 0:
        failures.append("no citation resolved to a file in this repository; the checker read nothing")
    if documents_with_citations == 0:
        failures.append("no listed document carries a citation; the document list is wrong")

    unused = sorted(set(FOREIGN_PREFIXES) - foreign_used)
    for prefix in unused:
        failures.append(
            f"the declared prefix {prefix!r} ({FOREIGN_PREFIXES[prefix]}) is cited nowhere; remove it"
        )

    if failures:
        print("FAIL check-citations-resolve:")
        for failure in failures:
            print(f"  {failure}")
        return 1
    print(
        f"check-citations-resolve: passed; {resolved} citations resolve and point at what they name, "
        f"{len(foreign_used)} declared foreign prefixes all still cited, "
        f"across {documents_with_citations} documents"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
