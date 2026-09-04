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

## Test chat identifiers

Go test files and `testdata/` use only the reserved synthetic Telegram supergroup block
whose identifiers start with `-1009`. The remaining eight or nine digits may vary, which
keeps both supported identifier lengths covered. Never copy a deployed chat identifier into
a test: chat identifiers grant no access, but they expose deployment topology without adding
test coverage.

`python3 scripts/check-test-chat-ids.py internal cmd testdata` enforces the block. The check
also fails when a target is missing, no test assets are found, or no supergroup identifier is
covered; an empty scan is not a pass.

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

## The gate is enforced, not remembered

`main` requires the `build` and `docs` checks to pass before a merge. It did not
until a PR was merged while its prose check was still failing, leaving `main` red
for a round — the rule was in this document and nothing was holding anyone to it.

`enforce_admins` is off deliberately, so a repository admin can still merge past
a check when there is a reason to. That is a decision someone takes knowingly;
what it replaces is taking it by accident.

A branch must also be up to date with `main` before it can merge. Two merged
changes were silently reverted before this was on: a worktree created before
`main` moved was squashed with `git reset --soft main && git add -A`, which
stages "make the tree look like it did before" and so records every file merged
in between as a deletion. The branch then merged cleanly and undid them. Squash a
branch by bringing it onto `main` first:

```sh
git fetch origin
git rebase origin/main
git reset --soft origin/main
git add -A
```

To remove the up-to-date requirement:
`gh api -X PATCH repos/Zakkaus/vestibule/branches/main/protection/required_status_checks -f strict=false`

To remove protection entirely:
`gh api -X DELETE repos/Zakkaus/vestibule/branches/main/protection`

### Migration rollback declarations

Every `migrations/*.sql` header must state whether an earlier binary can start on
its result. Use dbutil's `(compatible with vN+)` clause when it can; otherwise append
`[incompatible: reason]` to the header message. The reason is required because dbutil's
safe default is indistinguishable from a forgotten declaration without it.

## Before opening a PR

CI runs these. The release workflow runs the Go, deployment, and host-replacement
checks, the exhaustive console-route contract, and phase-acceptance coverage before
publishing binaries and container images. It skips frontend and document checks that do
not bear on a release, and the baseline ratchet, which compares a branch against its
base and has nothing to compare on a tag. Run them locally first.
**Clear the build and type caches first**: a stale cache turns a red gate green
locally.

