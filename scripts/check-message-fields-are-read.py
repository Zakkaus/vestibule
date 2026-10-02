#!/usr/bin/env python3
"""Every message field has a reader, directly or through a called i18n helper.

A declared field costs three translations and reads as a promise that the software
says this somewhere. Five were left behind by the rewrite -- the previous generation's
inline settings panel had an edit label, a stale-index notice, a reply-to-the-prompt
notice and a control-group refusal, and the feed had its own control-group refusal.
None of them is reachable: no call site names them, and the panel they belonged to is
not the panel this generation ships. Fifteen translated strings existed for text a
reader can never see.

The reader check follows catalogue-shaped selectors rather than searching for a bare
identifier. This keeps an unrelated result or status struct with a field such as
``Reason`` from counting as a translation reader.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CATALOGUE = ROOT / "internal" / "i18n"
FIELD = re.compile(r"^\t([A-Z][A-Za-z0-9]*)\s+(?:Text|Format|StringList)\b", re.M)
CATALOGUE_TYPE = re.compile(
    r"\b([A-Za-z_][A-Za-z0-9_]*)\s+\*?\s*i18n\.(?:[A-Za-z_][A-Za-z0-9_]*Catalog|Catalog)\b")
CATALOGUE_ASSIGNMENT = re.compile(
    r"\b([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|=)\s*&?\s*"
    r"([A-Za-z_][A-Za-z0-9_]*(?:\s*\.\s*[A-Za-z_][A-Za-z0-9_]*)+)")

# This field is genuinely unread. It remains visible rather than silently counted
# through the unrelated setup-page field while a product change decides whether
# the panel should render it or the catalogue should drop it.
UNREAD_EXEMPTIONS = {
    ("Language", ("internal/i18n/panel.go",)): "the panel language label has no production reader",
}



def catalogue_roots(text: str, catalogue_fields=()) -> set:
    """Find identifiers and paths that are statically catalogue-shaped."""
    normalized = re.sub(r"\(\s*\*\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*\.\s*\w+)*)\s*\)",
                        r"\1", text)
    roots = {"i18n.Messages"}
    roots.update(match.group(1) for match in CATALOGUE_TYPE.finditer(normalized))
    # Service structs keep their catalogue in fields whose types are *i18n.Catalog.
    # Carry those field names across files so a use such as v.messages.Reason is
    # recognised even though the struct declaration lives in another file.
    for field in catalogue_fields:
        roots.update(match.group(0) for match in re.finditer(
            r"\b[A-Za-z_][A-Za-z0-9_]*\.%s\b" % re.escape(field), normalized))

    changed = True
    while changed:
        changed = False
        for match in CATALOGUE_ASSIGNMENT.finditer(normalized):
            name, source = match.group(1), re.sub(r"\s+", "", match.group(2))
            if any(source == root or source.startswith(root + ".") for root in roots):
                if name not in roots:
                    roots.add(name)
                    changed = True
    return roots


def catalogue_fields_read(text: str, roots: set) -> set:
    """Return declared-field names reached through a catalogue-shaped selector."""
    normalized = re.sub(r"\(\s*\*\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*\.\s*\w+)*)\s*\)",
                        r"\1", text)
    fields = set()
    for root in roots:
        path = re.escape(root)
        for match in re.finditer(
            r"(?<![\w.])%s(?P<tail>(?:\s*\.\s*[A-Za-z_][A-Za-z0-9_]*)+)"
            % path,
            normalized,
        ):
            fields.update(re.findall(r"\.\s*([A-Z][A-Za-z0-9]*)", match.group("tail")))
    return fields


def go_identifiers(text: str) -> str:
    """Exclude Go comments and literals from helper dependency discovery."""
    return re.sub(r'"(?:\\.|[^"\\])*"|`[^`]*`|\'(?:\\.|[^\'\\])*\'|//[^\n]*|/\*[\s\S]*?\*/',
                  lambda match: "\n" * match.group().count("\n"), text)


def helper_readers(production_sources: str) -> list[str]:
    """Follow called catalogue helpers and their package-level dependencies."""
    declarations = {}
    for path in CATALOGUE.glob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        text = go_identifiers(path.read_text())
        starts = list(re.finditer(r"^(?:func|var|const|type)\b", text, re.M))
        for index, start in enumerate(starts):
            end = starts[index + 1].start() if index + 1 < len(starts) else len(text)
            body = text[start.start():end]
            # Types and the catalogue itself are declarations, not readers.
            if body.startswith("type "):
                continue
            names = re.findall(r"^func (\w+)\(", body, re.M)
            if not body.startswith("func "):
                names += re.findall(r"^(?:var |const |\t)(\w+)\s*(?:=(?!=)|\s+\w+\s*=(?!=))", body, re.M)
            for name in names:
                if name != "Messages":
                    declarations[name] = body
    pending = re.findall(r"\bi18n\.([A-Z]\w*)\s*\(", go_identifiers(production_sources))
    visited = set()
    readers = []
    while pending:
        name = pending.pop()
        if name in visited or name not in declarations:
            continue
        visited.add(name)
        body = declarations[name]
        readers.append(body)
        pending.extend(re.findall(r"\b\w+\b", body))
    return readers

def main() -> int:
    if not CATALOGUE.is_dir():
        print("FAIL check-message-fields-are-read: %s does not exist, so this check read "
              "nothing" % CATALOGUE)
        return 1

    declared = {}
    for path in sorted(CATALOGUE.glob("*.go")):
        if path.name.endswith("_test.go") or path.name == "catalog.go":
            continue
        text = path.read_text()
        for match in FIELD.finditer(text):
            line = text[: match.start()].count("\n") + 1
            declared.setdefault(match.group(1), []).append(
                "%s:%d" % (path.relative_to(ROOT), line))

    if not declared:
        print("FAIL check-message-fields-are-read: no message field was found in %s, so "
              "this check read nothing" % CATALOGUE.relative_to(ROOT))
        return 1

    source_texts = []
    production_texts = []
    for directory in ("internal", "cmd"):
        for path in (ROOT / directory).rglob("*.go"):
            if path.is_relative_to(CATALOGUE):
                continue
            source_texts.append(path.read_text())
            if not path.name.endswith("_test.go"):
                production_texts.append(path.read_text())
    all_source = "\n".join(source_texts)
    catalogue_fields = {
        match.group(1) for match in re.finditer(
            r"\b([A-Za-z_][A-Za-z0-9_]*)\s+\*?\s*i18n\.Catalog\b", all_source)
    }
    readers = [(text, catalogue_roots(text, catalogue_fields))
               for text in source_texts]
    for text in helper_readers("\n".join(production_texts)):
        readers.append((text, catalogue_roots(text, catalogue_fields) | {"Messages"}))

    read_fields = set()
    for text, roots in readers:
        read_fields.update(catalogue_fields_read(text, roots))

    failures = []
    exemptions = []
    used_exemptions = set()
    for name, where in sorted(declared.items()):
        if name in read_fields:
            continue
        paths = tuple(location.rsplit(":", 1)[0] for location in where)
        key = (name, paths)
        if key in UNREAD_EXEMPTIONS:
            exemptions.append("%s declares %s: %s"
                              % (", ".join(where), name, UNREAD_EXEMPTIONS[key]))
            used_exemptions.add(key)
            continue
        failures.append("%s declares %s and no catalogue reader outside "
                        "internal/i18n names it, directly or through a called helper; "
                        "its translations describe text nobody can read"
                        % (", ".join(where), name))
    for key in sorted(set(UNREAD_EXEMPTIONS) - used_exemptions):
        failures.append("%s is no longer an unread field; remove its exemption"
                        % (UNREAD_EXEMPTIONS[key]))
    for exemption in exemptions:
        print("EXEMPT check-message-fields-are-read: " + exemption)
    for failure in failures:
        print("FAIL check-message-fields-are-read: %s" % failure)
    if failures:
        return 1
    print("check-message-fields-are-read: passed; %d message fields, every one "
          "without an explicit existing exemption has a direct or called-helper reader"
          % len(declared))
    return 0


if __name__ == "__main__":
    sys.exit(main())
