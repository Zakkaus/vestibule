import { createHash } from "node:crypto";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

import { expect, test } from "@playwright/test";
import { readRenderRoutes } from "./render-gate-routes";

type IconManifest = Readonly<{
  collection: string;
  icons: readonly Readonly<{
    file: string;
    name: string;
    sha256: string;
    source: string;
  }>[];
  package: string;
  version: string;
}>;

const sourceRoot = fileURLToPath(new URL("../src", import.meta.url));
const iconRoot = join(sourceRoot, "icons", "lucide");
const manifestPath = join(iconRoot, "manifest.json");
const iconComponentPath = join(sourceRoot, "icons", "Icon.tsx");

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? sourceFiles(path) : [path];
  });
}
const sourceActionPattern =
  /<(button|Link)\b(?:(?!<\/?(?:button|Link)\b)[\s\S])*?data-slot="button"[\s\S]*?<\/\1>/g;

function sourceActionsWithoutIcons(): string[] {
  return sourceFiles(sourceRoot)
    .filter((path) => path.endsWith(".tsx"))
    .flatMap((path) => {
      const source = readFileSync(path, "utf8");
      const actions = [...source.matchAll(sourceActionPattern)];
      const slots = [...source.matchAll(/data-slot="button"/g)];

      if (actions.length !== slots.length) {
        return [
          `${relative(sourceRoot, path)}: source action parser covered ${actions.length} of ${slots.length} buttons`
        ];
      }

      return actions
        .filter((action) => !action[0].includes("<Icon"))
        .map((action) => {
          const line = source.slice(0, action.index ?? 0).split("\n").length;
          return `${relative(sourceRoot, path)}:${line}`;
        });
    })
    .sort();
}



test("Lucide icon assets are traceable and exclusive", () => {
  const manifest = JSON.parse(readFileSync(manifestPath, "utf8")) as IconManifest;
  const vendoredFiles = readdirSync(iconRoot)
    .filter((file) => file.endsWith(".svg"))
    .sort();
  const manifestFiles = manifest.icons.map((icon) => icon.file).sort();
  const registryFiles = [...readFileSync(iconComponentPath, "utf8").matchAll(
    /from "\.\/lucide\/([^"/]+)\.svg\?raw";/g
  )]
    .map((match) => `${match[1]}.svg`)
    .sort();

  expect(manifest.collection).toBe("Lucide Static");
  expect(manifest.package).toBe("lucide-static");
  expect(vendoredFiles).toEqual(manifestFiles);
  expect(registryFiles).toEqual(manifestFiles);

  for (const icon of manifest.icons) {
    const path = join(iconRoot, icon.file);
    const source = readFileSync(path, "utf8");

    expect(icon.source).toBe(`package/icons/${icon.file}`);
    expect(createHash("sha256").update(readFileSync(path)).digest("hex")).toBe(icon.sha256);
    expect(source).toContain(`@license lucide-static v${manifest.version} - ISC`);
  }

  const svgPaths = sourceFiles(sourceRoot)
    .filter((path) => path.endsWith(".svg"))
    .map((path) => relative(sourceRoot, path))
    .sort();
  expect(svgPaths).toEqual(manifestFiles.map((file) => join("icons", "lucide", file)).sort());

  // A component may inline an SVG only to draw a chart, and it says so on the
  // element. Naming the one file that was allowed to made the rule invisible to
  // the next chart: the home trend chart arrived from another branch and this
  // assertion reported it as a stray glyph. The rule is the declared capability,
  // not the filename.
  const rawSvgSources = sourceFiles(sourceRoot)
    .filter((path) => /\.tsx?$/.test(path))
    .flatMap((path) => {
      const source = readFileSync(path, "utf8");
      const openings = [...source.matchAll(/<svg\b[^>]*>/g)];
      if (openings.length === 0) {
        return [];
      }
      const undeclared = openings.filter((opening) => !/data-[a-z-]*chart[a-z-]*/.test(opening[0]));
      return undeclared.length === 0 ? [] : [relative(sourceRoot, path)];
    })
    .sort();
  expect(rawSvgSources).toEqual([]);
});

test("rendered console action buttons and navigation carry icons", async ({ page }) => {
  expect(sourceActionsWithoutIcons()).toEqual([]);
  for (const route of readRenderRoutes()) {
    await page.goto(route.urlPath);
    await page.locator("[data-app-shell]").waitFor({ state: "visible" });

    const missingButtons = await page.locator("[data-slot=\"button\"]").evaluateAll((buttons) =>
      buttons
        .filter((button) => !button.querySelector("[data-icon]"))
        .map((button) => button.textContent?.trim() ?? "")
    );
    const missingNavigation = await page.locator(".nav-item").evaluateAll((items) =>
      items
        .filter((item) => !item.querySelector("[data-icon]"))
        .map((item) => item.textContent?.trim() ?? "")
    );

    expect(missingButtons, `${route.sourcePath}: buttons without icons`).toEqual([]);
    expect(missingNavigation, `${route.sourcePath}: navigation without icons`).toEqual([]);
  }
});
