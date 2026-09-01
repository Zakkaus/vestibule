# Vendored design-system checks

Copied byte-for-byte from `/home/zakk/code/skills/web-ui/examples/design-language/checks/` so CI runs them without depending on that machine-local path. Re-copy when the upstream copies change; do not edit them here, or the next re-copy silently reverts the edit.

| Check | What it refuses |
|---|---|
| `html-structure.py` | An unclosed tag, duplicate id, dead internal anchor, Markdown emphasis left in HTML, or absolute home path |
| `style-rules.py` | A literal spacing or radius value, a hue outside the token layer and status classes, or a physical box property with a logical equivalent |
| `css-coverage.py` | A class or `data-*` value defined without a demonstration, or used without a definition |
| `shadowed.py` | A declaration silently overridden by a later one in the same block |
| `undefined-var.py` | A `var(--x)` nothing defines — the whole declaration is dropped in silence |
| `theme-leak.py` | A rule that changes a value by theme outside the token layer |
| `comment-boundaries.py` | A comment boundary that swallows a selector or rule, a stray `*/`, or an invalid selector prelude |
| `coverage-floor.py` | A named stylesheet or inline stylesheet that parses to no rules or declarations |
| `padding-ratio.py` | A text-bearing component with inverted or out-of-band horizontal-to-vertical padding |
| `peer-consistency.py` | Components that share a row but disagree on height, corner radius, or type size |
| `percentage-min.py` | A percentage in a minimum inline or block size, which can resolve to zero |
| `shorthand-across-layers.py` | A shorthand in one loading stylesheet that resets a longhand another stylesheet sets for the same selector |

