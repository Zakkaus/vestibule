# Vendored design-system checks

Copied verbatim from the shared design system so CI runs them without depending
on a path that exists on one machine. Re-copy when the upstream copies change;
do not edit them here, or the next re-copy silently reverts the edit.

| Check | What it refuses |
|---|---|
| `html-structure.py` | An unclosed tag, a duplicate id, a dead internal anchor, Markdown emphasis left in HTML, an absolute home path |
| `style-rules.py` | A literal spacing or radius value, and hue anywhere outside the token layer and the status classes |
| `css-coverage.py` | A class or `data-*` value defined without a demonstration, or used without a definition |
| `shadowed.py` | A declaration silently overridden by a later one in the same block |
| `undefined-var.py` | A `var(--x)` nothing defines — the whole declaration is dropped, in silence |
| `copy-drift.py` | A standalone page whose own copy of the component rules has drifted from the library |

Each takes file paths as arguments and exits non-zero on a finding. `copy-drift.py`
takes the page first, then the library stylesheets.

`copy-drift.py` reports nothing here: it compares `data-slot` vocabularies, and these
pages use classes. Recorded rather than dropped, so a later page built on the
attribute-based components is covered without anyone remembering to add it.
