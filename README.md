**English** · [简体中文](README.zh-CN.md)

# Vestibule

A Telegram group join-verification and moderation bot. One instance serves many groups,
and each group is configured by its own Telegram administrators through a web console.

The name is the room you wait in before you are let inside. Under approval mode, that is
literally where an applicant is: outside the group, holding.

## Status

**Being rewritten.** The tree started as `gentoo-zh-verify-bot` v4.5.6 carried over and
renamed. New deployments use a neutral multiple-choice example, not a Linux kernel
question. `docs/PLAN-v5.md` records the rewrite phases, their acceptance criteria,
and their exclusions.

The verification core is `internal/verification`, it imports no Telegram, and it reaches
the outside through three ports derived from its own call sites: `Gateway`, `LiveProbe`
and `Store`. Which phases are finished is recorded in one place, the status column of the
phase table in `docs/PLAN-v5.md`; this file used to repeat it and stopped at "phase six is underway".

The previous generation is still running in production and will not be replaced until this
one is ready.

## The four references

| To decide | Read |
|---|---|
| Console values, screen contents, copy rules | `web/design.html` |
| Package structure, data, flows, reliability | `web/architecture.html` and `docs/ARCHITECTURE.md` |
| Rules: limits, invariants, language, commits, the gate | `CONTRIBUTING.md` |
| What the software holds about people, and what an operator must state | `docs/PRIVACY.md` |

The rewrite order and each phase's acceptance criteria are in `docs/PLAN-v5.md`.

Both reference documents are pages. Open them locally:

```sh
python3 -m http.server 8787 --bind 127.0.0.1 --directory web
```

They render in the tokens they document, so a broken token breaks the page.

## Feed subscriptions

Feed subscriptions are effective group settings. The read-only console screen and `GET /api/chats/{id}/feeds` show their values and sources; `PUT /api/chats/{id}/feeds` replaces the complete feed override with revision and CSRF checks. `config.json` `feeds` entries are accepted only as a one-time startup import. Set `github_repos` to choose repositories and branches, then enable `issues` or `pulls` per repository; both event switches default to off. An omitted branch follows the repository's current default branch. See [`docs/GITHUB-FEEDS.md`](docs/GITHUB-FEEDS.md) for migration and delivery rules, and [`examples/feeds.json`](examples/feeds.json) for the PUT request body. RSS, Atom, JSON Feed ingestion and rich-text controls remain deferred.

## What it has to become

1. Anyone can add the bot to their own group and configure it themselves.
2. The web console covers every group setting; process-level `modules` explicitly enables optional `gentoo` and `linux` bot modules. An empty or absent list enables no optional module; the legacy `disabled_modules` key is rejected and must be migrated to `modules`.
3. State lives in a database and survives concurrency and restarts without loss or double
   settlement.
4. One command deploys it, and a failed upgrade rolls itself back.

The acceptance test is one sentence: **delete our own community's rows and the product still
works.**

## Architecture in one screen

```
cmd/bot/
internal/
├── app/            wiring, lifecycle, background tasks
├── verification/   state machine and policy. Gateway and Store are declared here
├── rules/          pure functions: normalisation, conditions, structural signals
├── telegram/       SDK, updates, send queue, its own store
├── console/        HTTP API, auth, embedded frontend
├── settings/  database/  status/
web/                frontend source
```

Interfaces are declared by the consumer, not the implementer. The console and Telegram
updates call the same service, so there is only one set of rules.

Hard limits, checked in CI: 600 lines per file, 80 lines per function, cyclomatic complexity
15, one concern per commit. New code goes in a package the architecture document already
declares.

## Deployment question bank

New groups use `quiz` verification with a built-in example that requires no operating-system
knowledge. The example is not a strong anti-automation check. Administrators can explicitly
select `kernel` or `mixed` in the verification settings.

For a native deployment, put a JSON file beside `config.json` and set
`"factory_questions_file": "questions.json"` in `config.json`; no rebuild is needed:

```json
{
  "questions": [
    {"q": "Select the word book.", "options": ["book", "chair"], "answer": 0}
  ],
  "fallback_questions": [
    {"q": "Type the word book.", "answers": ["book"]}
  ]
}
```

These are the existing question settings fields, not a separate import format. Include
at least one bank; each supplied bank must be a non-empty array. `answer` is a zero-based
option index. Relative paths resolve from the configuration file's directory;
absolute paths are also accepted. The process reads the bank at startup. If the setting is
absent, it uses the built-in examples; an explicitly configured missing or invalid file
prevents startup.

With the supplied Compose deployment, put the bank in the host directory named by
`VESTIBULE_STATE_DIRECTORY` and set
`"factory_questions_file": "/var/lib/vestibule/questions.json"`. That directory is
already mounted in the container; files beside the host `config.json` are not.
The bank must be readable by container UID 65532.

A Linux-flavoured bank ships as an example rather than as the default: see
[`examples/questions/`](examples/questions/), one file per supported language,
asking about kernel.org, gnu.org and how to save and quit vim. Point
`factory_questions_file` at the one matching the language your groups read.

The deployment bank applies to newly registered groups as well as existing groups without
their own bank. The question page shows the effective bank and its source: factory,
configuration file, or group override. Restoring a group removes its override and reveals
the inherited bank; it does not rewrite the deployment file. Edit the file and restart to
change the deployment bank. Existing per-group question and message editors remain available.

## Licence

See `LICENSE`.

Releases include `THIRD-PARTY-LICENSES`. Native installs place it at
`/usr/local/share/doc/vestibule/THIRD-PARTY-LICENSES`; containers carry it at
`/usr/share/doc/vestibule/THIRD-PARTY-LICENSES`. It covers the pinned Go runtime and
release dependencies, browser runtime dependencies, and vendored icons and styles.
The inventory records the missing upstream notice for the shared style source without
inventing a copyright holder.

Container builds append the actual Alpine runtime package inventory and source notices.
The corresponding source archives, Alpine build recipes and patches, and original
notices are shipped at `/usr/share/doc/vestibule/alpine-runtime-sources`.

After updating dependencies or vendored sources, run
`python3 scripts/generate-third-party-licenses.py`, then
`python3 scripts/check-third-party-licenses.py`. Regeneration and checking require network
access to pinned npm archives and Bot API/TDLib source notices.
