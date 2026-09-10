"""Spectrum macro cases for the repository gate self-coverage suite."""
from __future__ import annotations

import json
from pathlib import Path


class SpectrumGateCases:
    def spectrum_fixture(self, tree: Path) -> tuple[Path, tuple[str, ...]]:
        frontend, arguments = self.frontend_fixture(tree)
        (frontend / "src/Probe.tsx").write_text(
            'import { style } from "@react-spectrum/s2/style" with { type: "macro" };\n'
            'export function Probe() { return <div className={style({ color: "body", '
            '"--metric-ink": { type: "color", value: "body" } })}>'
            '<div className="known" /></div>; }\n',
            encoding="utf-8",
        )
        for path in (frontend / "src/style.css", frontend / "dist/assets/index.css"):
            path.write_text(path.read_text(encoding="utf-8").replace(
                "color: var(--ink)", "color: var(--metric-ink)"
            ), encoding="utf-8")
        emitted = frontend / "dist/assets/index.css"
        emitted.write_text(emitted.read_text(encoding="utf-8") +
                           ".generated { --metric-ink: var(--ink); }\n", encoding="utf-8")
        manifest = frontend / "dist/css-provenance.json"
        value = json.loads(manifest.read_text(encoding="utf-8"))
        value["assets"][0]["origins"].append({
            "path": "src/Probe.tsx", "kind": "spectrum-macro"
        })
        manifest.write_text(json.dumps(value), encoding="utf-8")
        return frontend, arguments

    def spectrum_undefined_var_fixture(self, tree: Path) -> tuple[Path, tuple[str, ...]]:
        frontend, _ = self.spectrum_fixture(tree)
        (frontend / "spectrum-supplemental.css").write_text(
            ".dependency-runtime { --supplemental-ink: black; "
            "height: var(--disclosure-panel-height); }\n",
            encoding="utf-8",
        )
        source = frontend / "src/style.css"
        source.write_text(
            source.read_text(encoding="utf-8")
            + ".known { background: var(--supplemental-ink); }\n",
            encoding="utf-8",
        )
        return frontend, (
            "--definitions",
            str(frontend / "dist/assets/index.css"),
            "--definitions",
            str(frontend / "spectrum-supplemental.css"),
            str(frontend / "src/style.css"),
        )

    def test_spectrum_missing_source_reference_is_rejected(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.spectrum_undefined_var_fixture(tree)
        self.assert_mutation_is_rejected(
            tree,
            "scripts/design-checks/undefined-var.py",
            "a source stylesheet referenced an undefined custom property",
            ("reads --spectrum-source-missing, which nothing defines",),
            lambda: self.replace_text(
                tree,
                "web/css-gate-fixture/src/style.css",
                ".known { color: var(--metric-ink); }\n",
                ".known { color: var(--metric-ink); }\n"
                ".missing { color: var(--spectrum-source-missing); }\n",
            ),
            *arguments,
        )

    def test_spectrum_removed_emitted_definition_is_rejected(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.spectrum_undefined_var_fixture(tree)
        self.assert_mutation_is_rejected(
            tree,
            "scripts/design-checks/undefined-var.py",
            "a macro definition disappeared from emitted CSS",
            ("reads --metric-ink, which nothing defines",),
            lambda: self.replace_text(
                tree,
                "web/css-gate-fixture/dist/assets/index.css",
                ".generated { --metric-ink: var(--ink); }\n",
                "",
            ),
            *arguments,
        )

    def test_spectrum_theme_only_emitted_definition_is_rejected(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.spectrum_undefined_var_fixture(tree)
        self.assert_mutation_is_rejected(
            tree,
            "scripts/design-checks/undefined-var.py",
            "a source reference resolved only in the other theme",
            ("--metric-ink", "declared only inside a theme block"),
            lambda: self.replace_text(
                tree,
                "web/css-gate-fixture/dist/assets/index.css",
                ".generated { --metric-ink: var(--ink); }\n",
                "@media (prefers-color-scheme: dark) { "
                ".generated { --metric-ink: var(--ink); } }\n",
            ),
            *arguments,
        )

    def test_spectrum_missing_supplemental_definition_file_is_usage_error(self) -> None:
        tree = self.temporary_tree()
        frontend, _ = self.spectrum_fixture(tree)
        missing = frontend / "missing-supplemental.css"
        result = self.invoke_gate(
            tree,
            "scripts/design-checks/undefined-var.py",
            "--definitions",
            str(missing),
            str(frontend / "src/style.css"),
        )
        output = self.output(result)
        self.assertEqual(result.returncode, 2, output)
        self.assertIn("cannot read", output)
        self.assertIn(str(missing), output)

    def test_spectrum_macro_css_requires_source_provenance(self) -> None:
        tree = self.temporary_tree()
        frontend, arguments = self.spectrum_fixture(tree)
        manifest = frontend / "dist/css-provenance.json"

        def mutate():
            original = manifest.read_text(encoding="utf-8")
            value = json.loads(original)
            value["assets"][0]["origins"].pop()
            manifest.write_text(json.dumps(value), encoding="utf-8")
            return lambda: manifest.write_text(original, encoding="utf-8")

        self.assert_mutation_is_rejected(
            tree, "scripts/check-css-coverage.py",
            "a Spectrum macro source escaped CSS provenance",
            ("Spectrum macro sources missing from provenance", "Probe.tsx"),
            mutate, *arguments,
        )

    def test_runtime_style_function_cannot_claim_macro_provenance(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.spectrum_fixture(tree)
        self.assert_mutation_is_rejected(
            tree, "scripts/check-css-coverage.py",
            "a runtime style function claimed generated CSS provenance",
            ("Spectrum macro origins have no Spectrum macro import", "Probe.tsx"),
            lambda: self.replace_text(tree, "web/css-gate-fixture/src/Probe.tsx",
                                      "@react-spectrum/s2/style", "./runtime-style"),
            *arguments,
        )

    def test_spectrum_macro_variable_must_exist_in_emitted_css(self) -> None:
        tree = self.temporary_tree()
        _, arguments = self.spectrum_fixture(tree)
        self.assert_mutation_is_rejected(
            tree, "scripts/check-css-coverage.py",
            "a macro variable disappeared from the emitted stylesheet",
            ("Spectrum macro variables missing from emitted CSS", "--metric-ink"),
            lambda: self.replace_text(tree, "web/css-gate-fixture/dist/assets/index.css",
                                      ".generated { --metric-ink: var(--ink); }", ""),
            *arguments,
        )

    def test_theme_only_emitted_definition_rejects_unconditional_project_reference(
        self,
    ) -> None:
        tree = self.temporary_tree()
        frontend, _ = self.spectrum_fixture(tree)
        emitted = frontend / "dist/assets/index.css"
        project = frontend / "src/style.css"
        arguments = ("--definitions", str(emitted), str(project))
        script = "scripts/design-checks/undefined-var.py"
        self.assert_gate_passes(tree, script, *arguments)
        restore = self.replace_text(
            tree,
            "web/css-gate-fixture/dist/assets/index.css",
            ".generated { --metric-ink: var(--ink); }",
            "@media (prefers-color-scheme: dark) { :root { --metric-ink: var(--ink); } }",
        )
        try:
            self.assert_gate_rejects(
                tree,
                script,
                "an unconditional project reference reads a theme-only definition",
                ("--metric-ink", "declared only inside a theme block"),
                *arguments,
            )
        finally:
            restore()
        self.assert_gate_passes(tree, script, *arguments)

    def test_theme_only_emitted_definition_allows_matching_conditional_reference(
        self,
    ) -> None:
        tree = self.temporary_tree()
        frontend, _ = self.spectrum_fixture(tree)
        emitted = frontend / "dist/assets/index.css"
        project = frontend / "src/style.css"
        arguments = ("--definitions", str(emitted), str(project))
        script = "scripts/design-checks/undefined-var.py"
        self.assert_gate_passes(tree, script, *arguments)
        restore_emitted = self.replace_text(
            tree,
            "web/css-gate-fixture/dist/assets/index.css",
            ".generated { --metric-ink: var(--ink); }",
            "@media (prefers-color-scheme: dark) { :root { --metric-ink: var(--ink); } }",
        )
        restore_project = self.replace_text(
            tree,
            "web/css-gate-fixture/src/style.css",
            ".known { color: var(--metric-ink); }",
            "@media (prefers-color-scheme: dark) { .known { color: var(--metric-ink); } }",
        )
        try:
            self.assert_gate_passes(tree, script, *arguments)
        finally:
            restore_project()
            restore_emitted()
        self.assert_gate_passes(tree, script, *arguments)

    def test_genuine_unconditional_project_definition_allows_reference(self) -> None:
        tree = self.temporary_tree()
        frontend, _ = self.spectrum_fixture(tree)
        emitted = frontend / "dist/assets/index.css"
        project = frontend / "src/style.css"
        arguments = ("--definitions", str(emitted), str(project))
        script = "scripts/design-checks/undefined-var.py"
        self.assert_gate_passes(tree, script, *arguments)
        restore_emitted = self.replace_text(
            tree,
            "web/css-gate-fixture/dist/assets/index.css",
            ".generated { --metric-ink: var(--ink); }",
            "@media (prefers-color-scheme: dark) { :root { --metric-ink: var(--ink); } }",
        )
        restore_project = self.replace_text(
            tree,
            "web/css-gate-fixture/src/style.css",
            ":root { --ink: oklch(0.2 0 0); }",
            ":root { --ink: oklch(0.2 0 0); --metric-ink: var(--ink); }",
        )
        try:
            self.assert_gate_passes(tree, script, *arguments)
        finally:
            restore_project()
            restore_emitted()
        self.assert_gate_passes(tree, script, *arguments)

    def test_definition_only_css_ignores_vendor_reference_but_checks_project_reference(
        self,
    ) -> None:
        tree = self.temporary_tree()
        frontend, _ = self.spectrum_fixture(tree)
        emitted = frontend / "dist/assets/index.css"
        project = frontend / "src/style.css"
        arguments = ("--definitions", str(emitted), str(project))
        script = "scripts/design-checks/undefined-var.py"
        restore_emitted = self.replace_text(
            tree,
            "web/css-gate-fixture/dist/assets/index.css",
            ".generated { --metric-ink: var(--ink); }",
            ".generated { --metric-ink: var(--vendor-runtime); }",
        )
        try:
            self.assert_gate_passes(tree, script, *arguments)
            restore_project = self.replace_text(
                tree,
                "web/css-gate-fixture/src/style.css",
                ".known { color: var(--metric-ink); }",
                ".known { color: var(--app-missing); }",
            )
            try:
                self.assert_gate_rejects(
                    tree,
                    script,
                    "a project reference is missing from emitted definitions",
                    ("--app-missing", "which nothing defines"),
                    *arguments,
                )
            finally:
                restore_project()
        finally:
            restore_emitted()
        self.assert_gate_passes(tree, script, *arguments)
