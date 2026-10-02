"""Execution regressions shared by the gate self-coverage suite."""


from gate_invocations import invocations, workflow_run_text


class GateInvocationCases:
    def test_workflow_sequence_run_values(self) -> None:
        for workflow in (
            "steps:\n  - run: python3 scripts/check-docs.py\n",
            "steps:\n  - run: |\n"
            "      python3 scripts/check-docs.py\n"
            "    env:\n"
            "      DISPLAY: |\n"
            "        python3 scripts/check-schema-manifest.py\n",
        ):
            with self.subTest(workflow=workflow):
                self.assertEqual(
                    invocations(workflow_run_text(workflow)),
                    {"scripts/check-docs.py"},
                )

    def test_release_gate_rejects_printed_go_commands(self) -> None:
        tree = self.temporary_tree()
        for replacement in (
            "# go vet ./...",
            "echo 'go vet ./...'",
            "printf '%s\\n' 'go vet ./...'",
            "command echo 'go vet ./...'",
            "LABEL=gate echo 'go vet ./...'",
            "echo 'skipped; go vet ./...'",
            "echo ';' go vet ./...",
            "true # skipped; go vet ./...",
            "echo '$(go vet ./...)'",
            'label="go vet ./..."',
        ):
            with self.subTest(replacement=replacement):
                self.assert_mutation_is_rejected(
                    tree,
                    "scripts/check-release-gate.py",
                    "printed command text replaced the release vet check",
                    ("go vet",),
                    lambda replacement=replacement: self.replace_text(
                        tree,
                        ".github/workflows/release.yml",
                        "          go vet ./...\n",
                        "          " + replacement + "\n",
                    ),
                )

    def test_gate_checkers_reject_printed_repository_scripts(self) -> None:
        tree = self.temporary_tree()
        for checker, workflow, indent in (
            ("check-gate-list.py", "ci.yml", "        run: "),
            ("check-release-gate.py", "release.yml", "          "),
        ):
            command = "python3 scripts/check-schema-manifest.py deploy/vestibule-schema-manifest"
            for replacement in ("echo '%s'", "printf '%%s\\n' '%s'", "command echo '%s'"):
                with self.subTest(checker=checker, replacement=replacement):
                    self.assert_mutation_is_rejected(
                        tree,
                        "scripts/" + checker,
                        "printed command text replaced the schema manifest check",
                        ("scripts/check-schema-manifest.py",),
                        lambda replacement=replacement: self.replace_text(
                            tree,
                            ".github/workflows/" + workflow,
                            indent + command + "\n",
                            indent + replacement % command + "\n",
                        ),
                    )

    def test_release_exclusion_requires_executed_npm_command(self) -> None:
        tree = self.temporary_tree()
        self.assert_mutation_is_rejected(
            tree,
            "scripts/check-release-gate.py",
            "a printed npm command kept an obsolete release exclusion alive",
            ("npm run build", "CI does not run it any more"),
            lambda: self.replace_text(
                tree,
                ".github/workflows/ci.yml",
                "          cd web && npm ci --silent && npm run build\n",
                "          cd web && npm ci --silent && echo 'npm run build'\n",
            ),
        )