```sh
gofmt -l .                       # must print nothing
scripts/lint.sh                  # package boundaries, file and function length, complexity
python3 scripts/check-test-chat-ids.py internal cmd testdata  # test topology stays synthetic
python3 scripts/check-baseline-ratchet.py origin/main   # a held violation may not grow
python3 scripts/test-gate-self-coverage.py  # every static gate rejects its recorded regression
go vet ./...
go build ./... && go build -tags gentoo ./...
go test -race ./... && go test -race -tags gentoo ./...
scripts/test-static-sqlite.sh    # the release build configuration, run rather than compiled
scripts/test-install.sh           # isolated install, upgrade, rollback, status, and uninstall
scripts/test-replacement.sh       # isolated host-unit request validation, rollback, and Docker-socket boundary
scripts/accept-phase9.sh          # phase-nine clauses stay full-sized; incomplete host paths print EXEMPT
go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 -exclude=G304,G703,G706 ./...
python3 scripts/gen-arch-md.py --check
python3 scripts/check-docs.py
python3 scripts/check-gate-list.py   # this list names every gate CI runs
python3 scripts/check-limits.py      # the limits stated here are the ones lint.go enforces
python3 scripts/check-console-copy.py  # no user-facing text written into a component
python3 scripts/check-privacy-tables.py   # docs/PRIVACY.md names every table holding a person
python3 scripts/check-migration-declarations.py migrations  # every migration declares rollback compatibility
python3 scripts/check-schema-manifest.py deploy/vestibule-schema-manifest  # released schema metadata matches migrations.Table
python3 scripts/check-phase-seams.py     # no screen reaches for a later phase's endpoints
python3 scripts/check-console-routes.py  # implemented console routes match the exhaustive architecture table

python3 scripts/check-phase-acceptance.py  # every completed plan phase has acceptance coverage
python3 scripts/check-acceptance-exemptions.py  # every EXEMPT reason can still be true
# The vendored copies must stay byte-identical to the design system they came
# from. That is two questions and they need two gates.
#
# "Did someone edit a copy in place" is answered inside the repository, against
# the hashes recorded when the copy was taken, so CI runs it:
python3 scripts/check-vendored.py    # copies match scripts/vendored-manifest.json
python3 scripts/check-locale-catalogues.py  # three catalogues agree, and the code's keys exist
python3 scripts/check-inherited-commands.py  # every command the previous generation answered still exists
python3 scripts/check-no-baked-identity.py  # no deployment's bot handle in shipped code
python3 scripts/check-message-fields-are-read.py  # a declared message field has a reader
python3 scripts/check-one-clock.py           # internal/verification reads time through its injected clock
python3 scripts/check-one-transport.py       # no screen reaches the API without the CSRF-bearing transport
python3 scripts/check-links-resolve.py       # no link points at a route that does not exist
python3 scripts/check-citations-resolve.py   # a documented file:line points at what the sentence names
python3 scripts/check-release-gate.py       # a tag gates whatever CI gates, or says why not
python3 scripts/check-binary-contract.py    # the unit and the binary agree on flags, environment and the stop signal
python3 scripts/check-writing-screens-know-a-stale-token.py  # a writing screen names csrf_invalid
python3 scripts/check-error-maps-hold-real-codes.py  # a screen maps only codes the API sends
python3 scripts/check-log-privacy.py         # no message body or challenge answer reaches the log
python3 scripts/check-mutations-authorise.py # every mutating handler authorises before it writes
python3 scripts/check-handlers-act-on-what-they-authorised.py  # and acts on that chat
python3 scripts/check-whole-table-writes.py  # a whole-table write names the guard that keeps it honest

# "Has the source moved forward" needs the source, which lives on a developer's
# machine, so it stays local. When it reports drift, re-copy and re-record the
# manifest; do not edit either side by hand. Read the direction before acting.
CHECKS=~/.claude/skills/web-ui/examples/design-language
DESIGN_STYLES=~/design-system/app
python3 "$CHECKS/checks/vendored.py" --source "$CHECKS/checks" scripts/design-checks/*.py
python3 "$CHECKS/checks/vendored.py" --source "$DESIGN_STYLES" web/src/styles/*.css

# The prose checker, the way CI runs it. Naming the gate was not enough: this
# document said CI runs it and gave no command, so it stayed a habit locally
# and a plan reached a pull request with a finding in it.
python3 ~/.claude/skills/chinese-skill/scripts/chinese_lint.py \
  docs/PRIVACY.zh-CN.md docs/PLAN-v5.md docs/ARCHITECTURE.md docs/README.md \
  web/design.html web/architecture.html
cd web && npm ci && npm run build && cd ..
for c in coverage-floor style-rules undefined-var shadowed theme-leak comment-boundaries percentage-min; do \
  python3 "scripts/design-checks/$c.py" web/dist/assets/*.css; done
for c in coverage-floor comment-boundaries padding-ratio peer-consistency percentage-min shorthand-across-layers; do \
  python3 "scripts/design-checks/$c.py" web/src/styles/tokens.css web/src/styles/components.css web/src/styles/shell.css web/src/app/app.css; done
python3 scripts/check-type-ramp.py
python3 scripts/check-css-coverage.py web/src/app/app.css web/src/app/app.css.fixture.html
for c in coverage-floor style-rules undefined-var shadowed theme-leak comment-boundaries percentage-min; do \
  python3 "scripts/design-checks/$c.py" internal/console/api/page.css; done
python3 scripts/check-css-coverage.py internal/console/api/page.css internal/console/api/page.css.fixture.html
python3 scripts/check-console-html.py
for c in html-structure coverage-floor style-rules shadowed undefined-var theme-leak comment-boundaries percentage-min; do \
  python3 "scripts/design-checks/$c.py" web/design.html web/architecture.html; done
python3 scripts/check-css-coverage.py web/design.html web/architecture.html
cd web && npm run e2e && cd ..  # the console journey and the render gate, in Chromium
```

The `gentoo` tag remains only as a compatibility regression: default and tagged commands must
select the same product behavior. It no longer selects an edition.

