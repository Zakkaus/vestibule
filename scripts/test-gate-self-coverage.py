#!/usr/bin/env python3
"""Prove each static gate rejects the regression it exists to prevent.

The normal source tree is deliberately valid, so running a gate against it proves
only that today's code happens to pass. These tests copy the tree, introduce one
realistic harmful change, and require the actual gate to reject it. Every case
also runs the unmodified and restored copy as a positive control.
"""
from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import unittest
from collections.abc import Callable, Iterable
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


class GateSelfCoverageTest(unittest.TestCase):
    def temporary_tree(self) -> Path:
        directory = tempfile.TemporaryDirectory(prefix="vestibule-gate-")
        self.addCleanup(directory.cleanup)
        tree = Path(directory.name) / "vestibule"
        shutil.copytree(
            ROOT,
            tree,
            ignore=shutil.ignore_patterns(
                ".git", "__pycache__", ".pytest_cache", "node_modules", "dist"
            ),
        )
        return tree

    def command(self, tree: Path, *arguments: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            arguments,
            cwd=tree,
            capture_output=True,
            text=True,
            check=False,
        )

    @staticmethod
    def output(result: subprocess.CompletedProcess[str]) -> str:
        return result.stdout + result.stderr

    def invoke_gate(
        self, tree: Path, script: str, *arguments: str
    ) -> subprocess.CompletedProcess[str]:
        return self.command(tree, sys.executable, script, *arguments)

    def assert_gate_passes(self, tree: Path, script: str, *arguments: str) -> None:
        result = self.invoke_gate(tree, script, *arguments)
        self.assertEqual(
            result.returncode,
            0,
            "valid source must pass %s:\n%s" % (script, self.output(result)),
        )

    def assert_gate_rejects(
        self,
        tree: Path,
        script: str,
        harm: str,
        expected: Iterable[str],
        *arguments: str,
    ) -> None:
        result = self.invoke_gate(tree, script, *arguments)
        output = self.output(result)
        self.assertNotEqual(
            result.returncode,
            0,
            "%s escaped %s:\n%s" % (harm, script, output),
        )
        for fragment in expected:
            self.assertIn(fragment, output, "%s did not name the harm:\n%s" % (script, output))

    def replace_text(
        self, tree: Path, relative: str, old: str, new: str
    ) -> Callable[[], None]:
        path = tree / relative
        original = path.read_text(encoding="utf-8")
        self.assertEqual(
            original.count(old),
            1,
            "mutation anchor %r in %s moved or became ambiguous" % (old, relative),
        )
        path.write_text(original.replace(old, new, 1), encoding="utf-8")
        return lambda: path.write_text(original, encoding="utf-8")

    def assert_mutation_is_rejected(
        self,
        tree: Path,
        script: str,
        harm: str,
        expected: Iterable[str],
        mutate: Callable[[], Callable[[], None]],
        *arguments: str,
    ) -> None:
        self.assert_gate_passes(tree, script, *arguments)
        restore = mutate()
        try:
            self.assert_gate_rejects(tree, script, harm, expected, *arguments)
        finally:
            restore()
        self.assert_gate_passes(tree, script, *arguments)

    def test_every_whole_table_delete_without_chat_scope_names_a_guard(self) -> None:
        tree = self.temporary_tree()
        addition = """
func probeClearWholeTable(ctx context.Context, db *Database) error {
	_, err := db.Exec(ctx, `
		DELETE FROM warning_counter`)
	return err
}
"""

        def mutate() -> Callable[[], None]:
            path = tree / "internal/database/warning_store.go"
            original = path.read_text(encoding="utf-8")
            path.write_text(original + addition, encoding="utf-8")
            return lambda: path.write_text(original, encoding="utf-8")

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-whole-table-writes.py",
            "a raw Go-string delete can erase every group's warning rows without a guard",
            ("a write could erase groups it never read", "probeClearWholeTable"),
            mutate,
            "internal/database",
        )

    def test_an_authorised_handler_acts_on_the_chat_it_authorised(self) -> None:
        tree = self.temporary_tree()

        def mutate() -> Callable[[], None]:
            path = tree / "internal/console/api/settings.go"
            original = path.read_text(encoding="utf-8")
            insertion = "\tvar input settingsPatchRequest\n\tother, _ := strconv.ParseInt(request.URL.Query().Get(\"group\"), 10, 64)\n"
            self.assertEqual(original.count("\tvar input settingsPatchRequest\n"), 1)
            self.assertEqual(original.count("s.settings.Update(chatID,"), 1)
            changed = original.replace("\tvar input settingsPatchRequest\n", insertion, 1)
            path.write_text(changed.replace("s.settings.Update(chatID,", "s.settings.Update(other,", 1), encoding="utf-8")
            return lambda: path.write_text(original, encoding="utf-8")

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-handlers-act-on-what-they-authorised.py",
            "an administrator of one group can update another group's settings",
            ("patchSettings", "authorises chatID", "other", "settings.Update"),
            mutate,
        )

    def test_every_visible_component_word_comes_from_a_locale_table(self) -> None:
        tree = self.temporary_tree()
        old = """        <p data-entry-copy aria-live=\"polite\">
          {t(\"entry.loading.description\")}
        </p>"""
        new = """        <p data-entry-copy aria-live=\"polite\">
          This may take a moment on a slow connection.
        </p>"""
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-console-copy.py",
            "a multiline untranslated sentence is visible to every console reader",
            ("a viewer reads something a component wrote", "This may take a moment"),
            lambda: self.replace_text(tree, "web/src/features/entry/EntryScreen.tsx", old, new),
        )

    def test_every_visible_accessibility_label_comes_from_a_locale_table(self) -> None:
        tree = self.temporary_tree()
        old = '<table data-record-table data-audit-table aria-label={t("audit.tableLabel")}>'
        for attribute in (
            "aria-label='Open the group audit table'",
            "aria-label={`Open the group audit table`}",
        ):
            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-console-copy.py",
                "an untranslated accessibility label is read aloud or shown on hover",
                ("a viewer reads something a component wrote", "Open the group audit table"),
                lambda attribute=attribute: self.replace_text(
                    tree,
                    "web/src/features/audit/AuditTable.tsx",
                    old,
                    '<table data-record-table data-audit-table %s>' % attribute,
                ),
            )

    def test_every_screen_error_map_uses_a_code_the_api_can_send(self) -> None:
        tree = self.temporary_tree()
        old = """    case \"init_data_replayed\":
      return entryFixtureFor(null);
    default:"""
        new = """    case \"init_data_replayed\":
      return entryFixtureFor(null);
    case \"never_sent_code\":
      return entryFixtureFor(null);
    default:"""
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-error-maps-hold-real-codes.py",
            "a dead session-error branch hides the explanation an administrator needs",
            ("web/src/features/entry", "never_sent_code", "no writeError call sends"),
            lambda: self.replace_text(tree, "web/src/features/entry/EntryScreen.tsx", old, new),
        )

    def test_every_catalogue_message_field_has_a_real_reader(self) -> None:
        tree = self.temporary_tree()
        old = "\tBanTime Text\n"
        new = "\tBanTime Text\n\tReason Text\n"
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-message-fields-are-read.py",
            "three translations can describe a ban reason that nobody can ever read",
            ("declares Reason", "no catalogue reader outside internal/i18n"),
            lambda: self.replace_text(tree, "internal/i18n/bot.go", old, new),
        )

    def test_a_new_package_boundary_violation_cannot_join_the_baseline(self) -> None:
        tree = self.temporary_tree()
        initialized = self.command(tree, "git", "init", "--quiet")
        self.assertEqual(initialized.returncode, 0, self.output(initialized))
        committed = self.command(
            tree,
            "git",
            "-c",
            "commit.gpgsign=false",
            "-c",
            "user.name=gate test",
            "-c",
            "user.email=gate-test@example.invalid",
            "add",
            "-A",
        )
        self.assertEqual(committed.returncode, 0, self.output(committed))
        committed = self.command(
            tree,
            "git",
            "-c",
            "commit.gpgsign=false",
            "-c",
            "user.name=gate test",
            "-c",
            "user.email=gate-test@example.invalid",
            "commit",
            "--quiet",
            "-m",
            "baseline fixture",
        )
        self.assertEqual(committed.returncode, 0, self.output(committed))
        base = self.command(tree, "git", "rev-parse", "HEAD")
        self.assertEqual(base.returncode, 0, self.output(base))
        parent = base.stdout.strip()
        self.assert_gate_passes(tree, "scripts/check-baseline-ratchet.py", parent)

        source = tree / "internal/rules/probe_boundary.go"
        baseline = tree / "scripts/baseline.txt"
        original_baseline = baseline.read_text(encoding="utf-8")
        source.write_text(
            "package rules\n\nimport \"net\"\n\nvar _ = net.IPv4len\n", encoding="utf-8"
        )
        unheld = self.command(tree, "bash", "scripts/lint.sh")
        self.assertNotEqual(
            unheld.returncode,
            0,
            "a new internal/rules net import must fail before a baseline can hide it:\n%s"
            % self.output(unheld),
        )
        self.assertIn("package-boundary: new internal/rules/probe_boundary.go:3 imports net", self.output(unheld))

        baseline.write_text(
            original_baseline
            + "package-boundary\tinternal/rules/probe_boundary.go\t3\tnet\t0\n",
            encoding="utf-8",
        )
        held = self.command(tree, "bash", "scripts/lint.sh")
        self.assertEqual(
            held.returncode,
            0,
            "the fixture must prove a matching baseline row can silence lint:\n%s" % self.output(held),
        )
        self.assert_gate_rejects(
            tree,
            "scripts/check-baseline-ratchet.py",
            "a new package-boundary violation can be added to the debt baseline",
            ("new code may not be added to the baseline", "package-boundary internal/rules/probe_boundary.go net"),
            parent,
        )

        source.unlink()
        baseline.write_text(original_baseline, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-baseline-ratchet.py", parent)

    def test_every_implemented_console_route_is_in_the_exhaustive_table(self) -> None:
        tree = self.temporary_tree()

        def mutate() -> Callable[[], None]:
            path = tree / "internal/console/api/server.go"
            original = path.read_text(encoding="utf-8")
            old = """\t\tif len(rest) == 0 {
\t\t\ts.audit(writer, request, chatID)
\t\t\treturn
\t\t}
\tcase http.MethodPost:"""
            new = """\t\tif len(rest) == 0 {
\t\t\ts.audit(writer, request, chatID)
\t\t\treturn
\t\t}
\t\tif len(rest) == 1 && rest[0] == \"export\" {
\t\t\ts.exportAudit(writer, request, chatID)
\t\t\treturn
\t\t}
\tcase http.MethodPost:"""
            self.assertEqual(original.count(old), 1)
            export = """
func (s *Server) exportAudit(writer http.ResponseWriter, request *http.Request, chatID int64) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "exported"})
}
"""
            path.write_text(original.replace(old, new, 1) + export, encoding="utf-8")
            return lambda: path.write_text(original, encoding="utf-8")

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-console-routes.py",
            "an undocumented console endpoint can serve audit data without authorisation review",
            ("export", "exhaustive"),
            mutate,
        )

    def test_every_present_tense_document_link_exists(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-docs.py",
            "a reader following the documentation index reaches a missing document",
            ("docs/README.md", "NOPE.md", "does not exist"),
            lambda: self.replace_text(
                tree,
                "docs/README.md",
                "[`INVENTORY.md`](INVENTORY.md)",
                "[`INVENTORY.md`](NOPE.md)",
            ),
        )

    def test_every_local_file_line_citation_resolves_or_is_explicitly_foreign(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-citations-resolve.py",
            "a precise-looking citation sends a reader to a nonexistent local source file",
            ("internal/store/nonexistent.go:19", "no declared prefix covers it"),
            lambda: self.replace_text(
                tree,
                "docs/PLAN-v5.md",
                "internal/verification/state_restore.go:19",
                "internal/store/nonexistent.go:19",
            ),
        )

    def test_a_deployment_bot_handle_cannot_be_compiled_into_the_shipped_code(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-no-baked-identity.py",
            "the entry screen names one deployment's bot to every other deployment's operator",
            ("@example_verify_bot", "has to come from the instance"),
            lambda: self.replace_text(
                tree,
                "web/src/features/entry/instance.ts",
                "const transport = createApiTransport(() => undefined);",
                'const transport = createApiTransport(() => undefined);\nconst fallback = "@example_verify_bot";\nvoid fallback;',
            ),
        )


if __name__ == "__main__":
    unittest.main(verbosity=2)
