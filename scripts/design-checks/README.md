# Vendored design-system checks

Copied verbatim from the shared design system so CI runs them without depending
on a path that exists on one machine. Re-copy when the upstream copies change;
do not edit them here, or the next re-copy silently reverts the edit.

| Check | What it refuses |
|---|---|
| `html-structure.py` | An unclosed tag, a duplicate id, a dead internal anchor, Markdown emphasis left in HTML, an absolute home path |
| `style-rules.py` | A literal spacing or radius value, and hue anywhere outside the token layer and the status classes |
| `css-coverage.py` | A class or `data-*` value defined without a demonstration, or used without a definition |

Each takes file paths as arguments and exits non-zero on a finding.
