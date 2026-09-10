#!/usr/bin/env python3
"""Prove each static gate rejects the regression it exists to prevent.

The normal source tree is deliberately valid, so running a gate against it proves
only that today's code happens to pass. These tests copy the tree, introduce one
realistic harmful change, and require the actual gate to reject it. Every case
also runs the unmodified and restored copy as a positive control.
"""
from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from collections.abc import Callable, Iterable
from pathlib import Path

from spectrum_gate_cases import SpectrumGateCases

ROOT = Path(__file__).resolve().parent.parent


class GateSelfCoverageTest(SpectrumGateCases, unittest.TestCase):
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

    def test_each_supported_locale_catalogue_is_required(self) -> None:
        tree = self.temporary_tree()
        locales = tree / "web/src/i18n/locales"
        for name in ("en", "zh-CN", "zh-TW"):
            path = locales / (name + ".json")
            original = path.read_text(encoding="utf-8")

            def mutate(path=path, original=original) -> Callable[[], None]:
                path.unlink()
                return lambda: path.write_text(original, encoding="utf-8")

            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-locale-catalogues.py",
                "a supported console catalogue disappeared",
                ("%s.json is a supported catalogue but is missing" % name,),
                mutate,
            )

    def test_gate_list_requires_each_go_invocation_in_both_directions(self) -> None:
        tree = self.temporary_tree()
        ci_cases = (
            ('        unformatted="$(gofmt -l .)"\n', "gofmt"),
            ("        run: go vet ./...\n", "go vet"),
            ("        run: go build ./...\n", "go build"),
            ("        run: go build -tags gentoo ./...\n", "go build -tags gentoo"),
            (
                "        run: go build -tags gentoo ./...\n",
                "go build -tags gentoo",
                "        run: go build -tags gentoo,integration ./...\n",
            ),
            (
                "        run: go build -tags gentoo ./...\n",
                "go build -tags gentoo",
                '        run: go build -tags="gentoo,integration" ./...\n',
            ),
            (
                "        run: go build -tags gentoo ./...\n",
                "go build -tags gentoo",
                '        run: go build -tags "gentoo,integration" ./...\n',
            ),
            ("        run: go test -race ./...\n", "go test -race"),
            (
                "        run: go test -race -tags gentoo ./...\n",
                "go test -race -tags gentoo",
            ),
            (
                "        run: go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...\n",
                "go run honnef.co/go/tools/cmd/staticcheck@v0.8.1",
            ),
            (
                "        run: go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...\n",
                "go run golang.org/x/vuln/cmd/govulncheck@v1.7.0",
            ),
            (
                "        run: go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 "
                "-exclude=G304,G703,G706 ./...\n",
                "go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0",
            ),
        )
        for case in ci_cases:
            old, key, *replacement = case
            new = replacement[0] if replacement else ""
            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-gate-list.py",
                "a documented Go gate disappeared from CI",
                (key,),
                lambda old=old, new=new: self.replace_text(
                    tree, ".github/workflows/ci.yml", old, new
                ),
            )

        document_cases = (
            ("gofmt -l .                       # must print nothing\n", "gofmt"),
            ("go vet ./...\n", "go vet"),
            (
                "go build ./... && go build -tags gentoo ./...\n",
                "go build",
                "go build -tags gentoo ./...\n",
            ),
            (
                "go build ./... && go build -tags gentoo ./...\n",
                "go build -tags gentoo",
                "go build ./...\n",
            ),
            (
                "go build ./... && go build -tags gentoo ./...\n",
                "go build -tags gentoo,integration",
                "go build ./... && go build -tags gentoo,integration ./...\n",
            ),
            (
                "go build ./... && go build -tags gentoo ./...\n",
                "go build -tags gentoo,integration",
                'go build ./... && go build -tags="gentoo,integration" ./...\n',
            ),
            (
                "go build ./... && go build -tags gentoo ./...\n",
                "go build -tags gentoo,integration",
                'go build ./... && go build -tags "gentoo,integration" ./...\n',
            ),
            (
                "go test -race ./... && go test -race -tags gentoo ./...\n",
                "go test -race",
                "go test -race -tags gentoo ./...\n",
            ),
            (
                "go test -race ./... && go test -race -tags gentoo ./...\n",
                "go test -race -tags gentoo",
                "go test -race ./...\n",
            ),
            (
                "go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...\n",
                "go run honnef.co/go/tools/cmd/staticcheck@v0.8.1",
            ),
            (
                "go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...\n",
                "go run golang.org/x/vuln/cmd/govulncheck@v1.7.0",
            ),
            (
                "go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 "
                "-exclude=G304,G703,G706 ./...\n",
                "go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0",
            ),
        )
        for case in document_cases:
            old, key, *replacement = case
            new = replacement[0] if replacement else ""
            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-gate-list.py",
                "a CI Go gate disappeared from the contributor contract",
                (key,),
                lambda old=old, new=new: self.replace_text(
                    tree, "CONTRIBUTING.md", old, new
                ),
            )

    def test_gate_list_rejects_lost_go_test_race(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-gate-list.py",
            "the default Go test lost its race detector",
            ("go test -race",),
            lambda: self.replace_text(
                tree,
                ".github/workflows/ci.yml",
                "        run: go test -race ./...\n",
                "        run: go test ./...\n",
            ),
        )

    def test_gate_list_rejects_changed_go_tool_version(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-gate-list.py",
            "a pinned Go analysis tool version changed",
            ("go run honnef.co/go/tools/cmd/staticcheck@v0.8.1",),
            lambda: self.replace_text(
                tree,
                ".github/workflows/ci.yml",
                "honnef.co/go/tools/cmd/staticcheck@v0.8.1",
                "honnef.co/go/tools/cmd/staticcheck@v0.8.2",
            ),
        )

    def test_gate_list_ignores_ci_step_names_comments_echo_and_literals(self) -> None:
        tree = self.temporary_tree()
        for replacement in (
            "        # go vet ./...\n",
            '        run: echo "go vet ./..."\n',
            '        run: echo "skipped; go vet ./..."\n',
            '        run: echo ";" go vet ./...\n',
            "        run: true # skipped; go vet ./...\n",
            "        run: echo '$(go vet ./...)'\n",
            "        run: echo \\$(go vet ./...)\n",
            '        run: label="skipped; go vet ./..."\n',
        ):
            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-gate-list.py",
                "a CI step name, comment, or echo pretended to run go vet",
                ("go vet",),
                lambda replacement=replacement: self.replace_text(
                    tree,
                    ".github/workflows/ci.yml",
                    "        run: go vet ./...\n",
                    replacement,
                ),
            )

    def test_documented_globs_do_not_excuse_missing_ci_checks(self) -> None:
        tree = self.temporary_tree()
        workflow = ".github/workflows/ci.yml"
        original = (tree / workflow).read_text(encoding="utf-8")
        removed = "".join(
            line[:len(line) - len(line.lstrip())] + "true\n"
            if "scripts/design-checks/" in line else line
            for line in original.splitlines(keepends=True)
        )
        for decoy in (
            "",
            "# scripts/design-checks/$c.py",
            'echo "scripts/design-checks/$c.py"',
            'label="scripts/design-checks/$c.py"',
        ):
            candidate = removed.replace(
                "        unformatted=",
                ("        " + decoy + "\n" if decoy else "") + "        unformatted=",
                1,
            )
            self.assert_mutation_is_rejected(
                tree,
                "scripts/check-gate-list.py",
                "documented or reported globs excused checks no longer executed by CI",
                ("scripts/design-checks/coverage-floor.py",),
                lambda candidate=candidate: self.replace_text(tree, workflow, original, candidate),
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

    def test_a_remote_asset_cannot_reach_the_console_bundle(self) -> None:
        # The bundle is build output, so the copied tree has none. Write the shape the gate
        # reads: a stylesheet whose assets are inline, then give it a font from a CDN.
        tree = self.temporary_tree()
        bundle = tree / "web" / "dist" / "assets" / "style.css"
        bundle.parent.mkdir(parents=True, exist_ok=True)
        bundle.write_text(
            '.mark{background-image:url("data:image/svg+xml,<svg/>")}\n', encoding="utf-8"
        )
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-no-external-assets.py",
            "the console asks a third party for its typeface on every page load",
            ("loads https://", "outside the instance"),
            lambda: self.replace_text(
                tree,
                "web/dist/assets/style.css",
                '.mark{',
                "@font-face{font-family:remote;src:url(https://use.typekit.net/af/x)}\n.mark{",
            ),
            "web/dist/assets/style.css",
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


    def frontend_fixture(self, tree: Path) -> tuple[Path, tuple[str, ...]]:
        frontend = tree / "web" / "css-gate-fixture"
        source = frontend / "src"
        dist = frontend / "dist" / "assets"
        source.mkdir(parents=True)
        dist.mkdir(parents=True)
        (source / "style.css").write_text(
            ":root { --ink: oklch(0.2 0 0); }\n.known { color: var(--ink); }\n",
            encoding="utf-8",
        )
        (source / "Probe.tsx").write_text(
            'export function Probe() { return <div className="known" />; }\n',
            encoding="utf-8",
        )
        (dist / "index.css").write_text(
            ":root { --ink: oklch(0.2 0 0); }\n.known { color: var(--ink); }\n",
            encoding="utf-8",
        )
        (frontend / "dist" / "css-provenance.json").write_text(
            json.dumps(
                {
                    "version": 1,
                    "assets": [
                        {
                            "file": "assets/index.css",
                            "origins": [{"path": "src/style.css", "kind": "project"}],
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )
        return frontend, (
            "--frontend",
            "web/css-gate-fixture",
            "--source",
            "web/css-gate-fixture/src",
            "--dist",
            "web/css-gate-fixture/dist",
            "--provenance",
            "web/css-gate-fixture/dist/css-provenance.json",
        )

    def test_frontend_project_css_cannot_be_relabelled_as_vendor(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)
        manifest = tree / "web/css-gate-fixture/dist/css-provenance.json"
        value = json.loads(manifest.read_text(encoding="utf-8"))
        value["assets"][0]["origins"][0]["kind"] = "vendor"
        manifest.write_text(json.dumps(value), encoding="utf-8")
        self.assert_gate_rejects(
            tree,
            "scripts/check-css-coverage.py",
            "project CSS was relabelled as dependency CSS",
            ("marks a non-dependency path as vendor", "src/style.css"),
            *arguments,
        )

    def test_frontend_unknown_project_class_is_rejected(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        path = tree / "web/css-gate-fixture/src/Probe.tsx"
        original = path.read_text(encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)
        path.write_text(original.replace("known", "missing"), encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                "scripts/check-css-coverage.py",
                "an unknown project class renders without authored CSS",
                (".missing is used in TSX but has no project CSS definition",),
                *arguments,
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)

    def test_frontend_class_literals_in_every_expression_branch_are_checked(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        path = tree / "web/css-gate-fixture/src/Probe.tsx"
        original = path.read_text(encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)
        path.write_text(
            original.replace(
                'className="known"',
                'className={active ? "known" : "missing"} '
                'UNSAFE_className={active ? "known" : "missing"}',
            ),
            encoding="utf-8",
        )
        try:
            self.assert_gate_rejects(
                tree,
                "scripts/check-css-coverage.py",
                "a class literal in a conditional branch renders without authored CSS",
                (".missing is used in TSX but has no project CSS definition",),
                *arguments,
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)

    def test_frontend_ambiguous_class_expression_is_not_silently_ignored(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "a dynamic class expression escaped static hook coverage",
            ("className",),
            lambda: self.replace_text(
                tree,
                "web/css-gate-fixture/src/Probe.tsx",
                'className="known"',
                'className={active ? "known" : runtimeClass}',
            ),
            *arguments,
        )

    def test_frontend_authored_css_must_be_listed_in_provenance(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)

        def mutate() -> Callable[[], None]:
            path = tree / "web/css-gate-fixture/src/escape.css"
            path.write_text(".escape { color: red; }\n", encoding="utf-8")
            return path.unlink

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "an authored stylesheet escaped provenance accounting",
            ("authored CSS files missing from provenance", "escape.css"),
            mutate,
            *arguments,
        )

    def test_frontend_fixture_html_cannot_demonstrate_live_css(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        style = tree / "web/css-gate-fixture/src/style.css"
        original_style = style.read_text(encoding="utf-8")
        fixture = tree / "web/css-gate-fixture/src/app.css.fixture.html"
        original_fixture = fixture.read_text(encoding="utf-8") if fixture.exists() else None

        def mutate() -> Callable[[], None]:
            style.write_text(original_style + ".fixture-only { color: red; }\n", encoding="utf-8")
            fixture.write_text('<div class="fixture-only"></div>\n', encoding="utf-8")

            def restore() -> None:
                style.write_text(original_style, encoding="utf-8")
                if original_fixture is None:
                    fixture.unlink()
                else:
                    fixture.write_text(original_fixture, encoding="utf-8")

            return restore

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "a demonstration-only fixture hid dead authored CSS",
            (".fixture-only is defined by project CSS but has no TSX use",),
            mutate,
            *arguments,
        )

    def test_frontend_project_runtime_property_cannot_use_vendor_exemption(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        source = tree / "web/css-gate-fixture/src/style.css"
        emitted = tree / "web/css-gate-fixture/dist/assets/index.css"
        original_source = source.read_text(encoding="utf-8")
        original_emitted = emitted.read_text(encoding="utf-8")

        def mutate() -> Callable[[], None]:
            source.write_text(
                original_source.replace("var(--ink)", "var(--disclosure-panel-height)"),
                encoding="utf-8",
            )
            emitted.write_text(
                original_emitted.replace("var(--ink)", "var(--disclosure-panel-height)"),
                encoding="utf-8",
            )

            def restore() -> None:
                source.write_text(original_source, encoding="utf-8")
                emitted.write_text(original_emitted, encoding="utf-8")

            return restore

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "project CSS used a dependency runtime variable without a project definition",
            ("project source has no project definition", "--disclosure-panel-height"),
            mutate,
            *arguments,
        )

    def test_frontend_runtime_exemption_needs_exact_vendor_source(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        emitted = tree / "web/css-gate-fixture/dist/assets/index.css"
        manifest = tree / "web/css-gate-fixture/dist/css-provenance.json"
        original_emitted = emitted.read_text(encoding="utf-8")
        original_manifest = manifest.read_text(encoding="utf-8")

        def mutate() -> Callable[[], None]:
            emitted.write_text(
                original_emitted
                + ".vendor-runtime { height: var(--disclosure-panel-height); }\n",
                encoding="utf-8",
            )
            value = json.loads(original_manifest)
            value["assets"][0]["origins"].append(
                {
                    "path": "node_modules/@react-spectrum/s2/dist/private/Other.css",
                    "kind": "vendor",
                }
            )
            manifest.write_text(json.dumps(value), encoding="utf-8")

            def restore() -> None:
                emitted.write_text(original_emitted, encoding="utf-8")
                manifest.write_text(original_manifest, encoding="utf-8")

            return restore

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "a broad dependency prefix hid a runtime property from its component stylesheet",
            ("runtime ownership is not verified", "--disclosure-panel-height"),
            mutate,
            *arguments,
        )

    def test_frontend_runtime_exemption_needs_js_setter_evidence(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        emitted = tree / "web/css-gate-fixture/dist/assets/index.css"
        manifest = tree / "web/css-gate-fixture/dist/css-provenance.json"
        original_emitted = emitted.read_text(encoding="utf-8")
        original_manifest = manifest.read_text(encoding="utf-8")
        stylesheet = (
            tree / "web/css-gate-fixture/node_modules/@react-spectrum/s2/dist/private/Disclosure.css"
        )
        setter = (
            tree / "web/css-gate-fixture/node_modules/react-aria/dist/private/disclosure/useDisclosure.js"
        )

        def mutate() -> Callable[[], None]:
            emitted.write_text(
                original_emitted
                + ".vendor-runtime { height: var(--disclosure-panel-height); }\n",
                encoding="utf-8",
            )
            value = json.loads(original_manifest)
            value["assets"][0]["origins"].append(
                {
                    "path": "node_modules/@react-spectrum/s2/dist/private/Disclosure.css",
                    "kind": "vendor",
                }
            )
            manifest.write_text(json.dumps(value), encoding="utf-8")
            stylesheet.parent.mkdir(parents=True)
            stylesheet.write_text(
                ".panel { height: var(--disclosure-panel-height); }\n",
                encoding="utf-8",
            )
            setter.parent.mkdir(parents=True)
            setter.write_text("// This fixture intentionally has no runtime setter.\n", encoding="utf-8")

            def restore() -> None:
                emitted.write_text(original_emitted, encoding="utf-8")
                manifest.write_text(original_manifest, encoding="utf-8")
                shutil.rmtree(tree / "web/css-gate-fixture/node_modules")

            return restore

        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-css-coverage.py",
            "a dependency stylesheet without a JS setter hid a runtime property",
            ("runtime ownership is not verified", "--disclosure-panel-height"),
            mutate,
            *arguments,
        )

    def test_styled_page_hook_deletion_is_visible_to_coverage(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        style = tree / "web/css-gate-fixture/src/style.css"
        probe = tree / "web/css-gate-fixture/src/Probe.tsx"
        original_style = style.read_text(encoding="utf-8")
        original_probe = probe.read_text(encoding="utf-8")
        styled = original_style + '\n[data-css-gate-page] { display: grid; }\n'
        marked = original_probe.replace("/>", 'data-css-gate-page />', 1)
        style.write_text(styled, encoding="utf-8")
        probe.write_text(marked, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)
        style.write_text(original_style, encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                "scripts/check-css-coverage.py",
                "a styled page hook was deleted from CSS",
                ("[data-css-gate-page] is used in TSX but has no project CSS definition",),
                *arguments,
            )
        finally:
            style.write_text(original_style, encoding="utf-8")
            probe.write_text(original_probe, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)

    def test_app_css_literal_radius_remains_a_style_rule_failure(self) -> None:
        tree = self.temporary_tree()
        path = tree / "web/src/app/app.css"
        original = path.read_text(encoding="utf-8")
        script = "scripts/design-checks/style-rules.py"
        self.assert_gate_passes(tree, script, "web/src/app/app.css")
        path.write_text(original + "[data-app-shell] { border-radius: 3px; }\n", encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                script,
                "app CSS introduced a literal radius outside the shared scale",
                ("literal border-radius", "3px"),
                "web/src/app/app.css",
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, script, "web/src/app/app.css")


    def test_frontend_emitted_css_must_resolve_custom_properties(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        path = tree / "web/css-gate-fixture/dist/assets/index.css"
        original = path.read_text(encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)
        path.write_text(original.replace("--ink:", "--other:"), encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                "scripts/check-css-coverage.py",
                "an emitted project rule reads an undefined custom property",
                ("unresolved custom properties", "--ink"),
                *arguments,
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, "scripts/check-css-coverage.py", *arguments)

    def test_frontend_authored_radius_rule_is_still_red(self) -> None:
        tree = self.temporary_tree()
        self.frontend_fixture(tree)
        path = tree / "web/css-gate-fixture/src/style.css"
        original = path.read_text(encoding="utf-8")
        script = "scripts/design-checks/style-rules.py"
        self.assert_gate_passes(tree, script, "web/css-gate-fixture/src/style.css")
        path.write_text(original + ".known { border-radius: 3px; }\n", encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                script,
                "an authored literal radius bypasses the shared control scale",
                ("literal border-radius", "3px"),
                "web/css-gate-fixture/src/style.css",
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, script, "web/css-gate-fixture/src/style.css")

    def test_frontend_authored_color_rule_is_still_red(self) -> None:
        tree = self.temporary_tree()
        self.frontend_fixture(tree)
        path = tree / "web/css-gate-fixture/src/style.css"
        original = path.read_text(encoding="utf-8")
        script = "scripts/design-checks/style-rules.py"
        self.assert_gate_passes(tree, script, "web/css-gate-fixture/src/style.css")
        path.write_text(original + ".known { color: #123456; }\n", encoding="utf-8")
        try:
            self.assert_gate_rejects(
                tree,
                script,
                "an authored hue bypasses the token palette",
                ("hue outside the token layer", "#123456"),
                "web/css-gate-fixture/src/style.css",
            )
        finally:
            path.write_text(original, encoding="utf-8")
        self.assert_gate_passes(tree, script, "web/css-gate-fixture/src/style.css")

    def test_frontend_unlisted_css_asset_is_rejected(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.frontend_fixture(tree)
        extra = tree / "web/css-gate-fixture/dist/assets/escape.css"
        extra.write_text(".escape { color: red; }\n", encoding="utf-8")
        self.assert_gate_rejects(
            tree,
            "scripts/check-css-coverage.py",
            "an emitted CSS file escaped provenance accounting",
            ("missing from provenance", "escape.css"),
            *arguments,
        )

if __name__ == "__main__":
    unittest.main(verbosity=2)
