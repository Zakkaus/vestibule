#!/usr/bin/env python3
"""Validate the five console and bot locale catalogues.

Console catalogues must preserve logical keys, complete plural categories, and
interpolation names. Literal and computed translation keys must resolve, and
every catalogue key must remain reachable from the source.

The default invocation also checks backend files, key and array structure, and
indexed printf placeholders. Explicit fixture paths isolate frontend checks.

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
JAPANESE = re.compile(r"[\u3040-\u30ff\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff]")
CYRILLIC = re.compile(r"[\u0400-\u04ff]")
CHINESE_CATALOGUES = {"zh-CN", "zh-TW"}
SUPPORTED_CATALOGUES = ("en", "zh-CN", "zh-TW", "ja", "ru")

NON_TRANSLATION_DOTTED_LITERALS = {"github.com"}
LANGUAGE_NEUTRAL_CHINESE_VALUES = {
    "locale.en": "English",
    "groups.applicants.withUsername": "@{{username}} · {{id}}",
    "groups.applicants.idOnly": "ID {{id}}",
    "diagnostics.values.percent": "{{value}}%",
    "messages.rules.identifier": "ID：{{id}}",
}
LANGUAGE_NEUTRAL_VALUES = {
    "ja": dict(LANGUAGE_NEUTRAL_CHINESE_VALUES),
    "ru": {**LANGUAGE_NEUTRAL_CHINESE_VALUES, "messages.rules.identifier": "ID: {{id}}"},
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
# These frontend calls use i18next's count-aware lookup but their logical stems
# are not recoverable from catalogue suffixes alone. Keep this registry in sync
# with the production families below; fixture invocations only validate entries
# they actually contain.
COUNT_SENSITIVE_KEYS = (
    "bypass.save.unsaved",
    "feeds.values.seconds",
    "groups.managedCount",
    "home.attention.queue.description",
    "home.values.questions",
    "moderation.save.unsaved",
    "questions.fallback.count",
    "questions.fallback.inheritedCount",
    "questions.questionBank.count",
)


def registered_stems_present(catalogues: dict[str, dict]) -> set[str]:
    return {
        stem
        for stem in COUNT_SENSITIVE_KEYS
        if any(key_exists(catalogue, stem) for catalogue in catalogues.values())
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
        source = sources.get(Path("features/home/HomeDashboard.tsx"))
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
    elif (prefix, expression, suffix) == ("groups.permissions.", "key", ""):
        values = declared_array_values(sources, Path("app/session.ts"), "consoleChatPermissionKeys")
    elif (prefix, expression, suffix) == ("groups.administrators.", "administrator.status", ""):
        values = declared_type_values(sources, Path("app/session.ts"), "ConsoleChatAdministratorStatus")
    elif prefix == "owner.fields." and expression in {"field", "violation.field"} and suffix == "":
        values = declared_array_values(sources, Path("features/owner/api.ts"), "ownerLimitFields")
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

BACKEND_CATALOGUES = ("zh", "zh-Hant", "en", "ja", "ru")
BACKEND_FILES = (
    "bot.json",
    "feed.json",
    "lookup_content.json",
    "lookup_distros.json",
    "lookup_packages.json",
    "moderate.json",
    "panel.json",
    "verification.json",
)
PRINTF = re.compile(r"%\[[0-9]+\][a-z]")


def backend_shape(value):
    if isinstance(value, dict):
        return ("object", tuple((key, backend_shape(item)) for key, item in sorted(value.items())))
    if isinstance(value, list):
        return ("array", tuple(sorted({backend_shape(item) for item in value})))
    return type(value).__name__


def backend_strings(value, prefix=""):
    if isinstance(value, dict):
        for key, item in value.items():
            path = f"{prefix}.{key}" if prefix else key
            yield from backend_strings(item, path)
    elif isinstance(value, list):
        for index, item in enumerate(value):
            yield from backend_strings(item, f"{prefix}[{index}]")
    elif isinstance(value, str):
        yield prefix, value


def check_backend_catalogues() -> None:
    root = ROOT / "internal/i18n/locales"
    if not root.is_dir():
        failures.append("backend locale directory is missing")
        return
    loaded = {}
    for name in BACKEND_CATALOGUES:
        directory = root / name
        if not directory.is_dir():
            failures.append("internal/i18n/locales/%s is missing" % name)
            continue
        actual = {path.name for path in directory.glob("*.json")}
        extra = sorted(actual - set(BACKEND_FILES))
        for filename in extra:
            failures.append("%s/%s is an unregistered backend catalogue file" % (name, filename))
        for filename in BACKEND_FILES:
            path = directory / filename
            if not path.is_file():
                failures.append("%s/%s is missing" % (name, filename))
                continue
            try:
                catalogue = json.loads(path.read_text(encoding="utf-8"))
            except (OSError, json.JSONDecodeError) as error:
                failures.append("%s/%s is not valid JSON: %s" % (name, filename, error))
                continue
            if not isinstance(catalogue, dict):
                failures.append("%s/%s must contain a JSON object" % (name, filename))
                continue
            loaded[(name, filename)] = catalogue
    for filename in BACKEND_FILES:
        source = loaded.get(("en", filename))
        if source is None:
            continue
        expected_shape = backend_shape(source)
        expected = dict(backend_strings(source))
        for name in BACKEND_CATALOGUES:
            catalogue = loaded.get((name, filename))
            if catalogue is None:
                continue
            if backend_shape(catalogue) != expected_shape:
                failures.append("%s/%s does not preserve the English key and array structure"
                                % (name, filename))
                continue
            for key, value in backend_strings(catalogue):
                source_value = expected.get(key)
                if source_value is None:
                    continue
                if set(PRINTF.findall(value)) != set(PRINTF.findall(source_value)):
                    failures.append("%s/%s %s changes printf placeholders"
                                    % (name, filename, key))


def check_values_are_nonempty_and_localized(name: str, catalogue: dict) -> None:
    if name == "en":
        script_name = "English"
        script = HAN
    elif name in CHINESE_CATALOGUES:
        script_name = "Chinese"
        script = HAN
    elif name == "ja":
        script_name = "Japanese"
        script = JAPANESE
    elif name == "ru":
        script_name = "Russian"
        script = CYRILLIC
    else:
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
        if name == "en":
            if HAN.search(value):
                failures.append("%s.json %s contains Chinese character; English "
                                "operators would receive untranslated text"
                                % (name, key))
            continue
        if script.search(value):
            continue
        neutral_values = LANGUAGE_NEUTRAL_CHINESE_VALUES if name in CHINESE_CATALOGUES else LANGUAGE_NEUTRAL_VALUES.get(name, {})
        if neutral_values.get(key) == value:
            continue
        failures.append("%s.json %s contains no %s text and is not a "
                        "language-neutral UI value; operators would receive "
                        "untranslated text" % (name, key, script_name))

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
    missing = [name for name in SUPPORTED_CATALOGUES if name not in catalogues]
    for name in missing:
        failures.append("%s.json is a supported catalogue but is missing" % name)
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

    source_values_by_stem = {}
    for source_key, source_value in source.items():
        source_values_by_stem.setdefault(plural_stem(source_key), source_value)
    source_stems = set(source_values_by_stem)

    for name, catalogue in sorted(catalogues.items()):
        if name == source_name:
            continue
        # Compare what a key means, not how it is spelled. Russian needs four
        # plural forms where English needs two, so comparing leaf keys would
        # have made a correct Russian catalogue impossible to add — the first
        # thing found when preparing for it.
        catalogue_stems = {plural_stem(key) for key in catalogue}
        for stem in sorted(source_stems - catalogue_stems):
            failures.append("%s.json is missing %s, which %s.json has"
                            % (name, stem, source_name))
        for stem in sorted(catalogue_stems - source_stems):
            failures.append("%s.json has %s, which %s.json does not"
                            % (name, stem, source_name))
        for key, value in sorted(catalogue.items()):
            stem = plural_stem(key)
            source_value = source.get(key)
            if source_value is None:
                source_value = source_values_by_stem.get(stem)
                if source_value is None:
                    continue
            wanted = set(PLACEHOLDER.findall(str(source_value)))
            got = set(PLACEHOLDER.findall(str(value)))
            if wanted != got:
                failures.append("%s.json %s interpolates %s where %s.json "
                                "interpolates %s" % (name, key, sorted(got) or "nothing",
                                                     source_name, sorted(wanted) or "nothing"))

    # A registered stem must stay plural even if every catalogue currently
    # contains only its bare key; suffix discovery alone cannot see that
    # coordinated collapse.
    plural_stems = {
        plural_stem(key)
        for catalogue in catalogues.values()
        for key in catalogue
        if plural_stem(key) != key
    } | registered_stems_present(catalogues)
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
        for stem in sorted(plural_stems):
            found = groups.get(stem, set())
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
                        "or remove it from all five catalogues"
                        % (source_name, key, source_dir.name))

    if len(sys.argv) == 1:
        production_stems = {
            plural_stem(key)
            for catalogue in catalogues.values()
            for key in catalogue
            if plural_stem(key) != key
        }
        registered = set(COUNT_SENSITIVE_KEYS)
        for stem in sorted(registered - production_stems):
            failures.append("count-sensitive registry lists %s, but production "
                            "has no plural family for it" % stem)
        for stem in sorted(production_stems - registered):
            failures.append("production plural family %s is not in the "
                            "count-sensitive registry" % stem)
        check_backend_catalogues()

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
