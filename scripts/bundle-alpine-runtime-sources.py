#!/usr/bin/env python3
"""Bundle the exact Alpine runtime package sources and notices for release images."""
from __future__ import annotations

import argparse
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request
import zipfile

APORTS_ARCHIVE = (
    "https://gitlab.alpinelinux.org/alpine/aports/-/archive/"
    "{commit}/aports-{commit}.tar.gz?path=main/{origin}"
)
NOTICE_WORDS = ("license", "copying", "notice", "copyright")

def safe_target(root: Path, name: str) -> Path:
    target = (root / name).resolve()
    resolved_root = root.resolve()
    if target != resolved_root and resolved_root not in target.parents:
        raise RuntimeError(f"unsafe source path: {name}")
    return target


def is_notice(path: Path) -> bool:
    basename = path.name.lower()
    return any(word in basename for word in NOTICE_WORDS)

def installed_packages(path: Path) -> list[dict[str, str]]:
    packages: list[dict[str, str]] = []
    for block in path.read_text(encoding="utf-8").split("\n\n"):
        fields: dict[str, str] = {}
        for line in block.splitlines():
            if len(line) > 2 and line[1] == ":":
                fields[line[0]] = line[2:]
        if not fields.get("P"):
            continue
        required = ("P", "V", "U", "L", "o", "c")
        missing = [key for key in required if not fields.get(key)]
        if missing:
            raise RuntimeError(f"installed package {fields.get('P', '<unknown>')} lacks {missing}")
        packages.append({"package": fields["P"], "version": fields["V"],
                         "upstream": fields["U"], "license": fields["L"],
                         "origin": fields["o"], "commit": fields["c"]})
    if not packages:
        raise RuntimeError(f"no package records in {path}")
    return sorted(packages, key=lambda package: package["package"])

def safe_extract_tar(archive: tarfile.TarFile, destination: Path) -> None:
    root = destination.resolve()
    for member in archive.getmembers():
        target = (destination / member.name).resolve()
        if target != root and root not in target.parents:
            raise RuntimeError(f"unsafe path in Alpine source archive: {member.name}")
        archive.extract(member, destination, filter="data")


def fetch_aports_source(origin: str, commit: str, destination: Path) -> Path:
    destination.mkdir(parents=True, exist_ok=True)
    url = APORTS_ARCHIVE.format(origin=origin, commit=commit)
    with urllib.request.urlopen(url, timeout=120) as response:
        archive_bytes = response.read()
    with tarfile.open(fileobj=io.BytesIO(archive_bytes), mode="r:gz") as archive:
        safe_extract_tar(archive, destination)
    apkbuilds = sorted(destination.glob("**/APKBUILD"))
    if len(apkbuilds) != 1:
        raise RuntimeError(f"expected one APKBUILD for {origin}@{commit}, found {len(apkbuilds)}")
    return apkbuilds[0].parent


def apkbuild_version(apkbuild: Path) -> str:
    values: dict[str, str] = {}
    for line in apkbuild.read_text(encoding="utf-8").splitlines():
        for key in ("pkgver", "pkgrel"):
            prefix = f"{key}="
            if line.startswith(prefix):
                values[key] = line.removeprefix(prefix).strip().strip("\"")
    if "pkgver" not in values or "pkgrel" not in values:
        raise RuntimeError(f"{apkbuild} has no literal pkgver/pkgrel")
    return f"{values['pkgver']}-r{values['pkgrel']}"


def verify_origin_versions(source_dir: Path, packages: list[dict[str, str]]) -> None:
    actual = apkbuild_version(source_dir / "APKBUILD")
    mismatches = [
        f"{package['package']}={package['version']}"
        for package in packages
        if package["version"] != actual
    ]
    if mismatches:
        raise RuntimeError(
            f"{source_dir}/APKBUILD is {actual}, but rows disagree: {', '.join(mismatches)}"
        )


def copied_files(source_dir: Path, destination: Path) -> None:
    for source in source_dir.rglob("*"):
        if not source.is_file():
            continue
        relative = source.relative_to(source_dir)
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)


