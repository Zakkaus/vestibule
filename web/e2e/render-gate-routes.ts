import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

export type RenderRoute = Readonly<{
  sourcePath: string;
  urlPath: string;
}>;

export type LocaleCatalogues = Readonly<Record<string, readonly string[]>>;

const appFile = fileURLToPath(new URL("../src/app/App.tsx", import.meta.url));
const i18nFile = fileURLToPath(new URL("../src/i18n/index.ts", import.meta.url));

function skipQuotedTextOrComment(source: string, start: number): number {
  const character = source[start];
  const nextCharacter = source[start + 1];

  if (character === "'" || character === '"' || character === "`") {
    for (let cursor = start + 1; cursor < source.length; cursor += 1) {
      if (source[cursor] === "\\") {
        cursor += 1;
        continue;
      }
      if (source[cursor] === character) {
        return cursor + 1;
      }
    }
    throw new Error("Unterminated string literal while reading route declarations");
  }

  if (character === "/" && nextCharacter === "/") {
    const lineEnd = source.indexOf("\n", start + 2);
    return lineEnd === -1 ? source.length : lineEnd + 1;
  }

  if (character === "/" && nextCharacter === "*") {
    const commentEnd = source.indexOf("*/", start + 2);
    if (commentEnd === -1) {
      throw new Error("Unterminated block comment while reading route declarations");
    }
    return commentEnd + 2;
  }

  return start;
}

function matchingDelimiter(
  source: string,
  start: number,
  opening: string,
  closing: string
): number {
  let depth = 0;

  for (let cursor = start; cursor < source.length; cursor += 1) {
    const skipped = skipQuotedTextOrComment(source, cursor);
    if (skipped !== cursor) {
      cursor = skipped - 1;
      continue;
    }

    if (source[cursor] === opening) {
      depth += 1;
      continue;
    }
    if (source[cursor] === closing) {
      depth -= 1;
      if (depth === 0) {
        return cursor;
      }
    }
  }

  throw new Error(`Unterminated ${opening}${closing} pair while reading route declarations`);
}

function directPropertyValue(
  source: string,
  objectStart: number,
  objectEnd: number,
  propertyName: string
): number | undefined {
  let nesting = 0;

  for (let cursor = objectStart + 1; cursor < objectEnd; cursor += 1) {
    const skipped = skipQuotedTextOrComment(source, cursor);
    if (skipped !== cursor) {
      cursor = skipped - 1;
      continue;
    }

    const character = source[cursor];
    if (character === "{" || character === "[" || character === "(") {
      nesting += 1;
      continue;
    }
    if (character === "}" || character === "]" || character === ")") {
      nesting -= 1;
      continue;
    }
    if (nesting !== 0 || !/[A-Za-z_$]/.test(character)) {
      continue;
    }

    const nameStart = cursor;
    while (/[A-Za-z0-9_$]/.test(source[cursor] ?? "")) {
      cursor += 1;
    }
    const name = source.slice(nameStart, cursor);
    while (/\s/.test(source[cursor] ?? "")) {
      cursor += 1;
    }
    if (source[cursor] !== ":") {
      continue;
    }
    cursor += 1;
    while (/\s/.test(source[cursor] ?? "")) {
      cursor += 1;
    }

    if (name === propertyName) {
      return cursor;
    }
    cursor -= 1;
  }

  return undefined;
}

function literalStringAt(source: string, start: number): string | undefined {
  if (source[start] !== '"') {
    return undefined;
  }

  const end = skipQuotedTextOrComment(source, start);
  return JSON.parse(source.slice(start, end)) as string;
}

function directRouteObjects(
  source: string,
  arrayStart: number,
  arrayEnd: number
): readonly (readonly [number, number])[] {
  const routes: (readonly [number, number])[] = [];

  for (let cursor = arrayStart + 1; cursor < arrayEnd; cursor += 1) {
    const skipped = skipQuotedTextOrComment(source, cursor);
    if (skipped !== cursor) {
      cursor = skipped - 1;
      continue;
    }
    if (source[cursor] !== "{") {
      continue;
    }

    const end = matchingDelimiter(source, cursor, "{", "}");
    routes.push([cursor, end]);
    cursor = end;
  }

  return routes;
}

function sourceStringArray(fileName: string, exportName: string): string[] {
  const source = readFileSync(fileName, "utf8");
  const declaration = new RegExp(`export\\s+const\\s+${exportName}\\s*=\\s*\\[`);
  const declarationMatch = declaration.exec(source);

  if (!declarationMatch || declarationMatch.index === undefined) {
    throw new Error(`${fileName}: missing ${exportName} string-array export`);
  }

  const arrayStart = source.indexOf("[", declarationMatch.index);
  const arrayEnd = matchingDelimiter(source, arrayStart, "[", "]");
  const strings = [...source.slice(arrayStart + 1, arrayEnd).matchAll(/"((?:\\.|[^"\\])*)"/g)].map(
    (match) => JSON.parse(`"${match[1]!}"`) as string
  );

  if (strings.length === 0) {
    throw new Error(`${fileName}: ${exportName} must contain string literals`);
  }

  return strings;
}

