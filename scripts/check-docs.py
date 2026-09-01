#!/usr/bin/env python3
"""Check this repository's documents against themselves.

Structure, style rules and coverage of the two pages belong to the vendored
design-system checks in scripts/design-checks. What is left here is what only
this repository knows: its plan, its route table, and its own cross-references.

Each check exists because the thing it catches shipped once and looked fine.
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PLAN = ROOT / "docs" / "PLAN-v5.md"

# A plan and an architecture describe the target state, so they may name files
# that do not exist yet. A README describes the present and may not.
FORWARD_LOOKING = {"docs/PLAN-v5.md", "docs/ARCHITECTURE.md"}

CN_NUM = {"零": 0, "一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6,
          "七": 7, "八": 8, "九": 9, "十": 10, "十一": 11, "十二": 12,
          "十三": 13, "十四": 14, "十五": 15}

HEADING = re.compile(r"^(#{1,6}) .*$", re.M)
HOME_PATH = re.compile(r"(?<![\w/])/(?:home|Users)/[A-Za-z0-9._-]+/")
REF = re.compile(r"`((?:docs|web|scripts)/[A-Za-z0-9_./-]+\.(?:md|html|py|sh|ya?ml))`")

# CONTRIBUTING.md opens by declaring itself the single statement of the rules.
# A second document that restates who may merge, push, release or tag does not
# stay a copy: the plan carried "未经维护者点头不合并、不发 PR、不 push" for
# several phases after CONTRIBUTING had granted exactly those three, and both
# sentences read as true. Restating is what drifts, so any other document that
# speaks about permission for those actions has to point here instead.
GOVERNED = re.compile(
    r"合并|合入 ?`?main|推送|push\b|pull request|发 ?PR|打标签|发布 ?v?\d|发布新版"
    r"|一次发布|release\b|\btag\b")
PERMISSION = re.compile(
    r"点头|授权|不许|不得|未经|自行"
    r"|without asking|approval|permission|may not|must not")
AUTHORITY = "What may happen without asking"

# A path:line citation in the plan is precise-looking and goes stale on its own:
# phase four shortened internal/panel/settings_panel.go by seventeen lines and
# phase seven went on citing 473–1398 for two phases. A citation about the tree
# as it was before the rewrite, or about the previous generation in ~/code/refs,
# cannot resolve here and says so on its own line.
CITED_LINE = re.compile(
    r"`([A-Za-z0-9_./-]+\.(?:go|ts|tsx|py|sh|md|html|json|ya?ml)):(\d+)(?:[\u2013-](\d+))?`")
HISTORICAL = re.compile(r"上一代|原先|重写前|refs/|已删除")
BULLET = re.compile(r"^\s*(?:[-*+]|\d+\.)\s")

failures: list[str] = []


def check_phase_count(path: Path, text: str, real: int) -> None:
    """Any document stating how many phases there are must state the real number.

    The plan was checked and nothing else was, so the README went on saying nine
    after the count reached eleven. A number is wrong in whichever file it sits.
    """
    for m in re.finditer(r"([零一二三四五六七八九十]+)个阶段", text):
        stated = CN_NUM.get(m.group(1))
        if stated in (None, 1) or stated == real:
            continue  # "一个阶段一个分支" states a ratio, not a total
        failures.append('%s: says "%s个阶段" but there are %d'
                        % (path.relative_to(ROOT), m.group(1), real))
    for m in re.finditer(r"\b(nine|ten|eleven|twelve|thirteen)\s+phases\b", text, re.I):
        word = {"nine": 9, "ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13}[m.group(1).lower()]
        if word != real:
            failures.append('%s: says "%s phases" but there are %d'
                            % (path.relative_to(ROOT), m.group(1), real))


def phase_count(text: str) -> int:
    return len(re.findall(r"^### 阶段([零一二三四五六七八九十]+) · (.+)$", text, re.M))


def check_plan_phases(text: str) -> None:
    """The phase table, the phase sections and any prose count must agree."""
    sections = re.findall(r"^### 阶段([零一二三四五六七八九十]+) · (.+)$", text, re.M)
    rows = re.findall(r"^\| ([零一二三四五六七八九十]+) \| `([^`]+)` \|", text, re.M)
    if not sections:
        failures.append("plan: no phase sections found")
        return
    if len(rows) != len(sections):
        failures.append("plan: %d rows in the phase table, %d phase sections"
                        % (len(rows), len(sections)))
    elif [r[0] for r in rows] != [s[0] for s in sections]:
        failures.append("plan: the phase table and the phase sections are in different orders")


def check_headings(path: Path, text: str) -> None:
    """A heading line carrying a second marker is a botched patch application.

    Applying a patch across a moved region produced `### A### A` on one line. It
    renders as a heading, so a read-through does not catch it.
    """
    for m in HEADING.finditer(text):
        if "#" in m.group(0)[len(m.group(1)):].lstrip():
            failures.append("%s: heading line contains a second marker: %s"
                            % (path.relative_to(ROOT), m.group(0)[:60]))


def check_home_paths(path: Path, text: str) -> None:
    """A public repository must not carry someone's working directory.

    Research arrived cited as an absolute path under a home directory. No reader
    can open it, so it is not evidence, and it publishes a directory layout for
    nothing. The vendored html-structure check covers the pages; this covers the
    Markdown, which nothing else reads.
    """
    for hit in sorted(set(HOME_PATH.findall(text))):
        failures.append("%s: contains an absolute home path (%s...)"
                        % (path.relative_to(ROOT), hit))


def check_links(path: Path, text: str) -> None:
    """Every document a present-tense document names has to exist."""
    for ref in sorted(set(REF.findall(text))):
        if not (ROOT / ref).exists():
            failures.append("%s: names %s, which does not exist"
                            % (path.relative_to(ROOT), ref))


def check_screen_coverage() -> None:
    """The route table calls itself exhaustive, so hold it to that.

    Declaring it exhaustive immediately exposed four screens with no write path.
    Without the check the claim would have been true for about a fortnight.
    """
    design = ROOT / "web" / "design.html"
    arch = ROOT / "docs" / "ARCHITECTURE.md"
    if not (design.exists() and arch.exists()):
        return
    d = design.read_text(encoding="utf-8")
    start = d.find("各屏职责")
    if start < 0:
        return
    screens = re.findall(r"<tr><td>([^<]+)</td>",
                         d[d.find("<table", start):d.find("</table>", start)])
    a = arch.read_text(encoding="utf-8")
    first, last = a.find("| GET /livez"), a.find("这张表是穷举的")
    if first < 0 or last < 0:
        failures.append("architecture: the route table or its exhaustiveness claim is gone")
        return
    region = a[first:last]
    for screen in screens:
        if screen not in region:
            failures.append("no route names the %s screen, but the route table "
                            "claims to be exhaustive" % screen)


def check_open_questions_have_a_future(plan_text: str) -> None:
    """No open question waits on a phase that has already finished.

    Each row of the open-questions table names the phase that will force the
    decision, so that nothing hangs unnoticed until somebody starts building it.
    The column cannot do that on its own: 结构信号 named phase two, phase two
    shipped, and the question was never asked. Nothing noticed, because nothing
    knew which phases were done.

    The status column in the phase table is what makes this checkable. It is
    maintained by hand, on merge; the value here is that the two tables cannot
    disagree in silence.
    """
    done = set()
    for match in re.finditer(r"^\| ([零一二三四五六七八九十]+) \| `[^`]+` \| [^|]* \| (\S+) \|",
                             plan_text, re.M):
        if match.group(2) == "完成":
            done.add(match.group(1))
    if not done:
        failures.append("plan: no phase is marked 完成 — has the status column moved?")
        return

    marker = plan_text.find("### 要维护者定")
    if marker < 0:
        failures.append("plan: the open-questions table is gone")
        return
    region = plan_text[marker:plan_text.find("###", marker + 3)]
    for match in re.finditer(r"^\| ([^|]+?) \| 阶段([零一二三四五六七八九十]+) \|", region, re.M):
        question, phase = match.group(1).strip(), match.group(2)
        if phase in done:
            failures.append("plan: 「%s」 waits on 阶段%s, which is already 完成"
                            % (question, phase))


def check_every_inventoried_file_has_a_phase(plan_text: str) -> None:
    """Every file the inventory dispositioned is claimed by some phase.

    docs/INVENTORY.md is the frozen per-file account of the tree the rewrite
    started from. A file listed there and named by no phase is work nobody has
    scheduled — it will surface at the end, as the thing that was always somebody
    else's.

    At the time this was written the count was zero, which is the reason to
    freeze it: an invariant is cheapest to hold from the moment it is true.

    Only the direction that matters is checked. A phase may cite something the
    inventory does not hold — deploy/ carries no Go code, and phases after the
    first name post-move paths by the plan's own rule — and those are not
    defects.
    """
    inventory = ROOT / "docs" / "INVENTORY.md"
    if not inventory.exists():
        return

    def expand(path: str) -> list:
        match = re.match(r"^(.*)\{([^}]*)\}(.*)$", path)
        if not match:
            return [path]
        before, inner, after = match.groups()
        return [before + part.strip() + after for part in inner.split(",")]

    listed = set()
    for match in re.finditer(r"^\| `([^`]+)`", inventory.read_text(encoding="utf-8"), re.M):
        for path in expand(match.group(1).split(":")[0]):
            if path.endswith(".go") and not path.endswith("_test.go"):
                listed.add(path)

    claimed = set()
    for section in re.findall(r"#### 文件处置(.*?)(?=####|\Z)", plan_text, re.S):
        for match in re.finditer(r"^\| `([^`]+)` \|", section, re.M):
            for path in expand(match.group(1).split(":")[0]):
                claimed.add(path)

    for path in sorted(listed - claimed):
        failures.append("plan: docs/INVENTORY.md dispositions %s and no phase claims it"
                        % path)


def check_phases_have_their_sections(text: str) -> None:
    """A phase that touches files says which files, what survives, and what it waits on.

    Phase nine had none of the three. It rewrites deploy/install.sh, keeps two
    systemd units whose hardening must survive verbatim, and cannot verify its own
    acceptance until phase five serves /livez — and the plan said none of that,
    while every other phase of the same kind said all of it.

    Phase zero built the gates before there was anything to move, and phase ten
    is a cutover rather than a change to files. Those two are exempt by name.
    """
    exempt = {"零", "十"}
    for match in re.finditer(r"^### 阶段([零一二三四五六七八九十]+) · (.+?)$(.*?)(?=^### |\Z)",
                             text, re.M | re.S):
        number, title, body = match.group(1), match.group(2), match.group(3)
        if number in exempt:
            continue
        for heading in ("#### 文件处置", "#### 必须保住的行为", "#### 依赖"):
            if heading not in body:
                failures.append("plan: 阶段%s · %s has no %s"
                                % (number, title, heading.strip("# ")))


def check_phase_branches_are_distinct(text: str) -> None:
    """No two phases claim the same branch.

    Phase five was written by copying phase two and editing the parts someone
    remembered: its heading and its file table are its own, its branch name and
    its opening paragraph still described extracting a pure rules package that
    phase two had already shipped. A dispatched agent reads the paragraph, not
    the heading.

    The branch name is the part a machine can hold. Two phases naming one branch
    is either a copy that was not finished or a plan that cannot be followed.
    """
    branches = {}
    for match in re.finditer(r"^\*\*分支\*\* `([^`]+)`", text, re.M):
        branches.setdefault(match.group(1), 0)
        branches[match.group(1)] += 1
    for branch, count in sorted(branches.items()):
        if count > 1:
            failures.append("plan: %d phases claim the branch %s"
                            % (count, branch))


def check_schema_matches_migration() -> None:
    """The architecture's schema and the migration name the same tables.

    Phase 3A added four tables the document had never drawn — the JSON states
    needed somewhere to live — and nothing noticed. The document is supposed to
    record what is established, and a schema is the one part of it a machine can
    hold to that.

    Names only, not column-by-column equivalence: the document annotates and
    reorders, and a checker that demands byte equality gets switched off.
    """
    migration = ROOT / "migrations" / "00-latest.sql"
    arch = ROOT / "docs" / "ARCHITECTURE.md"
    if not migration.exists() or not arch.exists():
        return

    def named(text: str) -> set:
        pattern = r"CREATE\s+(?:UNIQUE\s+)?(TABLE|INDEX)\s+([a-z_]+)"
        return {(kind.lower(), name) for kind, name in re.findall(pattern, text)}

    in_sql = named(migration.read_text(encoding="utf-8"))
    in_doc = named(arch.read_text(encoding="utf-8"))
    # dbutil creates and owns this one; the document explains it in prose.
    in_sql.discard(("table", "version"))

    for kind, name in sorted(in_sql - in_doc):
        failures.append("schema: migrations/00-latest.sql creates %s %s, "
                        "which docs/ARCHITECTURE.md does not show" % (kind, name))
    for kind, name in sorted(in_doc - in_sql):
        failures.append("schema: docs/ARCHITECTURE.md shows %s %s, "
                        "which migrations/00-latest.sql does not create" % (kind, name))


def list_items(text: str) -> list[str]:
    """Split into list items, each carrying its own continuation lines."""
    items: list[str] = []
    current: list[str] = []
    for line in text.split("\n"):
        if BULLET.match(line) or not line.strip():
            if current:
                items.append("\n".join(current))
            current = [line] if line.strip() else []
        else:
            current.append(line)
    if current:
        items.append("\n".join(current))
    return items


def check_plan_citations_resolve(plan_text: str) -> None:
    checked = 0
    for line in plan_text.split("\n"):
        if HISTORICAL.search(line):
            continue
        for path_text, start, end in CITED_LINE.findall(line):
            checked += 1
            path = ROOT / path_text
            if not path.exists():
                failures.append("plan: cites %s:%s and that file is not in the tree"
                                % (path_text, start))
                continue
            total = len(path.read_text(encoding="utf-8", errors="replace").split("\n"))
            highest = int(end or start)
            if highest > total:
                failures.append("plan: cites %s:%s-%s, and the file has %d lines"
                                % (path_text, start, end or start, total))
    if checked == 0:
        failures.append("plan: no path:line citation was checked — has the "
                        "citation format changed?")


def check_rules_are_stated_once(documents: list[Path]) -> None:
    contributing = ROOT / "CONTRIBUTING.md"
    if not contributing.exists():
        failures.append("CONTRIBUTING.md is missing, and it holds the rules")
        return
    if AUTHORITY not in contributing.read_text(encoding="utf-8"):
        failures.append("CONTRIBUTING: the 「%s」 section is gone, so nothing "
                        "defines what may be merged without asking" % AUTHORITY)
    scanned = 0
    for path in documents:
        if path == contributing or not path.exists():
            continue
        scanned += 1
        # Scoped to one list item, not the paragraph around it. The first
        # version exempted a whole block whenever any line in it cited
        # CONTRIBUTING, so the contradicting bullet went unreported once a
        # sibling bullet carried the citation — driving it red is what showed
        # that, since the probe came back green.
        for item in list_items(path.read_text(encoding="utf-8")):
            if "CONTRIBUTING" in item:
                continue
            for line in item.split("\n"):
                if PERMISSION.search(line) and GOVERNED.search(line):
                    failures.append(
                        "%s: states who may merge, push, release or tag without "
                        "pointing at CONTRIBUTING.md: %s"
                        % (path.relative_to(ROOT), line.strip()[:70]))
    if scanned == 0:
        failures.append("check_rules_are_stated_once read no documents at all")


def main() -> int:
    phases = 0
    if not PLAN.exists():
        failures.append("docs/PLAN-v5.md is missing")
    else:
        plan_text = PLAN.read_text(encoding="utf-8")
        phases = phase_count(plan_text)
        check_plan_phases(plan_text)
        check_phase_branches_are_distinct(plan_text)
        check_phases_have_their_sections(plan_text)
        check_every_inventoried_file_has_a_phase(plan_text)
        check_open_questions_have_a_future(plan_text)
        check_plan_citations_resolve(plan_text)

    check_schema_matches_migration()

    documents = list((ROOT / "docs").rglob("*.md")) + [
        ROOT / "README.md",
        ROOT / "README.zh-CN.md",   # it drifted to "八个阶段尚未开始" while unchecked
        ROOT / "CONTRIBUTING.md",
    ]
    for path in documents:
        if not path.exists():
            continue
        text = path.read_text(encoding="utf-8")
        check_headings(path, text)
        check_home_paths(path, text)
        check_phase_count(path, text, phases)
        if str(path.relative_to(ROOT)) not in FORWARD_LOOKING:
            check_links(path, text)

    check_rules_are_stated_once(documents)
    check_screen_coverage()

    if failures:
        for f in failures:
            print("FAIL check-docs: " + f, file=sys.stderr)
        return 1
    print("check-docs: passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
