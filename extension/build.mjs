// Build the extension with esbuild into dist/.
//   - content script: IIFE (injected via chrome.scripting, must self-execute)
//   - background service worker + side panel: ESM
// Static assets (manifest, html, icons) are copied from public/.
import { build } from "esbuild";
import { cp, mkdir, rm } from "node:fs/promises";
import { existsSync } from "node:fs";

const outdir = "dist";
const watch = process.argv.includes("--watch");

await rm(outdir, { recursive: true, force: true });
await mkdir(outdir, { recursive: true });

const common = {
  bundle: true,
  sourcemap: true,
  target: "chrome114",
  logLevel: "info",
  minify: process.env.NODE_ENV === "production",
};

await build({
  ...common,
  entryPoints: { content: "src/content.ts" },
  format: "iife",
  outdir,
  // The trailing bare expression becomes the injected script's completion value,
  // which chrome.scripting.executeScript returns as `result`.
  footer: { js: "globalThis.__groveGrabResult;" },
});

await build({
  ...common,
  entryPoints: { background: "src/background.ts", sidepanel: "src/sidepanel.ts" },
  format: "esm",
  outdir,
});

// Copy static assets.
await cp("public", outdir, { recursive: true });

if (!existsSync("dist/manifest.json")) {
  throw new Error("manifest.json missing from dist — check public/");
}

console.log(`built → ${outdir}/`);
if (watch) {
  console.log("(watch mode not implemented; re-run on change)");
}
