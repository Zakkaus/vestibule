**English** · [简体中文](README.zh-CN.md)

# Vestibule

A Telegram group join-verification and moderation bot. One instance serves many groups,
and each group is configured by its own Telegram administrators through a web console.

The name is the room you wait in before you are let inside. Under approval mode, that is
literally where an applicant is: outside the group, holding.

## Status

**Being rewritten.** The tree started as `gentoo-zh-verify-bot` v4.5.6 carried over and
renamed. Behaviour is unchanged so far: the rewrite is repackaging first, and every phase
so far has moved code without altering what it decides. `docs/PLAN-v5.md` has the twelve
phases, what each one accepts, and what it deliberately does not do.

Phase zero and phase one are complete. The verification core is `internal/verification`,
it imports no Telegram, and it reaches the outside through three ports derived from its own
call sites: `Gateway`, `LiveProbe` and `Store`. Phase six is underway in slices, one branch
per console screen.

The previous generation is still running in production and will not be replaced until this
one is ready.

## The three references

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

## What it has to become

1. Anyone can add the bot to their own group and configure it themselves.
2. The web console covers every group setting; process-level `disabled_modules` selects optional `gentoo` and `linux` bot modules.
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

## Licence

See `LICENSE`.