function routerArray(source: string): readonly [number, number] {
  const routerCall = source.indexOf("createBrowserRouter(");
  if (routerCall === -1) {
    throw new Error(`${appFile}: missing createBrowserRouter call`);
  }

  const arrayStart = source.indexOf("[", routerCall);
  if (arrayStart === -1) {
    throw new Error(`${appFile}: createBrowserRouter needs a route array`);
  }

  return [arrayStart, matchingDelimiter(source, arrayStart, "[", "]")];
}

function joinRoutePath(parentPath: string, routePath: string): string {
  if (routePath.startsWith("/")) {
    return routePath;
  }

  return parentPath === "/" ? `/${routePath}` : `${parentPath}/${routePath}`;
}

function navigablePath(sourcePath: string): string {
  const segments = sourcePath
    .split("/")
    .filter(Boolean)
    .map((segment) => {
      if (segment === "*") {
        return "render-gate-unmatched";
      }

      if (segment.startsWith(":")) {
        const parameterName = segment.slice(1).replace(/[^a-zA-Z0-9_-]/g, "");
        return `render-gate-${parameterName || "param"}`;
      }

      return segment;
    });

  return segments.length === 0 ? "/" : `/${segments.join("/")}`;
}

function collectLeafRoutes(
  source: string,
  arrayStart: number,
  arrayEnd: number,
  parentPath: string,
  routes: RenderRoute[]
): void {
  for (const [objectStart, objectEnd] of directRouteObjects(source, arrayStart, arrayEnd)) {
    const pathValue = directPropertyValue(source, objectStart, objectEnd, "path");
    const indexValue = directPropertyValue(source, objectStart, objectEnd, "index");
    const childrenValue = directPropertyValue(source, objectStart, objectEnd, "children");
    const path = pathValue === undefined ? undefined : literalStringAt(source, pathValue);
    const isIndex = source.startsWith("true", indexValue);
    const resolvedPath = isIndex
      ? parentPath
      : path
        ? joinRoutePath(parentPath, path)
        : parentPath;

    if (childrenValue !== undefined && source[childrenValue] === "[") {
      const childrenEnd = matchingDelimiter(source, childrenValue, "[", "]");
      collectLeafRoutes(source, childrenValue, childrenEnd, resolvedPath, routes);
      continue;
    }

    if (!isIndex && !path) {
      throw new Error(`${appFile}: leaf route needs an index or a literal path`);
    }

    routes.push({
      sourcePath: resolvedPath,
      urlPath: navigablePath(resolvedPath)
    });
  }
}

export function readRenderRoutes(): RenderRoute[] {
  const source = readFileSync(appFile, "utf8");
  const [arrayStart, arrayEnd] = routerArray(source);
  const routes: RenderRoute[] = [];
  collectLeafRoutes(source, arrayStart, arrayEnd, "", routes);

  const seenPaths: Record<string, true | undefined> = {};
  for (const route of routes) {
    if (seenPaths[route.sourcePath]) {
      throw new Error(`${appFile}: duplicate leaf route ${route.sourcePath}`);
    }
    seenPaths[route.sourcePath] = true;
  }
  if (routes.length === 0) {
    throw new Error(`${appFile}: no leaf routes found`);
  }

  return routes;
}

function flattenMessages(value: unknown, messages: string[], locale: string): void {
  if (typeof value === "string") {
    messages.push(value);
    return;
  }

  if (value && typeof value === "object" && !Array.isArray(value)) {
    for (const nestedValue of Object.values(value)) {
      flattenMessages(nestedValue, messages, locale);
    }
    return;
  }

  throw new Error(`Locale ${locale} contains a non-string message value`);
}

export function readLocaleCatalogues(): LocaleCatalogues {
  const locales = sourceStringArray(i18nFile, "locales");

  return Object.fromEntries(
    locales.map((locale) => {
      const fileName = fileURLToPath(
        new URL(`../src/i18n/locales/${locale}.json`, import.meta.url)
      );
      const messages: string[] = [];
      flattenMessages(JSON.parse(readFileSync(fileName, "utf8")), messages, locale);
      return [locale, messages];
    })
  );
}

export function readThemePreferences(): string[] {
  return sourceStringArray(
    fileURLToPath(new URL("../src/app/theme.ts", import.meta.url)),
    "themePreferences"
  );
}
