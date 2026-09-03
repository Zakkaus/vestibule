#!/usr/bin/env python3
"""The release workflow gates everything CI gates, or says why it does not.

The release step calls itself a test gate so that binaries never come from a broken commit. It is
a hand-written list, and a hand-written copy of another list drifts the moment somebody adds a
gate to one of them. That already happened: the citation gate was added to CI and to CONTRIBUTING
in the same change, and not here, because nothing was looking.

CI is the authority. Every gate CI runs must run before a tag publishes anything, or be named
below with the reason it cannot. A reason that stops applying — because CI no longer runs that
gate — fails this check rather than sitting in the list, which is the rule the acceptance
exemptions and the citation prefixes already carry.

What this compares is invocations, not command lines: the release legitimately runs the same gate
with different arguments.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CI = ROOT / ".github" / "workflows" / "ci.yml"
RELEASE = ROOT / ".github" / "workflows" / "release.yml"

# Gates CI runs that a tag deliberately does not, each with the reason it cannot run here.
EXCLUDED = {
    "scripts/check-baseline-ratchet.py":
        "it compares this commit's baseline against the pull request's base branch, and a tag has "
        "no base to compare against",
    "npm run build":
        "the release publishes the Go binaries, the images and the deployment assets; the console "
        "bundle is not among them, and nothing in the binary embeds it",
    "npm run e2e":
        "it drives a real browser against the console bundle the release does not publish",
    "Zakk-LLM/Chinese-skill":
        "it reads the prose in the repository, which no published asset carries",
    # The console is not among the published assets: the release ships the Go binaries, the images
    # and the deployment files, and nothing in the binary embeds a bundle. These four read the
    # console's own sources, so a tag cannot publish anything they would have caught.
    "scripts/check-console-copy.py": "it reads the console sources, which the release does not publish",
    "scripts/check-phase-seams.py": "it reads the console screens, which the release does not publish",
    "scripts/check-writing-screens-know-a-stale-token.py":
        "it reads the console screens, which the release does not publish",
    "scripts/check-error-maps-hold-real-codes.py":
        "it reads the console screens, which the release does not publish",
    "scripts/design-checks": "they read the console stylesheets and the two reference pages, none of which the release publishes",
}

# Command-line gates that are not a repository script. Each is a substring CI's text contains.
TOOL_GATES = {
    "gofmt -l": "gofmt",
    "go vet ./...": "go vet",
    "cmd/staticcheck": "staticcheck",
    "go build ./...": "go build",
    "go build -tags gentoo": "go build (gentoo tag)",
    "go test -race ./...": "go test -race",
    "go test -race -tags gentoo": "go test -race (gentoo tag)",
    "cmd/govulncheck": "govulncheck",
    "cmd/gosec": "gosec",
}


def invocations(text: str) -> set:
    found = set()
    for match in re.finditer(r"(?<![A-Za-z0-9_./~-])scripts/[A-Za-z0-9_./-]+\.(?:py|sh)", text):
        found.add(match.group(0))
    for match in re.finditer(r"npm run ([a-z0-9:-]+)", text):
        found.add("npm run " + match.group(1))
    for match in re.finditer(r"uses:\s*([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@", text):
        if not match.group(1).startswith("actions/") and not match.group(1).startswith("docker/"):
            found.add(match.group(1))
    return found


def globbed_directories(text: str) -> set:
    """A loop over scripts/design-checks/$c.py covers every script in that directory."""
    return {
        match.rsplit("/", 1)[0]
        for match in re.findall(r"scripts/[A-Za-z0-9_./-]*\$[A-Za-z0-9_{}]+[A-Za-z0-9_./-]*", text)
    }


def main() -> int:
    for path in (CI, RELEASE):
        if not path.exists():
            print(f"FAIL check-release-gate: {path.name} is missing")
            return 1
    ci = CI.read_text(encoding="utf-8")
    release = RELEASE.read_text(encoding="utf-8")

    ci_gates = invocations(ci)
    release_gates = invocations(release)
    release_dirs = globbed_directories(release)
    ci_dirs = globbed_directories(ci)

    failures = []
    for gate in sorted(ci_gates):
        if gate in release_gates:
            continue
        if "/" in gate and gate.rsplit("/", 1)[0] in release_dirs:
            continue
        if gate in EXCLUDED:
            continue
        failures.append(
            f"CI runs {gate} and the release does not, and no reason is recorded for leaving it out"
        )
    for directory in sorted(ci_dirs):
        if directory not in release_dirs and directory not in EXCLUDED:
            failures.append(
                f"CI runs every script in {directory}/ and the release does not, and no reason is recorded"
            )

    for name, label in sorted(TOOL_GATES.items()):
        if name in ci and name not in release and name not in EXCLUDED:
            failures.append(f"CI runs {label} and the release does not, and no reason is recorded")

    for gate in sorted(EXCLUDED):
        if gate in ci_gates or gate in ci or gate in ci_dirs:
            continue
        failures.append(
            f"{gate!r} is excused from the release gate, and CI does not run it any more; remove the entry"
        )

    # A comparison that reads nothing passes for the wrong reason.
    if len(ci_gates) < 10:
        failures.append(f"only {len(ci_gates)} gates were read out of CI; the parser is wrong")
    if len(release_gates) < 5:
        failures.append(f"only {len(release_gates)} gates were read out of the release; the parser is wrong")

    if failures:
        print("FAIL check-release-gate:")
        for failure in failures:
            print(f"  {failure}")
        print()
        print("A tag publishes binaries, images and deployment assets. Whatever CI refuses to let")
        print("into main must also stop a tag, or the reason it cannot must be written down.")
        return 1

    print(
        f"check-release-gate: passed; {len(ci_gates)} gates in CI, {len(release_gates)} in the "
        f"release, {len(EXCLUDED)} excused and every excuse still applies"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
