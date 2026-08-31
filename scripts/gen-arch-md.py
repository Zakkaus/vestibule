#!/usr/bin/env python3
"""Generate docs/ARCHITECTURE.md from web/architecture.html.

The page is the maintained artifact: it renders in the tokens it documents, so a
broken token breaks it. The Markdown copy exists so the content can be grepped
and diffed. Keeping both by hand is the drift the document itself warns about,
so this derives one from the other and the gate re-runs it and compares.
"""
import html as H
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC = ROOT / "web" / "architecture.html"
DST = ROOT / "docs" / "ARCHITECTURE.md"


def inline(s: str) -> str:
    s = re.sub(r"<br\s*/?>", "  \n", s)
    s = re.sub(r"<code>(.*?)</code>", r"`\1`", s, flags=re.S)
    s = re.sub(r"<b>(.*?)</b>|<strong>(.*?)</strong>",
               lambda m: "**%s**" % (m.group(1) or m.group(2)), s, flags=re.S)
    s = re.sub(r'<a [^>]*href="([^"]+)"[^>]*>(.*?)</a>', r"[\2](\1)", s, flags=re.S)
    s = re.sub(r"<span[^>]*>|</span>|<svg.*?</svg>", "", s, flags=re.S)
    s = re.sub(r"<[^>]+>", "", s)
    s = H.unescape(s)
    return re.sub(r"[ \t]*\n[ \t]*", " ", s).strip()


def table(block: str) -> list[str]:
    rows = re.findall(r"<tr>(.*?)</tr>", block, re.S)
    out: list[str] = []
    for i, row in enumerate(rows):
        cells = [inline(c) for c in re.findall(r"<t[hd][^>]*>(.*?)</t[hd]>", row, re.S)]
        if not cells:
            continue
        out.append("| " + " | ".join(cells) + " |")
        if i == 0:
            out.append("|" + "---|" * len(cells))
    return out


def convert(src: str) -> str:
    body = src[src.index("<main"):]
    out = ["# 架构", "",
           "<!-- 由 scripts/gen-arch-md.py 从 web/architecture.html 生成，不要手改。 -->",
           ""]
    # Walk the top-level blocks in document order.
    pattern = re.compile(
        r"<h2>(?P<h2>[^<]+)</h2>"
        r"|<h3>(?P<h3>[^<]+)</h3>"
        r"|<h4>(?P<h4>[^<]+)</h4>"
        r"|<table[^>]*>(?P<table>.*?)</table>"
        r"|<pre[^>]*>(?P<pre>.*?)</pre>"
        r"|<div class=\"(?:seq|flow)\">(?P<seq>.*?)</div>"
        r"|<li>(?P<li>.*?)</li>"
        r"|<p[^>]*>(?P<p>.*?)</p>",
        re.S)
    num = 0
    for m in pattern.finditer(body):
        if m.group("h2") is not None:
            out += ["", "## %d. %s" % (num, inline(m.group("h2"))), ""]
            num += 1
        elif m.group("h3") is not None:
            out += ["", "### " + inline(m.group("h3")), ""]
        elif m.group("h4") is not None:
            out += ["", "#### " + inline(m.group("h4")), ""]
        elif m.group("table") is not None:
            out += [""] + table(m.group("table")) + [""]
        elif m.group("pre") is not None or m.group("seq") is not None:
            raw = m.group("pre") or m.group("seq")
            text = H.unescape(re.sub(r"<[^>]+>", "", raw)).strip("\n")
            out += ["", "```text", text, "```", ""]
        elif m.group("li") is not None:
            out.append("- " + inline(m.group("li")))
        else:
            t = inline(m.group("p"))
            if t:
                out += ["", t, ""]
    text = "\n".join(out)
    return re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"


def main() -> int:
    generated = convert(SRC.read_text(encoding="utf-8"))
    if "--check" in sys.argv:
        current = DST.read_text(encoding="utf-8") if DST.exists() else ""
        if current != generated:
            print("docs/ARCHITECTURE.md is stale; run scripts/gen-arch-md.py", file=sys.stderr)
            return 1
        return 0
    DST.write_text(generated, encoding="utf-8")
    print("wrote %s (%d lines)" % (DST, generated.count("\n")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