`scripts/baseline.txt` is phase zero's snapshot of the violations that already
existed. **A row may leave it; none may join.** A row leaves when the violation is
cleared, or when the code moves and the same violation appears at a new path
unchanged — a rename is a move, not new debt. Anything else is new code, and new
code meets the limits: 600 lines a file, 80 a function, complexity 15.

This was not written down, and a rewrite duly replaced twenty-two old rows with
twenty-seven new ones — a 1,440-line file and a function at complexity 79 — while
every gate stayed green. `scripts/check-baseline-ratchet.py` refuses it now.

A phase that removes baselined violations owns three edits, not one: clear the
violations, delete their rows from `scripts/baseline.txt`, and lower
`scripts/held.txt` to the new count. The gate names which rows went stale and
the acceptance script refuses while any remain, so none of the three can be
skipped quietly.

A completed phase's acceptance is a script, not a paragraph. Its script is
`scripts/accept-phase<phase-number>.sh`, and it prints the plan's clauses in
order. An `EXEMPT` line names a clause that cannot run
mechanically and why; it never reads as a pass.
`scripts/check-phase-acceptance.py` refuses a plan whose completed phase has
neither that script nor a written phase-level exemption. It reads whether a
reason is present, not whether it is still true, so
`scripts/check-acceptance-exemptions.py` reads the reason: an exemption may not
rest on a phase the plan marks complete, nor call a decision open without
citing a row of the open-questions table that is still open. Four reasons had
expired at once when it was written, and one of them was hiding a clause that
by then passed. Phase one's script
still compares the baselined-violation count in `scripts/held.txt` as a ratchet:
the check fails when the number rises **and** when it falls without the file
being lowered to match, so progress is recorded rather than left as headroom to
creep back into. Read as prose it proved nothing; as a script phase one refuses
an empty package, a platform type in the core, and a rise in the number of
baselined violations the phase-zero gate is holding.

Chinese documents and user-visible copy go through the prose checker — `docs/`,
`web/design.html` and `web/architecture.html`. CI runs it as the `Zakk-LLM/Chinese-skill`
action, one step per document, so this is a gate rather than a habit. `docs/INVENTORY.md` and `docs/previous-generation/` stay out: one is a
lookup table whose cells repeat by design, the other is the replaced bot's own
documents, kept as they were written. It did not use to be: the plan went unchecked for several rounds
because nobody had named it, and had seven findings when it was finally run. Naming
it was not enough — a document was merged with a finding in it the same day this
became a CI step. If it is prose people follow, it goes through the checker.

### Before handing a slice to someone else

`python3 scripts/preflight.py <spec.md> <worktree>` reads a dispatch
specification against the working copy it will run in: the branch it names is
the branch that is checked out, every path it cites exists, every `path:line`
lands on a line with something on it, and every 「section」 it points at is in
one of the documents it cites.

Four slices were refused because the specification was wrong rather than hard —
a section cited in the wrong document, a branch name carried over from the slice
it was copied from, a renamed path, and a line number that had moved. Each
refusal was correct and cost a round. On the day it was written this caught two
more: a line range ending on a closing brace, and a bare `server.go:274` that
reads fine and cannot be resolved.

It is not a CI step. It runs against files outside the repository, at the moment
a slice is handed over.

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


## Stopping is for what forces you to invent

An agent told to stop on any discrepancy stops on all of them, and the cost of a
stop is the whole dispatch. Seven refusals on one slice were all correct, and the
last two were a missing authorisation and a miscounted number in prose. Neither
changed what would have been built.

**Stop** when continuing means inventing something the documents were supposed to
supply: a type with no definition, two documents that contradict each other on
what to build, a permission the spec withholds for work it also demands. Say what
is missing, with the file and line, and change nothing.

**Report and continue** when the discrepancy does not change the work: a count in
prose that does not match the code, a stale line number, a name spelled two ways.
Put it in the report under its own heading. The next reader fixes it; the dispatch
still lands.

The test is whether an answer is needed to proceed correctly. If the work is the
same either way, the finding is a note, not a blocker.

## What may happen without asking

Merge a phase branch into `main` when all three of these hold, and say in the
report that they did:

1. The gate above passes, with caches cleared.
2. The numbered acceptance script for that phase passes —
   `scripts/accept-phase<phase-number>.sh`. Its `EXEMPT` lines name every
   non-mechanical clause and the reason it cannot run as a script.
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
