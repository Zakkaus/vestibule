#!/usr/bin/env python3
"""Hold the console's three locale catalogues to each other, and to the code.

Nothing checked them. Agents edit all three every time a screen is added, the
counts happen to match, and four failures were possible and invisible:

  - a key added to one catalogue and forgotten in another, which renders the key
    itself on screen in that language and nowhere else;
  - a placeholder renamed or dropped in a translation, which prints {{count}} to
    a reader or silently deletes a number from the sentence. Key parity cannot
    see this: the key is present and the value looks like a sentence;
  - a mistyped key in a component. i18next has no typed key union here, so
    t("hom.title") compiles and renders "hom.title".
  - an empty or untranslated value, which leaves a blank control or changes the
    language in the middle of an otherwise localized screen.

All four were at zero when this was written, which is the cheapest moment to
freeze them.

Runtime keys are held too. The check validates every dotted key value declared in
source and enumerates each computed-key family from its source domain. A new
computed shape fails until the checker can enumerate it, rather than becoming an
unchecked path.

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
KEY_LITERAL = re.compile(
    r"""(?<![A-Za-z0-9_$])(["'])([A-Za-z][A-Za-z0-9_-]*(?:\.[A-Za-z][A-Za-z0-9_-]*)+)\1"""
)
KEY_TEMPLATE = re.compile(r"`([A-Za-z][A-Za-z0-9_.]*)\$\{([^}]*)\}([A-Za-z0-9_.]*)`")
QUOTED_VALUE = re.compile(r"""["']([A-Za-z][A-Za-z0-9_-]*)["']""")
HAN = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]")
CHINESE_CATALOGUES = {"zh-CN", "zh-TW"}
NON_TRANSLATION_DOTTED_LITERALS = {"github.com"}
LANGUAGE_NEUTRAL_CHINESE_VALUES = {
    "locale.en": "English",
    "groups.applicants.withUsername": "@{{username}} · {{id}}",
    "groups.applicants.idOnly": "ID {{id}}",
    "diagnostics.values.percent": "{{value}}%",
    "messages.rules.identifier": "ID：{{id}}",
}
# i18next resolves a plural key to one of these suffixes at call time, so a
# component asks for the bare key and the catalogue never holds it.
PLURAL_SUFFIXES = ("_zero", "_one", "_two", "_few", "_many", "_other")

# What Intl.PluralRules answers for each locale, read from the runtime rather
# than from memory: node -e 'new Intl.PluralRules(l).resolvedOptions()'. A
# category the language does not have can never be selected, so a translator
# maintains a string nobody will ever see; a category it does have and the
# catalogue lacks renders the key. The table is here rather than derived at
# check time because the job that runs this has no node.
#
# Adding a locale means adding its row. An unlisted locale fails.
PLURAL_CATEGORIES = {
    "en": {"one", "other"},
    "zh-CN": {"other"},
    "zh-TW": {"other"},
    "ja": {"other"},
    "ru": {"one", "few", "many", "other"},
}


def plural_stem(key: str) -> str:
    for suffix in PLURAL_SUFFIXES:
        if key.endswith(suffix):
            return key[: -len(suffix)]
    return key

def key_exists(catalogue: dict, key: str) -> bool:
    return key in catalogue or any(key + suffix in catalogue for suffix in PLURAL_SUFFIXES)


def quoted_values(source: str) -> set[str]:
    return set(QUOTED_VALUE.findall(source))


def declared_array_values(sources: dict[Path, str], path: Path, name: str) -> set[str] | None:
    source = sources.get(path)
    if source is None:
        return None
    match = re.search(
        r"\b(?:export\s+)?const\s+" + re.escape(name) + r"\s*=\s*\[(.*?)\]\s*as const",
        source,
        re.DOTALL,
    )
    return quoted_values(match.group(1)) if match else None


def declared_type_values(sources: dict[Path, str], path: Path, name: str) -> set[str] | None:
    source = sources.get(path)
    if source is None:
        return None
    match = re.search(r"\btype\s+" + re.escape(name) + r"\s*=\s*(.*?);", source, re.DOTALL)
    return quoted_values(match.group(1)) if match else None


