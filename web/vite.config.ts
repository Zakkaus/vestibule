import { relative } from "node:path";
import { defineConfig, type Plugin } from "vite";

function cssProvenance(): Plugin {
  let root = "";
  return {
    name: "console-css-provenance",
    enforce: "post",
    configResolved(config) { root = config.root; },
    generateBundle(_options, bundle) {
      const css = Object.values(bundle).filter(
        (output) => output.type === "asset" && output.fileName.endsWith(".css")
      );
      if (css.length !== 1) {
        this.error(`Expected one complete CSS bundle, received ${css.length}`);
      }
      const origins = [...this.getModuleIds()]
        .map((id) => id.split("?")[0]!)
        .filter((id) => id.endsWith(".css"))
        .map((id) => {
          const path = relative(root, id).replaceAll("\\", "/");
          return { path, kind: path.startsWith("node_modules/") ? "vendor" : "project" };
        })
        .sort((a, b) => a.path.localeCompare(b.path));
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

export default defineConfig({
  plugins: [cssProvenance()],
  build: {
    cssCodeSplit: false,
    cssMinify: false
  }
});
