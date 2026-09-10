import { createHash } from "node:crypto";
import type { Plugin } from "vite";

type ParseContext = {
  parse?: (source: string, options?: { sourceType?: "unambiguous" }) => unknown;
  error: (message: string) => never;
};

type AstNode = {
  type: string;
  start: number;
  end: number;
  [key: string]: unknown;
};

type Replacement = {
  start: number;
  end: number;
  text: string;
};

// S2 has no font opt-out, while Charts bundles a private S2 loader and style-loader CSS.
// Keep these transforms tied to the reviewed vendor bytes so bundle changes fail closed.
const S2_FONTS_SUFFIX = "/node_modules/@react-spectrum/s2/dist/private/Fonts.mjs";
const S2_FONTS_SHA256 = "a7d0a06f2eb12a7b10fe59371dd68861fa6c5e1bca6d8bde1ab8f73963e68e20";
const CHARTS_SUFFIX = "/node_modules/@spectrum-charts/react-spectrum-charts-s2/dist/index.js";
const CHARTS_SHA256 = "5ed8f9eea17c861f7b68e6d0f5bccd78abcadd94b36df1204e5d0d389d019301";

function sourceHash(source: string): string {
  return createHash("sha256").update(source).digest("hex");
}

function sourceId(id: string): string {
  return id.split("?")[0]!.replaceAll("\\", "/");
}

function walkAst(node: unknown, visit: (node: AstNode) => void): void {
  if (!node || typeof node !== "object") return;
  const astNode = node as AstNode;
  visit(astNode);
  for (const [key, value] of Object.entries(astNode)) {
    if (key === "start" || key === "end" || key === "loc") continue;
    if (Array.isArray(value)) {
      for (const child of value) walkAst(child, visit);
    } else {
      walkAst(value, visit);
    }
  }
}

function applyReplacements(source: string, replacements: Replacement[]): string {
  return [...replacements]
    .sort((a, b) => b.start - a.start)
    .reduce((result, replacement) => (
      result.slice(0, replacement.start) + replacement.text + result.slice(replacement.end)
    ), source);
}

function parseSource(context: ParseContext, source: string, id: string): AstNode {
  if (typeof context.parse !== "function") {
    context.error(`Cannot parse pinned font vendor input ${id}: Rolldown parser is unavailable`);
  }
  return context.parse(source, { sourceType: "unambiguous" }) as AstNode;
}

function failUnknownInput(context: Pick<ParseContext, "error">, id: string, reason: string): never {
  context.error(`Refusing unverified font vendor input ${id}: ${reason}`);
}

function transformS2Fonts(context: ParseContext, source: string, id: string): string {
  if (sourceHash(source) !== S2_FONTS_SHA256) {
    failUnknownInput(context, id, "source hash changed");
  }

  const ast = parseSource(context, source, id);
  const body = Array.isArray(ast.body) ? ast.body as AstNode[] : [];
  const imports = body.filter((node) => node.type === "ImportDeclaration");
  const dataDeclarations = body.filter((node) => node.type === "VariableDeclaration");
  const functions = body.filter((node) => node.type === "FunctionDeclaration");
  const fontImports = imports.filter((node) => {
    const imported = node.source as AstNode | undefined;
    return imported?.type === "Literal" && (
      imported.value === "./font-faces.css" || imported.value === "./font-faces_css.mjs"
    );
  });
  const loader = functions.find((node) => source.slice(node.start, node.end).includes("use.typekit.net"));

  if (
    body.length !== 10 ||
    imports.length !== 6 ||
    dataDeclarations.length !== 2 ||
    functions.length !== 1 ||
    fontImports.length !== 2 ||
    !loader ||
    (source.match(/use\.typekit\.net/g) ?? []).length !== 11
  ) {
    failUnknownInput(context, id, "expected S2 Fonts module structure changed");
  }

  const replacements: Replacement[] = [
    ...fontImports.map((node) => ({ start: node.start, end: node.end, text: "" })),
    ...dataDeclarations.map((node) => ({ start: node.start, end: node.end, text: "" })),
    { start: loader.start, end: loader.end, text: `function ${(loader.id as AstNode).name}() { return null; }` }
  ];
  const transformed = applyReplacements(source, replacements);
  if (transformed.includes("use.typekit.net") || transformed.includes("font-faces.css")) {
    failUnknownInput(context, id, "S2 font loader data was not fully removed");
  }
  return transformed;
}

function transformCharts(context: ParseContext, source: string, id: string): string {
  if (sourceHash(source) !== CHARTS_SHA256) {
    failUnknownInput(context, id, "source hash changed");
  }

  const ast = parseSource(context, source, id);
  const cssLiterals: AstNode[] = [];
  const typekitFunctions: AstNode[] = [];
  const fontDataDeclarations: AstNode[] = [];
  walkAst(ast, (node) => {
    if (node.type === "Literal" && typeof node.value === "string") {
      const value = node.value as string;
      if (value.includes("@font-face")) cssLiterals.push(node);
    }
    if (node.type === "FunctionDeclaration" && node.id && typeof (node.id as AstNode).name === "string") {
      const text = source.slice(node.start, node.end);
      if ((node.id as AstNode).name === "B" && text.includes("use.typekit.net")) typekitFunctions.push(node);
    }
    if (node.type === "VariableDeclaration") {
      const declarations = Array.isArray(node.declarations) ? node.declarations as AstNode[] : [];
      const names = declarations.map((declaration) => {
        const name = declaration.id as AstNode | undefined;
        return name?.type === "Identifier" ? name.name : undefined;
      });
      if (names.length === 2 && names[0] === "x" && names[1] === "k") fontDataDeclarations.push(node);
    }
  });

  const remoteCss = cssLiterals.filter((node) => {
    const value = node.value as string;
    return (value.match(/@font-face/g) ?? []).length === 16 &&
      (value.match(/use\.typekit\.net/g) ?? []).length === 48;
  });
  if (
    cssLiterals.length !== 3 ||
    remoteCss.length !== 3 ||
    typekitFunctions.length !== 1 ||
    fontDataDeclarations.length !== 1 ||
    (source.match(/@font-face/g) ?? []).length !== 48 ||
    (source.match(/use\.typekit\.net/g) ?? []).length !== 146
  ) {
    failUnknownInput(context, id, "expected Charts font module structure changed");
  }

  const loader = typekitFunctions[0]!;
  const data = fontDataDeclarations[0]!;
  const replacements: Replacement[] = [
    ...remoteCss.map((node) => ({ start: node.start, end: node.end, text: "\"\"" })),
    { start: data.start, end: data.end, text: "" },
    { start: loader.start, end: loader.end, text: "function B() { return null; }" }
  ];
  const transformed = applyReplacements(source, replacements);
  if (transformed.includes("use.typekit.net") || transformed.includes("@font-face")) {
    failUnknownInput(context, id, "Charts font loader data was not fully removed");
  }
  return transformed;
}

export function removeRemoteFonts(): Plugin {
  return {
    name: "console-remove-pinned-remote-fonts",
    enforce: "pre",
    transform(source, id) {
      const cleanId = sourceId(id);
      if (cleanId.endsWith(S2_FONTS_SUFFIX)) {
        return { code: transformS2Fonts(this, source, id), map: null };
      }
      if (cleanId.endsWith(CHARTS_SUFFIX)) {
        return { code: transformCharts(this, source, id), map: null };
      }
      return null;
    }
  };
}