def runtime_template_values(
    sources: dict[Path, str], prefix: str, expression: str, suffix: str
) -> set[str] | None:
    expression = expression.strip()
    if (prefix, expression, suffix) == ("challenge.state.", "record.result.state", ""):
        values = declared_array_values(sources, Path("lib/challenge.ts"), "challengeStates")
    elif (prefix, expression, suffix) == ("home.attention.tones.", "item.tone", ""):
        source = sources.get(Path("features/home/HomeScreen.tsx"))
        values = set(re.findall(r'\btone:\s*"([A-Za-z][A-Za-z0-9_-]*)"', source)) if source else None
    elif (prefix, expression, suffix) == ("stats.filters.errors.", "error", ""):
        values = declared_type_values(sources, Path("features/stats/StatsScreen.tsx"), "QueryError")
    elif (prefix, expression, suffix) == ("stats.summary.", "label", ""):
        source = sources.get(Path("features/stats/StatsViews.tsx"))
        match = re.search(r"\bconst\s+entries\s*=\s*\[(.*?)\]\s*as const;", source, re.DOTALL) if source else None
        values = set(re.findall(r'\[\s*"([A-Za-z][A-Za-z0-9_-]*)"\s*,', match.group(1))) if match else None
    elif prefix == "version.errors." and expression == "scope":
        source = sources.get(Path("features/version/VersionScreen.tsx"))
        match = re.search(r'\bscope:\s*((?:"[A-Za-z][A-Za-z0-9_-]*"\s*\|\s*)+"[A-Za-z][A-Za-z0-9_-]*")', source) if source else None
        values = quoted_values(match.group(1)) if match else None
    else:
        return None
    return {prefix + value + suffix for value in values} if values else None

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

