# Vendored Lucide icons

`lucide/` contains byte-for-byte copies of the SVG files used by the console.
Do not edit them locally. Update from the declared package, then regenerate the
manifest hashes and run the icon provenance test.

- Collection: Lucide Static (`lucide-static`)
- Version: 1.39.0
- Source tarball: <https://registry.npmjs.org/lucide-static/-/lucide-static-1.39.0.tgz>
- Tarball integrity: `sha512-TaDiwNFZ78c+HrZRNWHoR+LVkIOL8ebYOyNYvytdPkaLWE29G9umks6p1aeCE8kSEIoifnlNLPxMvvE3zqT/YA==`
- Package license: ISC; the bundled `LICENSE` also carries the Feather MIT
  notice for derived icons.

`manifest.json` records the original package path and SHA-256 for every copied
SVG. `web/e2e/icons.spec.ts` rejects an unlisted SVG, a changed byte, or a raw
icon glyph outside this collection.
