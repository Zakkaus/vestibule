import { isAbsolute, relative } from "node:path";
import { defineConfig, type Plugin } from "vite";
import macros from "unplugin-parcel-macros";
import { removeRemoteFonts } from "./build/remove-remote-fonts.ts";

const cssSources = new Set<string>();

function cssProvenance(): Plugin {
  let root = "";
  return {
    name: "console-css-provenance",
    enforce: "post",
    configResolved(config) { root = config.root; },
    buildStart() { cssSources.clear(); },
    generateBundle(_options, bundle) {
      const css = Object.values(bundle).filter(
        (output) => output.type === "asset" && output.fileName.endsWith(".css")
      );
      if (css.length !== 1) {
        this.error(`Expected one complete CSS bundle, received ${css.length}`);
      }
      const modules = [...this.getModuleIds()];
      const macroModules = new Set(modules.filter((id) => /^macro-[a-f0-9]+\.css$/.test(id)));
      const stylesheets = new Set([...modules, ...cssSources]
        .map((id) => id.split("?")[0]!)
        .filter((id) => isAbsolute(id) && id.endsWith(".css") && !macroModules.has(relative(root, id))));
      const origins = [...stylesheets].map((id) => {
        const path = relative(root, id).replaceAll("\\", "/");
        return { path, kind: path.startsWith("node_modules/") ? "vendor" : "project" };
      });
      const macroSources = new Set([...macroModules]
        .flatMap((id) => this.getModuleInfo(id)?.importers ?? []));
      for (const id of macroSources) {
        origins.push({ path: relative(root, id).replaceAll("\\", "/"), kind: "spectrum-macro" });
      }
      origins.sort((a, b) => a.path.localeCompare(b.path));
      if (!origins.some((origin) => origin.kind === "project")) {
        this.error("No authored stylesheet appeared in the build module graph");
      }
      this.emitFile({
        type: "asset",
        fileName: "css-provenance.json",
        source: JSON.stringify({ version: 1, assets: [{ file: css[0]!.fileName, origins }] }, null, 2)
      });
    }
  };
}

const remoteFonts = removeRemoteFonts();

export default defineConfig({
  plugins: [macros.vite(), remoteFonts, cssProvenance()],
  optimizeDeps: {
    // The Node-only macro runs in the build plugin, not the browser dependency graph.
    exclude: ["@react-spectrum/s2/style"],
    rolldownOptions: {
      plugins: [remoteFonts]
    }
  },
  css: {
    postcss: {
      plugins: [{
        postcssPlugin: "console-css-source-origins",
        Once(root) {
          root.walk((node) => {
            if (node.source?.input.file) cssSources.add(node.source.input.file);
          });
        }
      }, {
        // Spectrum's stylesheet declares its typefaces against Adobe's font CDN. A
        // console that is installed on someone else's server must not call a third
        // party on every page load, and an air-gapped install would wait out the
        // request instead: the same wait that hangs document.fonts.ready in CI. The
        // declarations already carry system fallbacks, so dropping the remote faces
        // costs the shipped typeface, not the layout.
        postcssPlugin: "console-css-drop-remote-fonts",
        AtRule: {
          "font-face": (rule) => {
            const remote = rule.nodes?.some(
              (node) => node.type === "decl" && node.prop === "src" && /url\(\s*['"]?https?:/i.test(node.value)
            );
            if (remote) rule.remove();
          }
        }
      }]
    }
  },
  build: {
    cssCodeSplit: false,
    cssMinify: "lightningcss"
  }
});