def check_values_are_nonempty_and_localized(name: str, catalogue: dict) -> None:
    if name != "en" and name not in CHINESE_CATALOGUES:
        failures.append("%s.json has no value-language rule, so untranslated "
                        "entries would be invisible to this check" % name)
        return
    for key, value in sorted(catalogue.items()):
        if not isinstance(value, str):
            failures.append("%s.json %s is not a string, so it cannot label the "
                            "interface" % (name, key))
            continue
        if not value.strip():
            failures.append("%s.json %s is empty; operators would see a blank "
                            "label or message" % (name, key))
            continue
        match = HAN.search(value)
        if name == "en" and match:
            failures.append("%s.json %s contains Chinese character %r; English "
                            "operators would receive untranslated text"
                            % (name, key, match.group()))
        if name in CHINESE_CATALOGUES and not match:
            if LANGUAGE_NEUTRAL_CHINESE_VALUES.get(key) == value:
                continue
            failures.append("%s.json %s contains no Chinese text and is not a "
                            "language-neutral UI value; Chinese operators would "
                            "receive untranslated text" % (name, key))


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
        check_values_are_nonempty_and_localized(path.stem, catalogues[path.stem])
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
        # Compare what a key means, not how it is spelled. Russian needs four
        # plural forms where English needs two, so comparing leaf keys would
        # have made a correct Russian catalogue impossible to add — the first
        # thing found when preparing for it.
        source_stems = {plural_stem(key) for key in source}
        catalogue_stems = {plural_stem(key) for key in catalogue}
        for stem in sorted(source_stems - catalogue_stems):
            failures.append("%s.json is missing %s, which %s.json has"
                            % (name, stem, source_name))
        for stem in sorted(catalogue_stems - source_stems):
            failures.append("%s.json has %s, which %s.json does not"
                            % (name, stem, source_name))
        for key in sorted(set(source) & set(catalogue)):
            wanted = set(PLACEHOLDER.findall(str(source[key])))
            got = set(PLACEHOLDER.findall(str(catalogue[key])))
            if wanted != got:
                failures.append("%s.json %s interpolates %s where %s.json "
                                "interpolates %s" % (name, key, sorted(got) or "nothing",
                                                     source_name, sorted(wanted) or "nothing"))

    # Each locale carries exactly the plural categories its language has.
    for name, catalogue in sorted(catalogues.items()):
        expected = PLURAL_CATEGORIES.get(name)
        if expected is None:
            failures.append("%s.json is a locale this check does not know the "
                            "plural categories for — add its row from "
                            "Intl.PluralRules rather than guessing" % name)
            continue
        groups = {}
        for key in catalogue:
            stem = plural_stem(key)
            if stem == key:
                continue
            groups.setdefault(stem, set()).add(key[len(stem) + 1:])
        for stem, found in sorted(groups.items()):
            for extra in sorted(found - expected):
                failures.append("%s.json defines %s_%s and %s never selects that "
                                "category, so nobody can read it"
                                % (name, stem, extra, name))
            for missing in sorted(expected - found):
                failures.append("%s.json has no %s_%s and %s selects that category"
                                % (name, stem, missing, name))

    literal = 0
    dynamic = 0
    literal_keys = set()
    runtime_keys: dict[str, tuple[Path, int]] = {}
    source_files: list[tuple[Path, str]] = []
    source_texts: dict[Path, str] = {}
    for path in sorted(source_dir.rglob("*")):
        if path.suffix not in (".ts", ".tsx") or locales_dir in path.parents:
            continue
        text = path.read_text(encoding="utf-8")
        source_files.append((path, text))
        source_texts[path.relative_to(source_dir)] = text
    for path, text in source_files:
        dynamic += len(DYNAMIC_KEY.findall(text))
        for match in LITERAL_KEY.finditer(text):
            literal += 1
            key = match.group(1)
            literal_keys.add(key)
            if key_exists(source, key):
                continue
            line = text[: match.start()].count("\n") + 1
            failures.append("%s:%d asks for %s and %s.json does not define it"
                            % (path.relative_to(ROOT), line, key, source_name))
        for match in KEY_LITERAL.finditer(text):
            key = match.group(2)
            if key in NON_TRANSLATION_DOTTED_LITERALS:
                continue
            line = text[: match.start()].count("\n") + 1
            runtime_keys.setdefault(key, (path, line))
        for match in KEY_TEMPLATE.finditer(text):
            prefix, expression, suffix = match.groups()
            line = text[: match.start()].count("\n") + 1
            values = runtime_template_values(source_texts, prefix, expression, suffix)
            if values is None:
                failures.append("%s:%d builds %s${%s}%s but its variants are not "
                                "enumerated; an operator could see an unchecked raw key"
                                % (path.relative_to(ROOT), line, prefix, expression, suffix))
                continue
            for key in values:
                runtime_keys.setdefault(key, (path, line))

    if literal == 0:
        failures.append("no literal translation key was found in %s — has the call "
                        "shape changed?" % source_dir)

    for key, (path, line) in sorted(runtime_keys.items()):
        if key in literal_keys or key_exists(source, key):
            continue
        failures.append("%s:%d declares runtime key %s but %s.json does not define it; "
                        "operators would see the raw key"
                        % (path.relative_to(ROOT), line, key, source_name))

    # The other direction. The check above asks whether every key the code names
    # exists; nothing asked whether every key defined is named by anything. A
    # catalogue entry nobody reaches is three strings a translator maintains for
    # a screen that stopped using them, and it was at zero when this was added,
    # which is the cheap moment to hold it there.
    #
    # Runtime templates still count through their prefix here. This direction
    # catches a whole entry going cold; the keyed checks above prove which
    # concrete values each template can request.
    sources = "".join(text for _, text in source_files)
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

    print("check-locale-catalogues: passed; %d catalogues have non-empty localized "
          "values, agree on %d keys and their placeholders, every key is reached, "
          "%d literal calls and %d runtime calls resolve through %d declared key values"
          % (len(catalogues), len(source), literal, dynamic, len(runtime_keys)))
    return 0


if __name__ == "__main__":
    sys.exit(main())
