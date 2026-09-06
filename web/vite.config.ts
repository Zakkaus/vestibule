import { defineConfig } from "vite";

export default defineConfig({
  build: {
    cssMinify: false,
    license: { fileName: "THIRD-PARTY-LICENSES.txt" },
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "react", test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/, priority: 20 },
            { name: "mantine", test: /node_modules[\\/]@mantine[\\/]/, priority: 10 },
            { name: "catalogues", test: /src[\\/]i18n[\\/]locales[\\/]/ }
          ]
        }
      }
    }
  }
});
