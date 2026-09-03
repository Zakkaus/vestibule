#!/usr/bin/env python3
"""Every CSS class use has a definition, including utility-prefixed classes.

The vendored coverage check deliberately permits unused sr-, u-, is- and has-
definitions. Its shared ignore condition also permits undefined uses with those prefixes,
so an element can name a utility that has no effect. This repository-owned wrapper keeps
the definition-side exemption and closes the use-side hole without editing the vendored
copy.

Usage: check-css-coverage.py <file.css|file.html> ...
"""
import contextlib
import importlib.util
import io
import sys
from pathlib import Path
from types import ModuleType

ROOT = Path(__file__).resolve().parent.parent
VENDORED = ROOT / "scripts" / "design-checks" / "css-coverage.py"


def load_vendored_checker() -> ModuleType:
    spec = importlib.util.spec_from_file_location("vendored_css_coverage", VENDORED)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {VENDORED}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def prefixed_class_uses_have_definitions(
    module: ModuleType, argv: list[str]
) -> tuple[list[str], int]:
    defined = set()
    used = {}
    for arg in argv:
        css, html = module.read(Path(arg))
        defined.update(module.DEF.findall(css))
        for match in module.USE.finditer(html):
            value = match.group("d") if match.group("d") is not None else match.group("s")
            for name in value.split():
                if name.startswith(module.IGNORE_PREFIXES):
                    used.setdefault(name, arg)
        for groups in module.SCRIPT_USE.findall(html):
            for group in groups:
                for name in module.SPLIT.split(group):
                    if module.IDENT.fullmatch(name) and name.startswith(module.IGNORE_PREFIXES):
                        used.setdefault(name, arg)
    failures = [
        f".{name} is used in {used[name]} but has no CSS definition; the named utility has no effect"
        for name in sorted(used)
        if name not in defined
    ]
    return failures, len(used)


def main(argv: list[str]) -> int:
    if not argv:
        print(__doc__, file=sys.stderr)
        return 2
    try:
        module = load_vendored_checker()
    except (OSError, RuntimeError) as error:
        print(f"FAIL check-css-coverage: {error}")
        return 2

    stdout = io.StringIO()
    stderr = io.StringIO()
    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
        status = module.main(argv)
    if status:
        print(
            "FAIL check-css-coverage: stylesheet hooks and their demonstrations disagree; "
            "a console element can render unstyled or dead CSS can survive"
        )
        print(stdout.getvalue(), end="")
        print(stderr.getvalue(), end="", file=sys.stderr)
        return status

    failures, prefixed = prefixed_class_uses_have_definitions(module, argv)
    if failures:
        print("FAIL check-css-coverage:")
        for failure in failures:
            print("  " + failure)
        return 1

    print(stdout.getvalue(), end="")

    print(f"check-css-coverage: passed; {prefixed} utility-prefixed class names have definitions")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