def archive_notices(archive_path: Path, destination: Path) -> None:
    try:
        with tarfile.open(archive_path, mode="r:*") as archive:
            for member in archive.getmembers():
                if not member.isfile() or not is_notice(Path(member.name)):
                    continue
                stream = archive.extractfile(member)
                if stream is None:
                    continue
                target = safe_target(destination, member.name)
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(stream.read())
        return
    except (tarfile.ReadError, EOFError):
        pass
    try:
        with zipfile.ZipFile(archive_path) as archive:
            for member in archive.infolist():
                if member.is_dir() or not is_notice(Path(member.filename)):
                    continue
                target = safe_target(destination, member.filename)
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(archive.read(member))
    except zipfile.BadZipFile:
        return


def source_bundle(packages: list[dict[str, str]], installed: Path, output: Path) -> None:
    output.mkdir(parents=True)
    (output / "inventory").mkdir()
    (output / "aports").mkdir()
    (output / "sources").mkdir()
    (output / "notices").mkdir()
    shutil.copy2(installed, output / "inventory" / "installed")
    manifest = output / "inventory" / "packages.json"
    manifest.write_text(json.dumps(packages, indent=2) + "\n", encoding="utf-8")
    origins: dict[tuple[str, str], list[dict[str, str]]] = {}
    for package in packages:
        origins.setdefault((package["origin"], package["commit"]), []).append(package)
    with tempfile.TemporaryDirectory(prefix="alpine-aports-") as temporary:
        temporary_root = Path(temporary)
        for (origin, commit), origin_packages in sorted(origins.items()):
            key = f"{origin}@{commit}"
            checkout = fetch_aports_source(origin, commit, temporary_root / key)
            verify_origin_versions(checkout, origin_packages)
            aports_destination = output / "aports" / key
            copied_files(checkout, aports_destination)
            source_destination = output / "sources" / key
            source_destination.mkdir()
            subprocess.run(
                ["abuild", "-F", "-s", str(source_destination), "fetch", "verify"],
                cwd=aports_destination,
                check=True,
            )
            for source in sorted(source_destination.iterdir()):
                if source.is_file():
                    archive_notices(source, output / "notices" / key / source.name)
            for source in sorted(aports_destination.rglob("*")):
                if source.is_file() and is_notice(source):
                    relative = source.relative_to(aports_destination)
                    target = output / "notices" / key / "aports" / relative
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copy2(source, target)
def runtime_artifact(base_artifact: Path, packages: list[dict[str, str]], bundle: Path, artifact: Path) -> None:
    lines = [
        "## Alpine runtime package closure and source notices",
        "",
        "Generated from the final image's /lib/apk/db/installed before build-only tooling.",
        "Each package source was fetched from the exact Alpine aports commit and verified by abuild.",
        "The complete corresponding source bundle is in /usr/share/doc/vestibule/alpine-runtime-sources.",
        "",
    ]
    for package in packages:
        source_path = (
            "https://gitlab.alpinelinux.org/alpine/aports/-/tree/"
            f"{package['commit']}/main/{package['origin']}"
        )
        lines.extend([
            f"Package: {package['package']}",
            f"Installed version: {package['version']}",
            f"Upstream URL: {package['upstream']}",
            f"Declared license: {package['license']}",
            f"Alpine aports origin: {package['origin']}",
            f"Alpine aports commit: {package['commit']}",
            f"Alpine source path: {source_path}",
            "",
        ])
    lines.extend([
        "## Alpine extracted source notices",
        "",
        "The following bytes were extracted from the abuild-verified source archives or aports tree.",
        "",
    ])
    with artifact.open("wb") as destination:
        destination.write(base_artifact.read_bytes())
        destination.write(("\n\n" + "\n".join(lines) + "\n").encode("utf-8"))
        for notice in sorted((bundle / "notices").rglob("*")):
            if not notice.is_file():
                continue
            relative = notice.relative_to(bundle)
            destination.write(
                f"Notice source: /usr/share/doc/vestibule/alpine-runtime-sources/{relative}\n"
                "----- BEGIN LICENSE -----\n".encode("utf-8")
            )
            destination.write(notice.read_bytes())
            destination.write(b"\n----- END LICENSE -----\n\n")



def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--installed", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--base-artifact", type=Path, required=True)
    parser.add_argument("--artifact", type=Path, required=True)
    args = parser.parse_args()
    packages = installed_packages(args.installed)
    source_bundle(packages, args.installed, args.output)
    runtime_artifact(args.base_artifact, packages, args.output, args.artifact)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
