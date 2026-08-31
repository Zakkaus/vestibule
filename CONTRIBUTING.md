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

## When you are stuck, look at what others shipped

Three sources, in descending order of weight:

| Source | What it answers |
|---|---|
| [Zakkaus/gentoo-zh-verify-bot](https://github.com/Zakkaus/gentoo-zh-verify-bot) | Why the previous generation does what it does. A lot of it was bought with production incidents; a check that looks redundant usually has a reason |
| [Hentioe/policr-mini](https://github.com/Hentioe/policr-mini) | What a mature peer product decided, and what it got wrong |
| [mautrix/go](https://github.com/mautrix/go), [mautrix/telegram](https://github.com/mautrix/telegram) | How a high-quality Go program of the same shape handles lifecycle, concurrency and error layering |

Cite them as `repository/path/file.go:line` so a reader can open the same line.

**They are practice, not authority.** Their value is that someone shipped this and lived with
the consequences — that is evidence about cost, not proof of correctness. Ask why they did it
that way and whether the reason holds here.

Three cases found while checking them: the peer product stores an owner flag in one field and
reads a different one; a large project's guide still documents a CI job that no longer exists;
a proof-of-work implementation counts hex characters, so each difficulty step is sixteen times
the last and nothing usable sits in between.

Cite `file:line` when you use one. Say so plainly when a reference disagrees with our
architecture — write down both options and their costs rather than quietly following either.
The previous generation carries the most weight because it ran with the same users on the same
platform, but it is not authority either.

Do not pay this cost for an obvious change. Reading code to fix a typo is waste.

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
python3 scripts/gen-arch-md.py --check
python3 scripts/check-docs.py
cd web && npm ci && npm run build && cd ..
for c in style-rules undefined-var shadowed theme-leak; do \
  python3 "scripts/design-checks/$c.py" web/dist/assets/*.css; done
for c in html-structure style-rules css-coverage shadowed undefined-var theme-leak; do \
  python3 "scripts/design-checks/$c.py" web/design.html web/architecture.html; done
```

**Both build tags must pass.** Running only the default one misses the generic edition.

A phase that removes baselined violations owns three edits, not one: clear the
violations, delete their rows from `scripts/baseline.txt`, and lower
`scripts/held.txt` to the new count. The gate names which rows went stale and
the acceptance script refuses while any remain, so none of the three can be
skipped quietly.

A phase's acceptance is a script, not a paragraph. `scripts/accept-phase1.sh` is phase
one's, clause by clause in the plan's own order. The count of baselined violations it
compares against lives in `scripts/held.txt` and is a ratchet: the check fails when the
number rises **and** when it falls without the file being lowered to match, so progress
is recorded rather than left as headroom to creep back into. Read as prose it proved nothing; as a
script it refuses an empty package, a platform type in the core, and a rise in the
number of baselined violations the phase-zero gate is holding.

Chinese documents and user-visible copy go through the prose checker before the PR —
`docs/`, `web/design.html` and `web/architecture.html`. The plan went unchecked for
several rounds because nobody had named it, and it had seven findings when it was
finally run. If it is prose people follow, it goes through the checker.

### If you add a check, drive it red

A check written and never seen to fail usually cannot fail, and a check that always passes is
worse than none: it makes people think that spot is covered. Break the thing on purpose,
confirm it goes red, **confirm it goes red on your assertion and not somewhere else**, then
put it back.

### If you script a bulk edit

Assert the anchor exists and is unique before replacing anything, and check that the diff is
the size you expected. A script exiting zero is not evidence that it did the right thing.

## Documents ship with the change that invalidates them

The plan, the architecture document and the design document rot before the code does. Update
them **in the PR that makes them wrong**, not in a later cleanup pass. A later cleanup pass does
not happen.

When a phase lands, check four things:

- the plan marks that phase done, and says what was actually built rather than what was intended
- work discovered during the phase is filed under the phase that owns it, not appended at the end
- conclusions the change overturned are corrected in the architecture document, not appended to
- design text for shipped behaviour reads as description, not as intent

`docs/ARCHITECTURE.md` is generated from `web/architecture.html`. Edit the HTML and regenerate;
a hand edit to the Markdown is lost on the next run, and CI fails on the difference.

`scripts/check-docs.py` covers the part of this that a machine can see: stated counts against
real structure, and references from present-tense documents to files that exist. The plan and
the architecture document are exempt from the second check because they legitimately name files
that do not exist yet.

**The failure it cannot see is two documents contradicting each other.** One page saying the
token is entered in the browser while another says the install script asks for it are both
readable sentences; whoever reads one of them first follows it. When you change a decision, grep
for the old one across `docs/` and `web/` before you commit.


## What may happen without asking

Merge a phase branch into `main` when all three of these hold, and say in the
report that they did:

1. The gate above passes, with caches cleared.
2. The phase's acceptance script passes — `scripts/accept-phase1.sh` for phase
   one, and its equivalent for later phases.
3. Every check added on that branch was driven red, and the report names the
   deliberate break that made it fail.

Any one of them missing means stop and say which. **A green run nobody tried to
break is not evidence.**

There is no integration branch. One existed in earlier drafts only because
merging to `main` needed a person each time; a branch whose sole purpose is to
hold finished work away from the trunk is a queue, not a safeguard.

Running the agents is authorised too: dispatching a slice, watching it, stopping
one that has stalled or wandered outside its brief, and dispatching the next.
Stopping one is a judgement, so the report says what it had produced and why it
was stopped — a slice killed at forty minutes with nothing kept is a cost, and
hiding it makes the next dispatch no better.

Still requires a person: publishing a release and pushing a tag, since a merge
is undone by another commit while a release has already been downloaded;
anything touching the production bot, its token, its state directory or its
groups, which is why there is a test bot and a test group; rewriting published
history on `main`; and deleting data.

A reversible decision inside the work is not one of these. A branch name, a file
layout, which of two words to standardise on — decide it, record the reasoning
where the next reader will find it, and say that a sentence overturns it. Asking
costs a round and hands back a decision you were better placed to make.

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
