import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { transform } from "lightningcss";
import { parseSync, type ESTree } from "vite";

const grayToken = /\bgray-\d+\b/;
const backgroundProperty = /^(?:background|backgroundColor|background-color|--background|--card|--popover|--surface-raised|--muted)$/;

function sourceBackgrounds(filename: string, source: string): string[] {
  const failures: string[] = [];
  if (filename.endsWith(".css")) {
    transform({
      filename,
      code: Buffer.from(source),
      errorRecovery: false,
      visitor: {
        Declaration(declaration) {
          const property = declaration.property === "unparsed"
            ? declaration.value.propertyId.property
            : declaration.property === "custom" ? declaration.value.name : declaration.property;
          if (backgroundProperty.test(property) && grayToken.test(JSON.stringify(declaration.value))) {
            failures.push(`${filename}: ${property} contains a gray background token`);
          }
        }
      }
    });
    return failures;
  }
  const parsed = parseSync(filename, source, { lang: filename.endsWith("x") ? "tsx" : "ts" });
  expect(parsed.errors, filename).toEqual([]);
  const strings = (node: ESTree.Node): string[] => {
    if (node.type === "Literal" && typeof node.value === "string") return [node.value];
    if (node.type === "TemplateElement") return [node.value.raw];
    return Object.values(node).flatMap((value) => {
      if (Array.isArray(value)) return value.flatMap((child) => child?.type ? strings(child) : []);
      return value && typeof value === "object" && "type" in value ? strings(value as ESTree.Node) : [];
    });
  };
  const visit = (node: ESTree.Node): void => {
    if (node.type === "Property") {
      const key = node.key.type === "Identifier" ? node.key.name
        : node.key.type === "Literal" ? String(node.key.value) : "";
      const values = strings(node.value);
      if ((backgroundProperty.test(key) || (key.startsWith("--") && values.includes("backgroundColor"))) && values.some((value) => grayToken.test(value))) {
        failures.push(`${filename}:${node.start}: ${key} contains a gray background token`);
      }
    }
    for (const value of Object.values(node)) {
      if (Array.isArray(value)) {
        for (const child of value) if (child?.type) visit(child);
      } else if (value && typeof value === "object" && "type" in value) visit(value as ESTree.Node);
    }
  };
  visit(parsed.program);
  return failures;
}

test("all authored page and container backgrounds avoid gray palette tokens", () => {
  const root = new URL("../src/", import.meta.url);
  const failures = readdirSync(root, { recursive: true, withFileTypes: true }).flatMap((entry) => {
    if (!entry.isFile() || !/\.(?:[cm]?[jt]sx?|css)$/.test(entry.name)) return [];
    const filename = join(entry.parentPath, entry.name);
    return sourceBackgrounds(filename, readFileSync(filename, "utf8"));
  });
  expect(failures).toEqual([]);
});

test("background gate rejects quoted keys, conditional layers and CSS shorthand without banning text or borders", () => {
  for (const source of [
    'style({ backgroundColor: "gray-100" })',
    'style({ "backgroundColor": { default: "base", dark: "gray-200" } })',
    'style({ "--surface": { type: "backgroundColor", value: "gray-50" } })',
    'const element = <div style={{ background: `var(--spectrum-gray-100)` }} />'
  ]) expect(sourceBackgrounds("Probe.tsx", source), source).not.toEqual([]);
  for (const source of [
    '.card { background: var(--spectrum-gray-100) none; }',
    '.card { background-color: var(--gray-200); }',
    ':root { --card: var(--gray-100); }'
  ]) expect(sourceBackgrounds("probe.css", source), source).not.toEqual([]);
  expect(sourceBackgrounds("Probe.tsx", '// backgroundColor: "gray-100"\nstyle({ backgroundColor: "layer-1", color: "gray-900", borderColor: "gray-300" })')).toEqual([]);
  expect(sourceBackgrounds("probe.css", '/* background: var(--gray-100); */ .card { color: var(--gray-900); border-color: var(--gray-300); }')).toEqual([]);
});
