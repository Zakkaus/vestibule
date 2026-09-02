#!/usr/bin/env python3
"""Hold the console's three locale catalogues to each other, and to the code.

Nothing checked them. Agents edit all three every time a screen is added, the
counts happen to match, and three failures were possible and invisible:

  - a key added to one catalogue and forgotten in another, which renders the key
    itself on screen in that language and nowhere else;
  - a placeholder renamed or dropped in a translation, which prints {{count}} to
    a reader or silently deletes a number from the sentence. Key parity cannot
    see this: the key is present and the value looks like a sentence;
  - a mistyped key in a component. i18next has no typed key union here, so
    t("hom.title") compiles and renders "hom.title".

All three were at zero when this was written, which is the cheapest moment to
freeze them.

What it cannot see, stated rather than hidden: keys built at runtime. 149 call
sites pass something other than a string literal — an indexed lookup or a
template. Those resolve at runtime and this check skips them; the count is
printed so the gap stays visible.

Usage: check-locale-catalogues.py [locales-dir] [source-dir]
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLACEHOLDER = re.compile(r"\{\{\s*([A-Za-z0-9_]+)\s*\}\}")
LITERAL_KEY = re.compile(r"(?<![A-Za-z0-9_$])t\(\s*\"([A-Za-z0-9_.]+)\"")
DYNAMIC_KEY = re.compile(r"(?<![A-Za-z0-9_$])t\(\s*(?![\"'])")
# i18next resolves a plural key to one of these suffixes at call time, so a
# component asks for the bare key and the catalogue never holds it.
PLURAL_SUFFIXES = ("_zero", "_one", "_two", "_few", "_many", "_other")

failures: list[str] = []


def flatten(value: dict, prefix: str = "") -> dict:
    flat = {}
    for key, item in value.items():
        path = f"{prefix}.{key}" if prefix else key
        if isinstance(item, dict):
            flat.update(flatten(item, path))
        else:
            flat[path] = item
    return flat


def main() -> int:
    locales_dir = ROOT / (sys.argv[1] if len(sys.argv) > 1 else "web/src/i18n/locales")
    source_dir = ROOT / (sys.argv[2] if len(sys.argv) > 2 else "web/src")
    if not locales_dir.is_dir():
        print("FAIL check-locale-catalogues: %s does not exist, so nothing was read"
              % locales_dir)
        return 1

    catalogues = {}
    for path in sorted(locales_dir.glob("*.json")):
        catalogues[path.stem] = flatten(json.loads(path.read_text(encoding="utf-8")))
    if len(catalogues) < 2:
        print("FAIL check-locale-catalogues: found %d catalogue(s) in %s; there is "
              "nothing to compare" % (len(catalogues), locales_dir))
        return 1

    source_name = "en" if "en" in catalogues else sorted(catalogues)[0]
    source = catalogues[source_name]
    if not source:
        print("FAIL check-locale-catalogues: %s.json holds no key, so an empty run "
              "would report success" % source_name)
        return 1

    for name, catalogue in sorted(catalogues.items()):
        if name == source_name:
            continue
        for key in sorted(set(source) - set(catalogue)):
            failures.append("%s.json is missing %s, which %s.json has"
                            % (name, key, source_name))
        for key in sorted(set(catalogue) - set(source)):
            failures.append("%s.json has %s, which %s.json does not"
                            % (name, key, source_name))
        for key in sorted(set(source) & set(catalogue)):
            wanted = set(PLACEHOLDER.findall(str(source[key])))
            got = set(PLACEHOLDER.findall(str(catalogue[key])))
            if wanted != got:
                failures.append("%s.json %s interpolates %s where %s.json "
                                "interpolates %s" % (name, key, sorted(got) or "nothing",
                                                     source_name, sorted(wanted) or "nothing"))

    literal = 0
    dynamic = 0
    for path in sorted(source_dir.rglob("*")):
        if path.suffix not in (".ts", ".tsx") or locales_dir in path.parents:
            continue
        text = path.read_text(encoding="utf-8")
        dynamic += len(DYNAMIC_KEY.findall(text))
        for match in LITERAL_KEY.finditer(text):
            literal += 1
            key = match.group(1)
            if key in source:
                continue
            if any(key + suffix in source for suffix in PLURAL_SUFFIXES):
                continue
            line = text[: match.start()].count("\n") + 1
            failures.append("%s:%d asks for %s and %s.json does not define it"
                            % (path.relative_to(ROOT), line, key, source_name))

    if literal == 0:
        failures.append("no literal translation key was found in %s — has the call "
                        "shape changed?" % source_dir)

    # The other direction. The check above asks whether every key the code names
    # exists; nothing asked whether every key defined is named by anything. A
    # catalogue entry nobody reaches is three strings a translator maintains for
    # a screen that stopped using them, and it was at zero when this was added,
    # which is the cheap moment to hold it there.
    #
    # A key built at runtime is reached through a prefix, so a key counts as used
    # when any proper prefix of it appears inside a template literal. That is
    # deliberately generous: this check exists to catch a whole entry going cold,
    # not to prove each leaf is live.
    sources = ""
    for path in sorted(source_dir.rglob("*")):
        if path.suffix in (".ts", ".tsx") and locales_dir not in path.parents:
            sources += path.read_text(encoding="utf-8")
    for key in sorted(source):
        stem = key
        for suffix in PLURAL_SUFFIXES:
            if key.endswith(suffix):
                stem = key[: -len(suffix)]
                break
        if any(quote + stem + quote in sources for quote in ("\"", "'", "`")):
            continue
        parts = stem.split(".")
        if any("`" + ".".join(parts[:count]) + "." in sources
               for count in range(2, len(parts))):
            continue
        failures.append("%s.json defines %s and nothing in %s asks for it — use it "
                        "or remove it from all three catalogues"
                        % (source_name, key, source_dir.name))

    if failures:
        print("FAIL check-locale-catalogues: the catalogues and the code disagree")
        for failure in failures:
            print("  " + failure)
        return 1

    print("check-locale-catalogues: passed; %d catalogues agree on %d keys and their "
          "placeholders, every key is reached and %d literal ones resolve, %d runtime "
          "keys not checked" % (len(catalogues), len(source), literal, dynamic))
    return 0


if __name__ == "__main__":
    sys.exit(main())
