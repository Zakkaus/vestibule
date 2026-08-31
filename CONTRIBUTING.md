# Contributing

Issues and pull requests are welcome. This file is the single statement of the rules for
this repository. Where it conflicts with anything else, this file wins.

## Read one of these first

Do not start editing until you know which document governs the change.

| You are changing | Read |
|---|---|
| Console values, screens, copy | `web/design.html` |
| Packages, data, flows, reliability | `web/architecture.html` and `docs/ARCHITECTURE.md` |
| What to work on next, and when a phase is done | `docs/PLAN-v5.md` |

The two reference pages render in the tokens they document, so a broken token breaks the
page. Open them with `python3 -m http.server 8787 --bind 127.0.0.1 --directory web`.

## The project is mid-rewrite

The current tree is `gentoo-zh-verify-bot` v4.5.6 carried over and renamed. The target
architecture is not what the tree looks like today.

**New code goes into the packages `docs/ARCHITECTURE.md` declares, not into the packages
that happen to exist.** When the two disagree, the document is right and the tree is what we
are moving away from. If you need a package the document does not declare, change the
document first — say what it owns, what it may do, what it must not do — and only then write
code.

`docs/PLAN-v5.md` says which phase we are in and what that phase explicitly does not do.
Work outside the current phase does not get merged, however good it is.

## Invariants

These hold at every commit. Breaking one means the change is not finished.

1. The console and Telegram updates call the same service. Any path that writes state
   without going through it is a defect.
2. `internal/verification` does not import telego and contains no Telegram types.
3. Failure falls towards refusing. Not found, timed out, errored — nobody gets in. The only
   exception is an external condition explicitly configured to fail open.
4. State transitions are conditional updates. Never read-then-write. Zero rows affected
   means another path settled it first: treat it as settled, do not retry, do not error.
5. Write to the database before anything becomes externally visible. If the external call
   fails, delete the row.
6. A failed migration exits the process. Never serve traffic on an old schema.
7. No branch anywhere keys off a specific group. Delete our own community's rows and the
   product still works.
8. The bot token never reaches a log, a screen, or the repository.

## Limits that keep this from turning into spaghetti

Every change lands as complete architecture. "Tidy it later" has never once happened; it
only moves the cost to the next person.

| Thing | Limit | When you exceed it |
|---|---|---|
| One file | 600 lines | Split by responsibility, not by line count |
| One function | 80 lines | Extract sub-functions whose names say what they do |
| Cyclomatic complexity | 15 | Usually means branching should become a lookup or an interface |
| Concerns per commit | 1 | Behaviour and structure are two commits |

Limits are thresholds that start a conversation, not targets to approach. **When you get
close, ask whether it is in the wrong package.**

Never introduce a package named `util`, `common`, `helper` or `misc`. They have no boundary,
so everything ends up in them, and you get a second pile.

Also: one function does not fetch, decide and render; business packages do not assemble
user-visible strings — that belongs to `i18n`, and the business layer returns structured
results; and the second time a piece of logic appears, extract it. The third time means it
was extracted into the wrong place.

## Language

| Where | Language |
|---|---|
| Commit messages | English |
| `README.md` | English, with `README.zh-CN.md` alongside |
| Code comments | English |
| User-visible strings | Simplified Chinese source in the catalogue, plus Traditional Chinese and English |
| Design and architecture documents | Chinese |

Write Traditional Chinese natively; never derive it by converting Simplified Chinese. Keep
user-visible text in the catalogue — the localisation invariants reject Chinese literals in
production code, comments included.

See `internal/i18n/README.md` for the catalogue layout and translation workflow.

## Commits

- One commit per logical change. Squash the incremental fixups before opening a PR.
- Subject says what changed; body says why. The body does not restate the diff.
- Reference the issue or bug number in the body.
- **Every commit is GPG-signed.** Do not turn signing off. If the agent times out, fix the
  agent rather than the configuration; `git log --format='%h %G?'` must be all `G` before you
  push.
- No AI signatures or attribution anywhere: no `Co-Authored-By`, no generated-by line, no
  mention of which tool was used.
- Mechanical churn — reformatting, import reordering — goes in its own commit.

## Before opening a PR

CI runs these, and the release workflow runs the same gate before publishing binaries. Run
them locally first. **Clear the build and type caches first**: a stale cache turns a red gate
green locally.

```sh
gofmt -l .                       # must print nothing
go vet ./...
go build ./... && go build -tags gentoo ./...
go test -race ./... && go test -race -tags gentoo ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 -exclude=G304,G703,G706 ./...
```

**Both build tags must pass.** Running only the default one misses the generic edition.

Chinese documents and user-visible copy go through the prose checker before the PR.

### If you add a check, drive it red

A check written and never seen to fail usually cannot fail, and a check that always passes is
worse than none: it makes people think that spot is covered. Break the thing on purpose,
confirm it goes red, **confirm it goes red on your assertion and not somewhere else**, then
put it back.

### If you script a bulk edit

Assert the anchor exists and is unique before replacing anything, and check that the diff is
the size you expected. A script exiting zero is not evidence that it did the right thing.

## Pull requests

- Say why the change was made and what you verified. Skip the diff narration.
- Keep the template: description above the marker, checklist untouched.
- Tick a box only after running that check. An unticked box is information; a wrongly ticked
  one is misdirection.
- Watch CI afterwards. If something fails, read the log and fix the cause. Do not guess.

## Secrets

Never commit secrets. `BOT_TOKEN` and the optional `GITHUB_TOKEN` come from the environment;
`bot.env` and `config.json` are git-ignored. See `SECURITY.md` for reporting a vulnerability.

Two specific hazards in this project: the self-hosted Bot API server stores state in a
directory whose name is the bot token, so never list that directory; and a configuration file
gets pasted into issues and chats, so secrets go in by reference, never inline.
