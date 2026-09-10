#!/usr/bin/env python3
"""Generate the notices shipped with Vestibule releases and containers.

The Go list is the CGO-disabled release graph. The browser list walks only the
runtime dependencies rooted at web/package.json, not build or test tooling.
The bot-api sidecar list follows deploy/Dockerfile.bot-api and its pinned source refs.
Every notice is copied from the pinned source archive or module cache.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
GO_MOD = ROOT / "go.mod"
GO_PACKAGE = "./cmd/bot"
LOCKFILE = ROOT / "web" / "package-lock.json"
LUCIDE_LICENSE = ROOT / "web" / "src" / "icons" / "lucide" / "LICENSE"
BOT_API_DOCKERFILE = ROOT / "deploy" / "Dockerfile.bot-api"
BOT_API_REPOSITORY = "https://github.com/tdlib/telegram-bot-api"
BOT_API_REF = "e3e9dd8e5b3d7ab8537cd5a10dc31d5ffa8f82d1"
TDLIB_REPOSITORY = "https://github.com/tdlib/td"
TDLIB_REF = "bc9c263e2bfee06aaab41e82db51a103376030bc"

def fetch_text(url: str) -> str:
    curl = shutil.which("curl")
    if curl:
        result = subprocess.run(
            [curl, "--fail", "--location", "--silent", "--show-error", "--max-time", "60", url],
            check=True,
            capture_output=True,
            timeout=75,
        )
        return result.stdout.decode("utf-8")
    with urllib.request.urlopen(url, timeout=60) as response:
        return response.read().decode("utf-8")


def docker_arg(name: str) -> str:
    prefix = f"ARG {name}="
    for line in BOT_API_DOCKERFILE.read_text(encoding="utf-8").splitlines():
        if line.startswith(prefix):
            return line.removeprefix(prefix)
    raise RuntimeError(f"{BOT_API_DOCKERFILE} has no {name} argument")


def verify_bot_api_pins() -> None:
    if docker_arg("TELEGRAM_BOT_API_REF") != BOT_API_REF:
        raise RuntimeError("generator TELEGRAM_BOT_API_REF disagrees with Dockerfile.bot-api")
    tree = json.loads(fetch_text(
        f"https://api.github.com/repos/tdlib/telegram-bot-api/git/trees/{BOT_API_REF}?recursive=1"
    ))
    td = next((entry for entry in tree.get("tree", []) if entry.get("path") == "td"), None)
    if not td or td.get("sha") != TDLIB_REF:
        actual = td.get("sha") if td else "missing"
        raise RuntimeError(f"telegram-bot-api {BOT_API_REF} points to TDLib {actual}, not {TDLIB_REF}")


def source_notice(url: str) -> str:
    return fetch_text(url).rstrip()

# The plan declares the shared Spectrum styles Apache-2.0. The source checkout
# has no LICENSE or NOTICE file, so this section deliberately asserts no owner.
APACHE_2_LICENSE = (ROOT / "scripts" / "apache-2.0.txt").read_text(encoding="utf-8")

def release_go_env() -> dict[str, str]:
    env = os.environ.copy()
    version = next(
        fields[1]
        for fields in (line.split() for line in GO_MOD.read_text(encoding="utf-8").splitlines())
        if len(fields) == 2 and fields[0] == "go"
    )
    env["GOTOOLCHAIN"] = f"go{version}"
    return env


def go_modules() -> list[tuple[str, str, Path]]:
    env = release_go_env()
    env["CGO_ENABLED"] = "0"
    result = subprocess.run(
        [
            "go",
            "list",
            "-deps",
            "-f",
            "{{if .Module}}{{.Module.Path}}\\t{{.Module.Version}}\\t{{.Module.Dir}}{{end}}",
            GO_PACKAGE,
        ],
        cwd=ROOT,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    )
    modules: dict[str, tuple[str, Path]] = {}
    for line in result.stdout.splitlines():
        fields = line.split("\\t", 2)
        if len(fields) != 3 or fields[0] == "github.com/Zakkaus/vestibule":
            continue
        modules[fields[0]] = (fields[1], Path(fields[2]))
    return [(name, version, directory) for name, (version, directory) in sorted(modules.items())]

def go_runtime_notice() -> tuple[str, list[Path]]:
    env = release_go_env()
    goroot = Path(subprocess.check_output(["go", "env", "GOROOT"], env=env, text=True).strip())
    version = subprocess.check_output(["go", "env", "GOVERSION"], env=env, text=True).strip()
    paths = [goroot / "LICENSE", goroot / "PATENTS"]
    if not all(path.is_file() for path in paths):
        missing = ", ".join(str(path) for path in paths if not path.is_file())
        raise RuntimeError(f"Go toolchain {version} has no root LICENSE/PATENTS notices: {missing}")
    return version, paths


def license_files(directory: Path) -> list[Path]:
    candidates = []
    for path in directory.iterdir():
        if not path.is_file():
            continue
        name = path.name.lower()
        if "license" in name or "copying" in name or "notice" in name:
            candidates.append(path)
    if not candidates:
        raise RuntimeError(f"no license or notice file in Go module {directory}")
    return sorted(candidates)


def npm_packages() -> list[tuple[str, dict]]:
    lock = json.loads(LOCKFILE.read_text(encoding="utf-8"))
    packages = lock["packages"]
    seen: set[str] = set()
    todo = list(packages[""]["dependencies"])
    while todo:
        name = todo.pop()
        if name in seen:
            continue
        key = f"node_modules/{name}"
        if key not in packages:
            raise RuntimeError(f"runtime dependency {name} is absent from {LOCKFILE}")
        seen.add(name)
        metadata = packages[key]
        todo.extend(metadata.get("dependencies", {}))
        todo.extend(metadata.get("optionalDependencies", {}))
    return [(name, packages[f"node_modules/{name}"]) for name in sorted(seen)]


def verified_npm_archive(resolved: str, expected: str) -> bytes:
    if not resolved:
        raise RuntimeError("npm package has no pinned tarball URL")
    if not expected.startswith("sha512-"):
        raise RuntimeError(f"npm package has no sha512 integrity: {resolved}")
    curl = shutil.which("curl")
    if curl:
        result = subprocess.run(
            [curl, "--fail", "--location", "--silent", "--show-error", "--max-time", "60", resolved],
            check=True,
            capture_output=True,
            timeout=75,
        )
        archive_bytes = result.stdout
    else:
        with urllib.request.urlopen(resolved, timeout=60) as response:
            archive_bytes = response.read()
    actual = "sha512-" + base64.b64encode(hashlib.sha512(archive_bytes).digest()).decode()
    if actual != expected:
        raise RuntimeError(f"integrity mismatch for {resolved}: got {actual}, want {expected}")
    return archive_bytes


def npm_license_files(metadata: dict) -> list[tuple[str, str]]:
    resolved = metadata.get("resolved")
    archive_bytes = verified_npm_archive(resolved, metadata.get("integrity", ""))
    with tarfile.open(fileobj=io.BytesIO(archive_bytes), mode="r:gz") as archive:
        files = []
        for member in archive.getmembers():
            basename = Path(member.name).name.lower()
            if not member.isfile() or not any(word in basename for word in ("license", "copying", "notice")):
                continue
            stream = archive.extractfile(member)
            if stream is None:
                continue
            # Some packages ship CRLF licence text. Writing it out verbatim and reading it
            # back translates the line endings, so the artifact could never match a fresh
            # render and the check failed on every run. Normalise once, here.
            text = stream.read().decode("utf-8").replace("\r\n", "\n").replace("\r", "\n")
            files.append((member.name.removeprefix("package/"), text))
    if not files and not metadata.get("license"):
        raise RuntimeError(f"npm package {resolved} has no license or notice file and declares none")
    return sorted(files)


def lucide_metadata() -> dict:
    metadata = json.loads(
        (ROOT / "web" / "src" / "icons" / "lucide" / "manifest.json").read_text(encoding="utf-8")
    )
    for key in ("version", "sourceTarball", "integrity", "license"):
        if not metadata.get(key):
            raise RuntimeError(f"Lucide manifest has no {key}")
    return metadata


def lucide_license(metadata: dict) -> str:
    files = npm_license_files({
        "resolved": metadata["sourceTarball"],
        "integrity": metadata["integrity"],
    })
    archive_license = next(
        (text for path, text in files if Path(path).name.lower() == "license"),
        None,
    )
    if archive_license is None:
        raise RuntimeError("Lucide archive has no LICENSE file")
    local_license = LUCIDE_LICENSE.read_text(encoding="utf-8").rstrip()
    if local_license != archive_license.rstrip():
        raise RuntimeError("vendored Lucide LICENSE differs from the verified source archive")
    return archive_license.rstrip()


def bot_api_sections() -> list[str]:
    verify_bot_api_pins()
    bot_api_license = source_notice(
        f"{BOT_API_REPOSITORY}/raw/{BOT_API_REF}/LICENSE_1_0.txt"
    )
    tdlib_license = source_notice(
        f"{TDLIB_REPOSITORY}/raw/{TDLIB_REF}/LICENSE_1_0.txt"
    )
    sqlite_license = source_notice(
        f"{TDLIB_REPOSITORY}/raw/{TDLIB_REF}/sqlite/sqlite/LICENSE"
    )
    tl_parser_license = source_notice(
        f"{TDLIB_REPOSITORY}/raw/{TDLIB_REF}/td/generate/tl-parser/LICENSE"
    )

    return [
        section(f"Bot API sidecar source @ {BOT_API_REF}", [
            f"Source repository: {BOT_API_REPOSITORY}",
            f"Source commit: {BOT_API_REF}",
            "License file: LICENSE_1_0.txt",
            "----- BEGIN LICENSE -----",
            bot_api_license,
            "----- END LICENSE -----",
        ]),
        section(f"TDLib sidecar submodule @ {TDLIB_REF}", [
            f"Source repository: {TDLIB_REPOSITORY}",
            f"Source commit: {TDLIB_REF}",
            "License file: LICENSE_1_0.txt",
            "----- BEGIN LICENSE -----",
            tdlib_license,
            "----- END LICENSE -----",
        ]),
        section(f"TDLib SQLite @ {TDLIB_REF}", [
            f"Source file: {TDLIB_REPOSITORY}/blob/{TDLIB_REF}/sqlite/sqlite/LICENSE",
            "License file: sqlite/sqlite/LICENSE",
            "----- BEGIN LICENSE -----",
            sqlite_license,
            "----- END LICENSE -----",
        ]),
        section(f"TDLib TL parser (build-only) @ {TDLIB_REF}", [
            f"Source file: {TDLIB_REPOSITORY}/blob/{TDLIB_REF}/td/generate/tl-parser/LICENSE",
            "This GPL-2.0 parser is built during the sidecar build and is not linked into the runtime image.",
            "License file: td/generate/tl-parser/LICENSE",
            "----- BEGIN LICENSE -----",
            tl_parser_license,
            "----- END LICENSE -----",
        ]),
        section("Alpine runtime package notices", [
            "The Docker build snapshots each final alpine:3.22 runtime stage before adding build-only tooling.",
            "The build-time source bundler appends the exact installed package closure and verified source notices to this artifact.",
            "The checked-in artifact does not claim a complete Go or web dependency inventory for either image.",
        ]),
    ]


def section(title: str, lines: list[str]) -> str:
    return f"## {title}\n\n" + "\n".join(lines).rstrip() + "\n\n"


def notice_lines(paths: list[Path]) -> list[str]:
    lines: list[str] = []
    for path in paths:
        lines.extend([
            f"License file: {path.name}",
            "----- BEGIN LICENSE -----",
            path.read_text(encoding="utf-8").rstrip(),
            "----- END LICENSE -----",
            "",
        ])
    return lines


def render() -> str:
    output: list[str] = [
        "\n".join([
            "THIRD-PARTY-LICENSES",
            "=====================",
            "",
            "Generated by scripts/generate-third-party-licenses.py.",
            "Go entries include the standard library/runtime and the CGO-disabled release graph.",
            "npm entries are the runtime graph rooted at web/package.json; dev-only build",
            "and test packages are intentionally excluded. Go/npm license text is copied from",
            "the pinned module cache or integrity-checked source archives; the shared styles use",
            "the canonical Apache-2.0 text in scripts/apache-2.0.txt.",
            "The sidecar entries below cover deploy/Dockerfile.bot-api only; they do not make",
            "the Go or web inventory complete for that image.",
            "",
            "",
        ])
    ]

    go_version, runtime_paths = go_runtime_notice()
    output.append(section(f"Go standard library/runtime @ {go_version}", [
        "Source: Go GOROOT LICENSE and PATENTS notices",
        *notice_lines(runtime_paths),
    ]))
    for name, version, directory in go_modules():
        output.append(section(f"Go: {name} @ {version}", [
            f"Source module: {name}@{version}",
            *notice_lines(license_files(directory)),
        ]))

    for name, metadata in npm_packages():
        license_lines: list[str] = []
        for path, text in npm_license_files(metadata):
            license_lines.extend([
                f"License file: {path}",
                "----- BEGIN LICENSE -----",
                text.rstrip(),
                "----- END LICENSE -----",
                "",
            ])
        if not license_lines:
            # A package may declare its terms in package.json and ship no separate file.
            # The declaration above is then the whole statement, and saying so is more
            # honest than an empty section that looks like a missing notice.
            license_lines = ["No license file ships in the tarball; the declaration above is the package's own."]
        output.append(section(f"npm: {name} @ {metadata['version']}", [
            f"Source tarball: {metadata['resolved']}",
            f"Tarball integrity: {metadata['integrity']}",
            f"Declared SPDX license: {metadata.get('license', 'not declared')}",
            *license_lines,
        ]))
    output.extend(bot_api_sections())


    lucide_metadata_value = lucide_metadata()
    lucide = lucide_license(lucide_metadata_value)
    output.append(section(
        f"Vendored Lucide Static icons @ {lucide_metadata_value['version']}",
        [
            "Source: web/src/icons/VENDORED.md and the vendored lucide/LICENSE file",
            f"Source tarball: {lucide_metadata_value['sourceTarball']}",
            f"Tarball integrity: {lucide_metadata_value['integrity']}",
            f"License: {lucide_metadata_value['license']}",
            "----- BEGIN LICENSE -----",
            lucide,
            "----- END LICENSE -----",
        ],
    ))
    output.append(section("Vendored Spectrum design-system styles", [
        "Files: web/src/styles/{tokens,components,shell}.css",
        "Source: shared design-system app/ styles (provenance: web/src/styles/VENDORED.md)",
        "License: Apache-2.0 (declared by the approved open-source acceptance plan)",
        "Upstream notice: the available source tree has no LICENSE or NOTICE file; no copyright holder is asserted here.",
        "----- BEGIN LICENSE -----",
        APACHE_2_LICENSE.rstrip(),
        "----- END LICENSE -----",
    ]))
    return "".join(output).rstrip() + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("-o", "--output", type=Path, default=ROOT / "THIRD-PARTY-LICENSES")
    parser.add_argument("--check", action="store_true", help="compare the checked-in artifact with a fresh render")
    args = parser.parse_args()
    expected = render()
    if args.check:
        if not args.output.is_file():
            print(f"FAIL third-party license check: {args.output} is missing")
            return 1
        actual = args.output.read_text(encoding="utf-8")
        if actual != expected:
            print(f"FAIL third-party license check: {args.output} differs from the generated inventory")
            return 1
        print(f"third-party license check: passed; {args.output.name} matches the generated inventory")
        return 0
    args.output.write_text(expected, encoding="utf-8")
    return 0


if __name__ == "__main__":
    sys.exit(main())
