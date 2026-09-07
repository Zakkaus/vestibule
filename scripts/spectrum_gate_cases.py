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
