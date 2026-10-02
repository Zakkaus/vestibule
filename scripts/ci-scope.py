#!/usr/bin/env python3
"""Select the CI jobs a pull request's changed paths can affect."""
import argparse
import fnmatch
import os
import subprocess
import sys

JOBS = ("go", "e2e", "static")
GO_PATHS = ("internal/", "cmd/", "migrations/", "testdata/", "deploy/")
E2E_BACKEND_PATHS = (
    "internal/console/", "internal/settings/", "internal/verification/", "internal/i18n/",
)
REFERENCE_PAGES = ("web/design.html", "web/architecture.html")


def path_scope(path: str) -> set[str]:
    if (path.startswith((".github/", "scripts/")) or path in ("go.mod", "go.sum")
            or ("/" not in path.removeprefix("web/")
                and fnmatch.fnmatchcase(path, "web/package*.json"))):
        return set(JOBS)
    if path.endswith(".md") or path.startswith("docs/") or path in REFERENCE_PAGES:
        return set()
    if path.startswith("web/"):
        return {"e2e"}
    if path.startswith(GO_PATHS):
        jobs = {"go", "static"}
        if path.startswith(E2E_BACKEND_PATHS):
            jobs.add("e2e")
        return jobs
    return set(JOBS)


def scope(paths: list[str], event: str) -> set[str]:
    if event != "pull_request":
        return set(JOBS)
    jobs = set()
    for path in paths:
        jobs.update(path_scope(path))
    return jobs


def self_test() -> int:
    cases = [
        ([], set()),
        (["README.md", "docs/guide.md"], set()),
        (["docs/diagram.svg"], set()),
        (["web/design.html", "web/architecture.html"], set()),
        (["web/src/app.tsx"], {"e2e"}),
        (["web/e2e/journey.spec.ts"], {"e2e"}),
        (["web/playwright.config.ts"], {"e2e"}),
        (["web/README.md"], set()),
        (["internal/console/api/handler.go"], set(JOBS)),
        (["internal/settings/store.go"], set(JOBS)),
        (["internal/verification/service.go"], set(JOBS)),
        (["internal/i18n/locales/en.json"], set(JOBS)),
        (["internal/feed/poll.go"], {"go", "static"}),
        (["cmd/vestibule/main.go"], {"go", "static"}),
        (["migrations/001.sql"], {"go", "static"}),
        (["testdata/state.json"], {"go", "static"}),
        (["deploy/vestibule.service"], {"go", "static"}),
        ([".github/workflows/ci.yml"], set(JOBS)),
        ([".github/README.md"], set(JOBS)),
        (["scripts/check-docs.py"], set(JOBS)),
        (["go.mod"], set(JOBS)),
        (["go.sum"], set(JOBS)),
        (["web/package.json"], set(JOBS)),
        (["web/package-lock.json"], set(JOBS)),
        (["web/packages/example.json"], {"e2e"}),
        (["unknown.conf"], set(JOBS)),
        (["other/file.go"], set(JOBS)),
        (["internal/feed/poll.go", "web/src/app.tsx"], set(JOBS)),
    ]
    for paths, expected in cases:
        actual = scope(paths, "pull_request")
        if actual != expected:
            print(f"FAIL ci-scope: {paths}: {actual} != {expected}")
            return 1
    for event in ("push", "workflow_dispatch", "unknown"):
        if scope(["README.md"], event) != set(JOBS):
            print(f"FAIL ci-scope: {event} must run every job")
            return 1
    print(f"ci-scope: passed; {len(cases)} path cases and 3 event cases")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--event", default=os.environ.get("GITHUB_EVENT_NAME", ""))
    parser.add_argument("--base")
    parser.add_argument("--head", default="HEAD")
    args = parser.parse_args()
    if args.self_test:
        return self_test()
    paths = []
    if args.event == "pull_request":
        if not args.base:
            parser.error("--base is required for pull_request")
        # Disable rename folding so moves out of a scoped tree include the old path.
        result = subprocess.run(
            ["git", "diff", "--name-only", "--no-renames", "-z", args.base, args.head, "--"],
            check=True, capture_output=True, text=True,
        )
        paths = [path for path in result.stdout.split("\0") if path]
    selected = scope(paths, args.event)
    for job in JOBS:
        print(f"{job}={str(job in selected).lower()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
