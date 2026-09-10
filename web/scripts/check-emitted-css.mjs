import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { transform } from "lightningcss";

function parseCSS(filename, code) {
  let declarations = 0;
  const result = transform({
    filename,
    code,
    errorRecovery: false,
    visitor: { Declaration() { declarations++; } }
  });
  if (result.warnings.length || declarations === 0) {
    throw new Error(`${filename}: ${result.warnings.length} parser warnings, ${declarations} declarations`);
  }
  return declarations;
}

try {
  const files = process.argv.slice(2);
  if (files.length === 1 && files[0] === "--self-test") {
    assert.equal(parseCSS("valid.css", Buffer.from(":root { --ink: red; } .known { color: var(--ink); }")), 2);
    for (const [name, css] of [
      ["selector", ":root { --ink: red; } .known: { color: var(--ink); }"],
      ["declaration", ".known { color red; }"],
      ["empty", ":root {}"]
    ]) {
      assert.throws(() => parseCSS(`${name}.css`, Buffer.from(css)), undefined, `${name} must fail`);
    }
    console.log("check:css self-test: valid CSS accepted; malformed selector, declaration, and empty stylesheet rejected");
  } else {
    if (!files.length) throw new Error("Pass every emitted CSS asset from check-css-coverage.py --print-emitted-assets");
    for (const file of files) {
      console.log(`check:css: ${file}: ${parseCSS(file, readFileSync(file))} declarations parsed`);
    }
  }
} catch (error) {
  console.error(`FAIL check:css: ${error.message}`);
  process.exitCode = 1;
}
